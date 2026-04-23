package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jcleira/magiklead/backend/internal/ingest"
)

// EdgarSource ingests SEC Form 3/4/5 insider-transaction filings via the
// quarterly bulk TSV dumps published at
// https://www.sec.gov/data-research/sec-markets-data/insider-transactions-data-sets.
// Each dump is a zip of tab-separated tables (SUBMISSION.tsv,
// REPORTINGOWNER.tsv, …); we join them to yield one SourceRecord per
// (filing × reporting officer). This is the highest-quality structured
// US-exec data on public companies — Tim Cook, Satya Nadella, et al.
// appear every few months when they exercise options or grant shares.
//
// The 10-K / DEF 14A HTML extraction mentioned in
// docs/2026-04-21-lead-database-architecture/plan.md §T04 is deferred:
// Form 4 already covers the target population (executive officers of
// US public companies) and does so with structured fields instead of
// HTML prose, so the marginal value of parsing the narrative sections
// of 10-Ks doesn't justify the scraper complexity in the first pass.
type EdgarSource struct {
	httpClient *http.Client
	userAgent  string
	maxRecords int
	now        func() time.Time
}

const (
	edgarBulkURL      = "https://www.sec.gov/files/structureddata/data/insider-transactions-data-sets/%dq%d_form345.zip"
	edgarHTTPTimeout  = 5 * time.Minute
	submissionTSV     = "SUBMISSION.tsv"
	reportingOwnerTSV = "REPORTINGOWNER.tsv"
)

// NewEdgarSource reads config from env vars and returns a configured
// source. SEC_USER_AGENT is required — the SEC rejects bulk requests
// without one (format: "Company Name contact@domain.com").
//
// EDGAR_MAX_RECORDS optionally caps parsed records per batch. A single
// quarter typically carries 100k–500k reporting-owner rows; before T05
// dedup lands, leaving this uncapped lets the resolver produce mountains
// of duplicate canonical rows. Set to e.g. 10000 for a smoke test.
func NewEdgarSource() (*EdgarSource, error) {
	userAgent := strings.TrimSpace(os.Getenv("SEC_USER_AGENT"))
	if userAgent == "" {
		return nil, errors.New(`SEC_USER_AGENT is required (format: "Company Name contact@domain.com")`)
	}
	max := 0
	if v := strings.TrimSpace(os.Getenv("EDGAR_MAX_RECORDS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid EDGAR_MAX_RECORDS=%q", v)
		}
		max = n
	}
	return &EdgarSource{
		httpClient: &http.Client{Timeout: edgarHTTPTimeout},
		userAgent:  userAgent,
		maxRecords: max,
		now:        time.Now,
	}, nil
}

func (*EdgarSource) Name() string { return "sec-edgar" }
func (*EdgarSource) Type() string { return "bulk-tsv" }

// Fetch downloads the quarterly zip for every quarter from `since` up
// to the current one. Missing quarters (e.g. the current one before
// SEC has published its file) are logged and skipped.
func (s *EdgarSource) Fetch(ctx context.Context, since time.Time) ([]ingest.RawBatch, error) {
	quarters := quartersSince(since, s.now())
	batches := make([]ingest.RawBatch, 0, len(quarters))
	for _, q := range quarters {
		url := fmt.Sprintf(edgarBulkURL, q.Year, q.Quarter)
		data, err := s.fetchURL(ctx, url)
		if errors.Is(err, errEdgarNotFound) {
			// Quarter not yet published — skip without failing the whole run.
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", url, err)
		}
		batches = append(batches, ingest.RawBatch{
			FileName: fmt.Sprintf("form345-%dq%d.zip", q.Year, q.Quarter),
			Content:  data,
			Meta: map[string]any{
				"year":    q.Year,
				"quarter": q.Quarter,
				"url":     url,
			},
		})
	}
	return batches, nil
}

// Parse unzips the bulk dump, joins SUBMISSION + REPORTINGOWNER on
// ACCESSION_NUMBER, and yields one SourceRecord per officer. Directors
// and 10%-owners are skipped unless they also carry the OFFICER flag,
// since their row here doesn't give a meaningful job title.
func (s *EdgarSource) Parse(ctx context.Context, raw ingest.RawBatch) ([]ingest.SourceRecord, error) {
	return parseInsiderZip(raw.Content, s.maxRecords)
}

// ---------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------

var errEdgarNotFound = errors.New("edgar: not found")

type quarter struct{ Year, Quarter int }

// quartersSince returns every (year, quarter) between `since` and `now`,
// inclusive. If `since` is zero, only the current quarter is returned.
func quartersSince(since, now time.Time) []quarter {
	endY, endQ := quarterOf(now)
	if since.IsZero() {
		return []quarter{{Year: endY, Quarter: endQ}}
	}
	startY, startQ := quarterOf(since)
	if startY > endY || (startY == endY && startQ > endQ) {
		return nil
	}
	var qs []quarter
	for y := startY; y <= endY; y++ {
		first, last := 1, 4
		if y == startY {
			first = startQ
		}
		if y == endY {
			last = endQ
		}
		for q := first; q <= last; q++ {
			qs = append(qs, quarter{Year: y, Quarter: q})
		}
	}
	return qs
}

func quarterOf(t time.Time) (int, int) {
	return t.Year(), int(t.Month()-1)/3 + 1
}

func (s *EdgarSource) fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept-Encoding", "gzip, deflate")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, errEdgarNotFound
	case http.StatusForbidden:
		return nil, fmt.Errorf("SEC returned 403 — check SEC_USER_AGENT (got %q)", s.userAgent)
	case http.StatusTooManyRequests:
		return nil, errors.New("SEC rate limit hit (429); back off and retry")
	default:
		return nil, fmt.Errorf("SEC returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// parseInsiderZip extracts SUBMISSION.tsv + REPORTINGOWNER.tsv from the
// archive, joins them, and yields SourceRecords.
func parseInsiderZip(data []byte, maxRecords int) ([]ingest.SourceRecord, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	submissions, err := readTSV(zr, submissionTSV)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", submissionTSV, err)
	}
	owners, err := readTSV(zr, reportingOwnerTSV)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", reportingOwnerTSV, err)
	}

	// Index submissions by ACCESSION_NUMBER so we can enrich each owner
	// row with the issuer data in one pass.
	subIdx := make(map[string]map[string]string, len(submissions))
	for _, row := range submissions {
		if acc := row["ACCESSION_NUMBER"]; acc != "" {
			subIdx[acc] = row
		}
	}

	records := make([]ingest.SourceRecord, 0, len(owners))
	for _, owner := range owners {
		acc := owner["ACCESSION_NUMBER"]
		sub := subIdx[acc]
		if sub == nil {
			continue
		}
		if !isOfficer(owner) {
			continue
		}

		name := firstNonEmpty(owner, "RPTOWNERNAME", "RPTOWNER_NAME", "RPT_OWNER_NAME")
		title := firstNonEmpty(owner, "OFFICER_TITLE", "RPTOWNER_TITLE")
		company := firstNonEmpty(sub, "ISSUERNAME", "ISSUER_NAME")
		if name == "" || title == "" || company == "" {
			continue
		}

		personCIK := firstNonEmpty(owner, "RPTOWNERCIK", "RPTOWNER_CIK", "RPT_OWNER_CIK")
		issuerCIK := firstNonEmpty(sub, "ISSUERCIK", "ISSUER_CIK")
		ticker := firstNonEmpty(sub, "ISSUERTRADINGSYMBOL", "ISSUER_TRADING_SYMBOL")

		records = append(records, ingest.SourceRecord{
			ExternalID: acc + ":" + personCIK,
			Fields: map[string]any{
				"name":          name,
				"title":         title,
				"company":       company,
				"issuer_cik":    issuerCIK,
				"issuer_ticker": ticker,
				"person_cik":    personCIK,
				"filing_date":   firstNonEmpty(sub, "FILING_DATE", "FILED_AS_OF_DATE"),
				"document_type": firstNonEmpty(sub, "DOCUMENT_TYPE"),
				"accession":     acc,
			},
		})
		if maxRecords > 0 && len(records) >= maxRecords {
			break
		}
	}
	return records, nil
}

// readTSV finds a tab-separated member in the zip (matched
// case-insensitively) and returns every data row as a
// map[header]value.
func readTSV(zr *zip.Reader, name string) ([]map[string]string, error) {
	var f *zip.File
	for _, entry := range zr.File {
		if strings.EqualFold(entry.Name, name) {
			f = entry
			break
		}
	}
	if f == nil {
		return nil, fmt.Errorf("%s not found in zip", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	r := csv.NewReader(rc)
	r.Comma = '\t'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}

	var rows []map[string]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}
		row := make(map[string]string, len(header))
		for i, col := range header {
			if i < len(rec) {
				row[col] = strings.TrimSpace(rec[i])
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// isOfficer returns true when the row represents an executive officer
// — either via an explicit IS_OFFICER / OFFICER boolean column or via
// a RPTOWNER_RELATIONSHIP enum containing "OFFICER". SEC has shipped
// both shapes historically, so accept either.
func isOfficer(row map[string]string) bool {
	for _, k := range []string{"IS_OFFICER", "OFFICER"} {
		if truthy(row[k]) {
			return true
		}
	}
	rel := strings.ToUpper(firstNonEmpty(row,
		"RPTOWNER_RELATIONSHIP", "RPTOWNERRELATIONSHIP", "RELATIONSHIP"))
	return strings.Contains(rel, "OFFICER")
}

func firstNonEmpty(row map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(row[k]); v != "" {
			return v
		}
	}
	return ""
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "y", "yes":
		return true
	}
	return false
}
