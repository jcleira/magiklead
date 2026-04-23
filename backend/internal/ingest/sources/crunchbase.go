package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcleira/magiklead/backend/internal/ingest"
)

// CrunchbaseSource ingests an exported CrunchBase bulk dump. CrunchBase
// publishes three coordinated CSVs — people, organizations, and jobs —
// that together describe the "who works where" graph. Joining jobs.csv
// onto the other two yields one SourceRecord per employment relationship.
//
// Access model: CrunchBase no longer ships daily bulk data for free.
// Users supply their own extract (partner licence, academic access,
// or a self-scraped subset) by pointing CRUNCHBASE_CSV_DIR at a
// directory with the three files. On each run we bundle them into a
// single zip so raw_ingests preserves the exact input we parsed
// against — matching the retention policy from research.md §5.
//
// Expected CSV schema (matches CrunchBase's historical export format):
//
//	people.csv        uuid, name, first_name, last_name,
//	                  linkedin_url, twitter_url, facebook_url,
//	                  homepage_url, primary_role
//	organizations.csv uuid, name, primary_role, domain, homepage_url,
//	                  linkedin_url, twitter_url, country_code,
//	                  founded_on
//	jobs.csv          uuid, person_uuid, org_uuid, title,
//	                  started_on, ended_on, is_current
//
// Missing optional columns are tolerated; missing required columns
// (the uuids and names) cause the row to be skipped.
type CrunchbaseSource struct {
	csvDir string
	now    func() time.Time
}

// NewCrunchbaseSource reads CRUNCHBASE_CSV_DIR and returns a configured
// source. Returns an error if the directory doesn't contain all three
// required files.
func NewCrunchbaseSource() (*CrunchbaseSource, error) {
	dir := strings.TrimSpace(os.Getenv("CRUNCHBASE_CSV_DIR"))
	if dir == "" {
		return nil, errors.New("CRUNCHBASE_CSV_DIR is required (directory containing people.csv, organizations.csv, jobs.csv)")
	}
	for _, name := range crunchbaseRequiredFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return nil, fmt.Errorf("crunchbase: %s missing from %s: %w", name, dir, err)
		}
	}
	return &CrunchbaseSource{csvDir: dir, now: time.Now}, nil
}

var crunchbaseRequiredFiles = []string{"people.csv", "organizations.csv", "jobs.csv"}

func (*CrunchbaseSource) Name() string { return "crunchbase" }
func (*CrunchbaseSource) Type() string { return "bulk-csv" }

// Fetch zips the three source CSVs into a single archival blob. Using
// zip (rather than three separate RawBatches) keeps each ingestion run
// atomic — either the whole dump lands or none of it does, and the
// checksum check covers the exact join input.
func (s *CrunchbaseSource) Fetch(ctx context.Context, since time.Time) ([]ingest.RawBatch, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range crunchbaseRequiredFiles {
		path := filepath.Join(s.csvDir, name)
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		w, err := zw.Create(name)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("zip create %s: %w", name, err)
		}
		if _, err := io.Copy(w, f); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("zip copy %s: %w", name, err)
		}
		_ = f.Close()
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("zip finalize: %w", err)
	}
	return []ingest.RawBatch{{
		FileName: fmt.Sprintf("crunchbase-%s.zip", s.now().UTC().Format("20060102-150405")),
		Content:  buf.Bytes(),
		Meta: map[string]any{
			"source_dir": s.csvDir,
			"files":      crunchbaseRequiredFiles,
		},
	}}, nil
}

// Parse unzips the bundle, loads the three CSVs into keyed maps, and
// emits one SourceRecord per jobs row that can be joined to both a
// person and an organization.
func (*CrunchbaseSource) Parse(ctx context.Context, raw ingest.RawBatch) ([]ingest.SourceRecord, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw.Content), int64(len(raw.Content)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	people, err := readCrunchbaseCSV(zr, "people.csv")
	if err != nil {
		return nil, fmt.Errorf("read people.csv: %w", err)
	}
	orgs, err := readCrunchbaseCSV(zr, "organizations.csv")
	if err != nil {
		return nil, fmt.Errorf("read organizations.csv: %w", err)
	}
	jobs, err := readCrunchbaseCSV(zr, "jobs.csv")
	if err != nil {
		return nil, fmt.Errorf("read jobs.csv: %w", err)
	}

	peopleByID := indexCSV(people, "uuid")
	orgsByID := indexCSV(orgs, "uuid")

	records := make([]ingest.SourceRecord, 0, len(jobs))
	for _, job := range jobs {
		personID := firstNonEmpty(job, "person_uuid", "person_id")
		orgID := firstNonEmpty(job, "org_uuid", "organization_uuid", "organization_id")
		if personID == "" || orgID == "" {
			continue
		}
		person := peopleByID[personID]
		org := orgsByID[orgID]
		if person == nil || org == nil {
			continue
		}

		name := firstNonEmpty(person, "name")
		if name == "" {
			first := firstNonEmpty(person, "first_name")
			last := firstNonEmpty(person, "last_name")
			name = strings.TrimSpace(first + " " + last)
		}
		if name == "" {
			continue
		}

		fields := map[string]any{
			"name":          name,
			"first_name":    firstNonEmpty(person, "first_name"),
			"last_name":     firstNonEmpty(person, "last_name"),
			"title":         firstNonEmpty(job, "title"),
			"company":       firstNonEmpty(org, "name"),
			"crunchbase_person_uuid": personID,
			"crunchbase_org_uuid":    orgID,
			"crunchbase_job_uuid":    firstNonEmpty(job, "uuid"),
		}
		if v := firstNonEmpty(org, "domain"); v != "" {
			fields["company_domain"] = v
		} else if v := firstNonEmpty(org, "homepage_url"); v != "" {
			if d := domainFromURL(v); d != "" {
				fields["company_domain"] = d
			}
		}
		if v := firstNonEmpty(person, "linkedin_url"); v != "" {
			fields["linkedin_url"] = v
		}
		if v := firstNonEmpty(person, "twitter_url"); v != "" {
			fields["twitter_url"] = v
		}
		if v := firstNonEmpty(job, "started_on", "start_date"); v != "" {
			fields["start_date"] = v
		}
		if v := firstNonEmpty(job, "ended_on", "end_date"); v != "" {
			fields["end_date"] = v
		}
		fields["is_current"] = truthy(firstNonEmpty(job, "is_current"))

		records = append(records, ingest.SourceRecord{
			ExternalID: fields["crunchbase_job_uuid"].(string),
			Fields:     fields,
		})
	}
	return records, nil
}

// readCrunchbaseCSV pulls a comma-separated CSV out of the zip and
// returns each data row as a map keyed by normalized header name.
// Headers are lowercased + trimmed so callers can look them up with
// stable keys regardless of CrunchBase's casing.
func readCrunchbaseCSV(zr *zip.Reader, name string) ([]map[string]string, error) {
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
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	for i := range header {
		header[i] = strings.ToLower(strings.TrimSpace(header[i]))
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

func indexCSV(rows []map[string]string, key string) map[string]map[string]string {
	idx := make(map[string]map[string]string, len(rows))
	for _, row := range rows {
		if id := row[key]; id != "" {
			idx[id] = row
		}
	}
	return idx
}
