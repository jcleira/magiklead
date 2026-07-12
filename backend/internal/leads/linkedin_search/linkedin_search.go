// Package linkedinsearch is the LinkedIn prospect source. It mirrors the
// PDL integration's write-through (internal/leads/pdl) — search results
// land in the canonical person graph keyed on a `linkedin_url`
// identifier so the same prospect is cached across tenants — but writes
// NO email: LinkedIn profiles carry no address, so the emails table is
// never touched and persons.has_email stays false.
//
// Production feeds this write-through from a RapidAPI LinkedIn
// people-search (default rockapis `linkedin-data-api`, behind a Doer
// seam); local dev gets its prospects from the seed loader (cmd/seed),
// which plants synthetic LinkedIn profiles through the same canonical
// path. The RapidAPI wire shape below is provisional; confirm it
// against the live endpoint when a real key is wired (only fetch/parse
// change — the write-through is source-agnostic).
//
// Per docs/2026-06-09-linkedin-only-outreach/issues/02-source-linkedin-prospects.md.
package linkedinsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// Doer is the HTTP-client seam — net/http.Client satisfies it, tests
// pass a stub.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// IdentifierType keys the person_identifiers row that links a canonical
// person to its LinkedIn profile and is the cross-tenant dedup key.
const IdentifierType = "linkedin_url"

// Common errors. The handler maps each to the right HTTP response.
var (
	ErrNotConfigured = errors.New("linkedinsearch: API key not configured")
	ErrUnauthorized  = errors.New("linkedinsearch: unauthorized (401)")
	ErrRateLimited   = errors.New("linkedinsearch: rate limited (429)")
	ErrMalformed     = errors.New("linkedinsearch: malformed response")
	ErrUpstream      = errors.New("linkedinsearch: upstream error")
)

// Module is the LinkedIn-search deep module. Construct one per process
// with New. All methods are safe for concurrent use.
type Module struct {
	pool    *pgxpool.Pool
	q       *repository.Queries
	apiKey  string
	doer    Doer
	baseURL string
	host    string
}

// New returns a Module backed by the given pgxpool. Pass an empty apiKey
// to construct a module that reports Configured()==false (the RapidAPI
// path is then unavailable; demo write-through still works). Pass nil
// for doer to use the default *http.Client.
func New(pool *pgxpool.Pool, apiKey string, doer Doer) *Module {
	if doer == nil {
		doer = &http.Client{Timeout: 30 * time.Second}
	}
	return &Module{
		pool:    pool,
		q:       repository.New(pool),
		apiKey:  apiKey,
		doer:    doer,
		baseURL: "https://linkedin-data-api.p.rapidapi.com",
		host:    "linkedin-data-api.p.rapidapi.com",
	}
}

// Configured reports whether the RapidAPI key is set. The handler uses
// this to degrade (like PDL) when no key is set in dev.
func (m *Module) Configured() bool { return m != nil && m.apiKey != "" }

// SetBaseURL overrides the RapidAPI base URL. Tests point it at an
// httptest.Server; production callers leave the default.
func (m *Module) SetBaseURL(u string) { m.baseURL = strings.TrimRight(u, "/") }

// Filters describes a LinkedIn people-search request.
type Filters struct {
	Titles    []string
	Locations []string
	Keywords  string
	Limit     int
}

// Profile is the source-agnostic input to WriteThrough — what both the
// RapidAPI path and the demo scraper produce.
type Profile struct {
	FullName    string
	FirstName   string
	LastName    string
	Title       string
	CompanyName string
	Domain      string
	LinkedInURL string
	Location    string
}

// Person is the post-write-through canonical view returned by Search /
// WriteThrough. IDs reference rows now present in persons / organizations.
type Person struct {
	PersonID         uuid.UUID
	Name             string
	FirstName        string
	LastName         string
	Title            string
	OrganizationID   uuid.UUID
	OrganizationName string
	Domain           string
	Location         string
	LinkedInURL      string
}

// Search calls the RapidAPI people-search, writes every returned profile
// through to the canonical graph, and returns the canonical view.
func (m *Module) Search(ctx context.Context, f Filters) ([]Person, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}
	profiles, err := m.fetch(ctx, f)
	if err != nil {
		return nil, err
	}
	return m.WriteThrough(ctx, profiles)
}

// WriteThrough lands profiles in the canonical: organization (by
// primary_domain) → person (deduped by the linkedin_url identifier) →
// linkedin_url identifier → current employment. It writes no email.
// Profiles without a LinkedIn URL are skipped (the URL is the dedup key).
func (m *Module) WriteThrough(ctx context.Context, profiles []Profile) ([]Person, error) {
	out := make([]Person, 0, len(profiles))
	for _, p := range profiles {
		person, err := m.writeOne(ctx, p)
		if err != nil {
			log.Printf("linkedinsearch: write-through %q: %v", p.LinkedInURL, err)
			continue
		}
		out = append(out, person)
	}
	return out, nil
}

func (m *Module) writeOne(ctx context.Context, p Profile) (Person, error) {
	linkedinURL := strings.TrimSpace(p.LinkedInURL)
	if linkedinURL == "" {
		return Person{}, errors.New("linkedinsearch: profile has no linkedin_url")
	}

	org, err := m.upsertOrganization(ctx, p)
	if err != nil {
		return Person{}, err
	}
	person, isNew, err := m.upsertPerson(ctx, p, linkedinURL)
	if err != nil {
		return Person{}, err
	}
	if isNew {
		if err := m.q.CreatePersonIdentifier(ctx, repository.CreatePersonIdentifierParams{
			PersonID:        person.ID,
			IdentifierType:  IdentifierType,
			IdentifierValue: linkedinURL,
			IsPrimary:       pgtype.Bool{Bool: true, Valid: true},
		}); err != nil {
			return Person{}, fmt.Errorf("linkedinsearch: link linkedin_url: %w", err)
		}
	}
	if err := m.upsertEmployment(ctx, person.ID, org.ID, p.Title); err != nil {
		return Person{}, err
	}

	out := Person{
		PersonID:         uuid.UUID(person.ID.Bytes),
		Name:             person.CanonicalName,
		Title:            p.Title,
		OrganizationID:   uuid.UUID(org.ID.Bytes),
		OrganizationName: org.CanonicalName,
		LinkedInURL:      linkedinURL,
	}
	if person.FirstName.Valid {
		out.FirstName = person.FirstName.String
	}
	if person.LastName.Valid {
		out.LastName = person.LastName.String
	}
	if person.Location.Valid {
		out.Location = person.Location.String
	}
	if org.PrimaryDomain.Valid {
		out.Domain = org.PrimaryDomain.String
	}
	return out, nil
}

func (m *Module) upsertOrganization(ctx context.Context, p Profile) (repository.Organization, error) {
	domain := strings.ToLower(strings.TrimSpace(p.Domain))
	name := strings.TrimSpace(p.CompanyName)
	if name == "" && domain == "" {
		name = "Unknown"
	}

	if domain != "" {
		existing, err := m.q.FindOrganizationByPrimaryDomain(ctx, pgText(domain))
		if err == nil {
			return m.q.UpdateOrganizationFromPDL(ctx, repository.UpdateOrganizationFromPDLParams{
				ID:            existing.ID,
				CanonicalName: name,
				Industries:    nil, // LinkedIn search carries no industry
				SizeRange:     pgtype.Text{},
			})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return repository.Organization{}, fmt.Errorf("linkedinsearch: lookup org by domain: %w", err)
		}
	}

	return m.q.CreateOrganizationFromPDL(ctx, repository.CreateOrganizationFromPDLParams{
		CanonicalName: name,
		PrimaryDomain: pgText(domain),
		Industries:    nil,
		SizeRange:     pgtype.Text{},
	})
}

// upsertPerson dedups on the linkedin_url identifier and returns whether
// the person was newly inserted (the caller writes the identifier only
// on inserts). has_email is never set true on this path.
func (m *Module) upsertPerson(ctx context.Context, p Profile, linkedinURL string) (repository.Person, bool, error) {
	fullName := profileName(p)
	existing, err := m.q.FindPersonByIdentifier(ctx, repository.FindPersonByIdentifierParams{
		IdentifierType:  IdentifierType,
		IdentifierValue: linkedinURL,
	})
	if err == nil {
		updated, err := m.q.UpdatePersonFromPDL(ctx, repository.UpdatePersonFromPDLParams{
			ID:             existing.ID,
			CanonicalName:  fullName,
			FirstName:      pgText(p.FirstName),
			LastName:       pgText(p.LastName),
			NormalizedName: normalize(fullName),
			Location:       pgText(p.Location),
			HasEmail:       false,
		})
		return updated, false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return repository.Person{}, false, fmt.Errorf("linkedinsearch: lookup person: %w", err)
	}

	created, err := m.q.CreatePersonFromPDL(ctx, repository.CreatePersonFromPDLParams{
		CanonicalName:  fullName,
		FirstName:      pgText(p.FirstName),
		LastName:       pgText(p.LastName),
		NormalizedName: normalize(fullName),
		Location:       pgText(p.Location),
		HasEmail:       false,
	})
	return created, true, err
}

func (m *Module) upsertEmployment(ctx context.Context, personID, orgID pgtype.UUID, title string) error {
	existing, err := m.q.FindCurrentEmployment(ctx, repository.FindCurrentEmploymentParams{
		PersonID:       personID,
		OrganizationID: orgID,
	})
	if err == nil {
		if !existing.Title.Valid || existing.Title.String != title {
			return m.q.UpdateEmploymentTitle(ctx, repository.UpdateEmploymentTitleParams{
				ID:    existing.ID,
				Title: pgText(title),
			})
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("linkedinsearch: lookup employment: %w", err)
	}
	_, err = m.q.CreateEmployment(ctx, repository.CreateEmploymentParams{
		PersonID:       personID,
		OrganizationID: orgID,
		Title:          pgText(title),
		IsCurrent:      pgtype.Bool{Bool: true, Valid: true},
	})
	return err
}

// fetch calls the RapidAPI people-search endpoint and parses the result
// into source-agnostic profiles.
func (m *Module) fetch(ctx context.Context, f Filters) ([]Profile, error) {
	q := url.Values{}
	keywords := strings.TrimSpace(f.Keywords)
	if keywords == "" {
		keywords = strings.Join(f.Titles, " ")
	}
	q.Set("keywords", keywords)
	if len(f.Locations) > 0 {
		q.Set("geo", strings.Join(f.Locations, ","))
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(searchSize(f.Limit)))
	}

	resp, err := m.do(ctx, http.MethodGet, "/search-people?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if err := mapStatus(resp.StatusCode, raw); err != nil {
		return nil, err
	}
	return parseSearch(raw)
}

func (m *Module) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.baseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("linkedinsearch: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-RapidAPI-Key", m.apiKey)
	req.Header.Set("X-RapidAPI-Host", m.host)
	resp, err := m.doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linkedinsearch: do request: %w", err)
	}
	return resp, nil
}

// -------- RapidAPI wire shapes (provisional) -------------------------------

type searchResponse struct {
	Data struct {
		Items []rapidProfile `json:"items"`
	} `json:"data"`
}

type rapidProfile struct {
	FullName      string `json:"fullName"`
	FirstName     string `json:"firstName"`
	LastName      string `json:"lastName"`
	Headline      string `json:"headline"`
	ProfileURL    string `json:"profileURL"`
	Username      string `json:"username"`
	CompanyName   string `json:"companyName"`
	CompanyDomain string `json:"companyDomain"`
	Location      string `json:"location"`
}

func parseSearch(raw []byte) ([]Profile, error) {
	var sr searchResponse
	if err := json.Unmarshal(raw, &sr); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	out := make([]Profile, 0, len(sr.Data.Items))
	for _, it := range sr.Data.Items {
		linkedinURL := strings.TrimSpace(it.ProfileURL)
		if linkedinURL == "" && strings.TrimSpace(it.Username) != "" {
			linkedinURL = "https://www.linkedin.com/in/" + strings.TrimSpace(it.Username)
		}
		out = append(out, Profile{
			FullName:    strings.TrimSpace(it.FullName),
			FirstName:   strings.TrimSpace(it.FirstName),
			LastName:    strings.TrimSpace(it.LastName),
			Title:       strings.TrimSpace(it.Headline),
			CompanyName: strings.TrimSpace(it.CompanyName),
			Domain:      strings.TrimSpace(it.CompanyDomain),
			LinkedInURL: linkedinURL,
			Location:    strings.TrimSpace(it.Location),
		})
	}
	return out, nil
}

func mapStatus(code int, raw []byte) error {
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return ErrUnauthorized
	case code == http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("%w: http %d: %s", ErrUpstream, code, raw)
	}
}

// SyntheticURL builds a deterministic linkedin_url for demo profiles that
// have no real one (the company-site scraper yields names + titles, not
// profile links). Namespaced by domain so two people with the same name
// at different companies don't collide on the globally-unique identifier.
func SyntheticURL(name, domain string) string {
	slug := slugify(name)
	if d := slugify(strings.SplitN(domain, ".", 2)[0]); d != "" {
		slug = slug + "-" + d
	}
	return "https://www.linkedin.com/in/" + slug
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func profileName(p Profile) string {
	if n := strings.TrimSpace(p.FullName); n != "" {
		return n
	}
	return strings.TrimSpace(strings.TrimSpace(p.FirstName) + " " + strings.TrimSpace(p.LastName))
}

func searchSize(n int) int {
	if n <= 0 {
		return 25
	}
	if n > 100 {
		return 100
	}
	return n
}

// normalize matches the canonical name-normalization: lowercase +
// collapsed whitespace. Kept inline to avoid an import cycle (mirrors
// the same helper in internal/leads/pdl).
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
