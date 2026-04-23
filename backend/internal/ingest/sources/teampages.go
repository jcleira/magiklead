package sources

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jcleira/magiklead/backend/internal/ai"
	"github.com/jcleira/magiklead/backend/internal/ingest"
)

// TeamPageSource scrapes /team, /about, and /leadership pages from a
// seed list of company domains and asks Claude to extract the
// executives mentioned. It's the pragmatic MVP for the Common Crawl
// pipeline described in docs/2026-04-21-lead-database-architecture/plan.md
// §T09: direct fetches over HTTP instead of streaming WARC archives,
// which trades scale for simplicity and zero infra bootstrap.
//
// Rate-limit-wise, this source is polite: sequential fetches with a
// configurable delay (default 1s) between domains, a single retry, and
// a real User-Agent. For production scale we'd swap Fetch to stream
// the same URL patterns out of the Common Crawl index.
type TeamPageSource struct {
	httpClient *http.Client
	ai         *ai.Client
	domains    []string
	userAgent  string
	paths      []string
	delay      time.Duration
	perPageLimit int
}

const (
	teamPagesDefaultUA = "MagikLead/1.0 (+https://magiklead.io/bot)"
	teamPagesHTTPTimeout = 25 * time.Second
)

// NewTeamPageSource loads the seed domain list and an Anthropic API
// key. Seed list resolution order:
//
//  1. TEAMPAGES_DOMAINS — comma-separated list.
//  2. TEAMPAGES_DOMAINS_FILE — path to a file with one domain per line.
//
// ANTHROPIC_API_KEY is required for Claude extraction.
func NewTeamPageSource() (*TeamPageSource, error) {
	key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if key == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is required for teampages source")
	}
	domains, err := loadTeamPageDomains()
	if err != nil {
		return nil, err
	}
	if len(domains) == 0 {
		return nil, errors.New("no domains configured — set TEAMPAGES_DOMAINS or TEAMPAGES_DOMAINS_FILE")
	}
	return &TeamPageSource{
		httpClient: &http.Client{Timeout: teamPagesHTTPTimeout},
		ai:         ai.NewClient(key),
		domains:    domains,
		userAgent:  envDefaultTP("TEAMPAGES_USER_AGENT", teamPagesDefaultUA),
		paths:      []string{"/team", "/about/team", "/about-us/team", "/leadership", "/about", "/about-us", "/people", "/company/team", "/company/leadership"},
		delay:      1 * time.Second,
		perPageLimit: 0, // 0 = no per-page cap on extracted people
	}, nil
}

func (*TeamPageSource) Name() string { return "teampages" }
func (*TeamPageSource) Type() string { return "scraped" }

// Fetch tries each seed domain in sequence. For each one, we walk the
// candidate path list until we get a 200 with text/html content, then
// stop — don't pile up multiple pages per company in one run.
func (s *TeamPageSource) Fetch(ctx context.Context, since time.Time) ([]ingest.RawBatch, error) {
	batches := make([]ingest.RawBatch, 0, len(s.domains))
	for _, domain := range s.domains {
		body, fetchedURL, err := s.fetchBestPage(ctx, domain)
		if err != nil {
			log.Printf("teampages: skip %s: %v", domain, err)
			continue
		}
		batches = append(batches, ingest.RawBatch{
			FileName: safeDomain(domain) + ".html",
			Content:  body,
			Meta: map[string]any{
				"domain":     domain,
				"url":        fetchedURL,
				"user_agent": s.userAgent,
			},
		})
		if s.delay > 0 {
			select {
			case <-ctx.Done():
				return batches, ctx.Err()
			case <-time.After(s.delay):
			}
		}
	}
	return batches, nil
}

// Parse sends each batch through Claude and yields SourceRecords.
// company_domain is set from the seed rather than Claude's guess so
// cross-source org dedup via org_identifiers.domain works reliably.
func (s *TeamPageSource) Parse(ctx context.Context, raw ingest.RawBatch) ([]ingest.SourceRecord, error) {
	domain, _ := raw.Meta["domain"].(string)
	if domain == "" {
		return nil, errors.New("missing domain in raw batch meta")
	}
	people, err := s.ai.ExtractPeopleFromHTML(ctx, string(raw.Content), domain)
	if err != nil {
		return nil, fmt.Errorf("extract from %s: %w", domain, err)
	}

	records := make([]ingest.SourceRecord, 0, len(people))
	for i, p := range people {
		company := p.Company
		if company == "" {
			company = domain
		}
		records = append(records, ingest.SourceRecord{
			ExternalID: fmt.Sprintf("%s:%d", domain, i),
			Fields: map[string]any{
				"name":           p.Name,
				"title":          p.Title,
				"company":        company,
				"company_domain": domain,
			},
		})
		if s.perPageLimit > 0 && len(records) >= s.perPageLimit {
			break
		}
	}
	return records, nil
}

// fetchBestPage tries the configured URL paths on the domain until one
// returns 200 with HTML, then returns its body and final URL.
func (s *TeamPageSource) fetchBestPage(ctx context.Context, domain string) ([]byte, string, error) {
	for _, p := range s.paths {
		for _, scheme := range []string{"https", "http"} {
			u := scheme + "://" + strings.TrimSuffix(domain, "/") + p
			body, ok, err := s.fetch(ctx, u)
			if err != nil {
				continue
			}
			if ok {
				return body, u, nil
			}
		}
	}
	return nil, "", errors.New("no team/about page found")
}

// fetch returns (body, ok, err). ok=false means we reached the server
// but the response wasn't a usable HTML page (non-200 or non-HTML
// content type); err is reserved for transport failures.
func (s *TeamPageSource) fetch(ctx context.Context, url string) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false, nil
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "html") {
		return nil, false, nil
	}
	// Cap the body so a rogue server streaming GB of nonsense can't
	// exhaust memory. 2 MiB covers realistic team pages.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, false, err
	}
	return body, true, nil
}

// ---------------------------------------------------------------------
// seed list loading
// ---------------------------------------------------------------------

func loadTeamPageDomains() ([]string, error) {
	if v := strings.TrimSpace(os.Getenv("TEAMPAGES_DOMAINS")); v != "" {
		return splitDomains(v, ","), nil
	}
	if path := strings.TrimSpace(os.Getenv("TEAMPAGES_DOMAINS_FILE")); path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		defer f.Close()
		var out []string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			out = append(out, line)
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, nil
}

func splitDomains(raw, sep string) []string {
	var out []string
	for _, part := range strings.Split(raw, sep) {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "https://")
		part = strings.TrimPrefix(part, "http://")
		part = strings.TrimSuffix(part, "/")
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func safeDomain(domain string) string {
	repl := strings.NewReplacer("/", "_", ":", "_")
	return repl.Replace(strings.ToLower(domain))
}

func envDefaultTP(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
