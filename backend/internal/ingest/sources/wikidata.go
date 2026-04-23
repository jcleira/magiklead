package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jcleira/magiklead/backend/internal/ingest"
)

// WikidataSource pulls executive data from Wikidata's public SPARQL
// endpoint. The default query targets humans who hold the CEO position
// (Q484876) and optionally pulls a SEC CIK (P5531) when Wikidata has
// one on file — those rows dedupe with EDGAR automatically through the
// resolver's `cik` identifier path. CEOs without a CIK fall to T08's
// name-based cross-source matching.
//
// Rate-limit notes: query.wikidata.org has a 60-second query timeout
// and a per-user concurrency cap. See
// https://www.mediawiki.org/wiki/Wikidata_Query_Service/User_Manual#Query_limits.
// WMF's User-Agent policy (https://meta.wikimedia.org/wiki/User-Agent_policy)
// requires identifying contact info.
type WikidataSource struct {
	httpClient *http.Client
	userAgent  string
	endpoint   string
	query      string
}

const (
	wikidataEndpoint    = "https://query.wikidata.org/sparql"
	wikidataHTTPTimeout = 2 * time.Minute

	// defaultWikidataQuery returns humans who hold the CEO position
	// (Q484876) with their employer, using Wikidata's "truthy"
	// (wdt:) claims only. That's much cheaper than the full statement
	// model (p:/ps:/pq:) and reliably completes inside the 60s cap.
	//
	// What we give up in exchange:
	//  * start/end dates — those live on statement qualifiers, so a
	//    callsite that needs them must override WIKIDATA_QUERY;
	//  * multi-employment history — wdt: returns the "best" value only.
	//
	// What stayed in:
	//  * P5531 (SEC CIK) on both person and company, for identifier-
	//    based cross-source dedup with EDGAR;
	//  * P856 (official website) for extracting company_domain.
	defaultWikidataQuery = `
SELECT DISTINCT ?person ?personLabel ?personCik ?personLinkedIn ?company ?companyLabel ?companyCik ?companyWebsite ?companyLinkedIn WHERE {
  ?person wdt:P31 wd:Q5;
          wdt:P39 wd:Q484876;
          wdt:P108 ?company.
  OPTIONAL { ?person wdt:P5531 ?personCik. }
  OPTIONAL { ?person wdt:P6634 ?personLinkedIn. }
  OPTIONAL { ?company wdt:P5531 ?companyCik. }
  OPTIONAL { ?company wdt:P856 ?companyWebsite. }
  OPTIONAL { ?company wdt:P4264 ?companyLinkedIn. }
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en". }
}
LIMIT 500`
)

// NewWikidataSource reads config from env vars.
// WIKIDATA_USER_AGENT is required — WMF rejects requests without one.
// WIKIDATA_QUERY optionally overrides the default SPARQL query.
// WIKIDATA_ENDPOINT optionally overrides the endpoint (useful for tests).
func NewWikidataSource() (*WikidataSource, error) {
	ua := strings.TrimSpace(os.Getenv("WIKIDATA_USER_AGENT"))
	if ua == "" {
		return nil, errors.New(`WIKIDATA_USER_AGENT is required (format: "MagikLead/1.0 contact@magiklead.io")`)
	}
	query := strings.TrimSpace(os.Getenv("WIKIDATA_QUERY"))
	if query == "" {
		query = defaultWikidataQuery
	}
	endpoint := strings.TrimSpace(os.Getenv("WIKIDATA_ENDPOINT"))
	if endpoint == "" {
		endpoint = wikidataEndpoint
	}
	return &WikidataSource{
		httpClient: &http.Client{Timeout: wikidataHTTPTimeout},
		userAgent:  ua,
		endpoint:   endpoint,
		query:      query,
	}, nil
}

func (*WikidataSource) Name() string { return "wikidata" }
func (*WikidataSource) Type() string { return "sparql" }

// Fetch issues one SPARQL request and emits the raw JSON response as a
// single RawBatch. `since` is ignored — SPARQL doesn't expose
// incremental-change semantics for ad-hoc queries; rely on raw_ingests
// checksum dedup to skip unchanged result sets on re-runs.
func (s *WikidataSource) Fetch(ctx context.Context, since time.Time) ([]ingest.RawBatch, error) {
	u := s.endpoint + "?query=" + url.QueryEscape(s.query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wikidata request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		snippet := string(body)
		if len(snippet) > 300 {
			snippet = snippet[:300] + "…"
		}
		return nil, fmt.Errorf("wikidata returned %d: %s", resp.StatusCode, snippet)
	}

	return []ingest.RawBatch{{
		FileName: "wikidata-" + time.Now().UTC().Format("20060102-150405") + ".json",
		Content:  body,
		Meta: map[string]any{
			"endpoint":  s.endpoint,
			"query_len": len(s.query),
		},
	}}, nil
}

// Parse turns the SPARQL JSON response into SourceRecords. One row per
// binding becomes one SourceRecord — a single person with multiple
// positions yields multiple records, deduped later by the resolver via
// person_cik.
func (*WikidataSource) Parse(ctx context.Context, raw ingest.RawBatch) ([]ingest.SourceRecord, error) {
	var resp wikidataResponse
	if err := json.Unmarshal(raw.Content, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal sparql results: %w", err)
	}

	records := make([]ingest.SourceRecord, 0, len(resp.Results.Bindings))
	for _, b := range resp.Results.Bindings {
		name := bindingValue(b, "personLabel")
		if name == "" || looksLikeQID(name) {
			// Skip rows where the label service didn't resolve a human
			// name (we'd end up with "Q1234" as canonical_name).
			continue
		}
		fields := map[string]any{
			"name":        name,
			"wikidata_id": trimEntityURL(bindingValue(b, "person")),
		}
		if v := bindingValue(b, "personCik"); v != "" {
			fields["person_cik"] = v
		}
		if v := bindingValue(b, "personLinkedIn"); v != "" {
			// P6634 stores the bare handle (e.g. "timcook"); wrap it so
			// normalization downstream produces the same key every source
			// would.
			fields["linkedin_url"] = "https://www.linkedin.com/in/" + v
		}
		// Every row of the default query binds ?position to Q484876 so
		// the label service returns "chief executive officer" for every
		// row. When positionLabel is missing (custom WIKIDATA_QUERY) or
		// leaks through as a Q-ID we fall back to the same string so
		// employment rows always get a title.
		title := bindingValue(b, "positionLabel")
		if title == "" || looksLikeQID(title) {
			title = "Chief Executive Officer"
		}
		fields["title"] = title
		if v := bindingValue(b, "companyLabel"); v != "" && !looksLikeQID(v) {
			fields["company"] = v
		}
		if v := bindingValue(b, "companyCik"); v != "" {
			fields["issuer_cik"] = v
		}
		if v := bindingValue(b, "companyWebsite"); v != "" {
			if d := domainFromURL(v); d != "" {
				fields["company_domain"] = d
			}
		}
		if v := bindingValue(b, "companyLinkedIn"); v != "" {
			fields["company_linkedin_url"] = "https://www.linkedin.com/company/" + v
		}
		if v := bindingValue(b, "start"); v != "" {
			fields["start_date"] = v
		}
		if v := bindingValue(b, "end"); v != "" {
			fields["end_date"] = v
		}
		records = append(records, ingest.SourceRecord{
			ExternalID: trimEntityURL(bindingValue(b, "person")),
			Fields:     fields,
		})
	}
	return records, nil
}

// ---------------------------------------------------------------------
// SPARQL JSON shape + helpers
// ---------------------------------------------------------------------

type wikidataResponse struct {
	Results struct {
		Bindings []map[string]wikidataBinding `json:"bindings"`
	} `json:"results"`
}

type wikidataBinding struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func bindingValue(b map[string]wikidataBinding, key string) string {
	return strings.TrimSpace(b[key].Value)
}

// trimEntityURL returns the Q-ID (e.g. "Q267") from a full entity URL
// like "http://www.wikidata.org/entity/Q267".
func trimEntityURL(raw string) string {
	if raw == "" {
		return ""
	}
	if i := strings.LastIndex(raw, "/"); i != -1 {
		return raw[i+1:]
	}
	return raw
}

// looksLikeQID is true for unresolved entity identifiers that leak
// through as labels when Wikidata's label service can't find an
// English label for an item.
func looksLikeQID(s string) bool {
	if len(s) < 2 || (s[0] != 'Q' && s[0] != 'q') {
		return false
	}
	for _, r := range s[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// domainFromURL extracts the bare hostname from a homepage URL and
// strips a leading "www." so "https://www.apple.com/" becomes "apple.com".
func domainFromURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Host)
	return strings.TrimPrefix(host, "www.")
}
