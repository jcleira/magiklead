// Package pdl integrates People Data Labs as the real-data spine
// behind the canonical person graph. PDL search results write through
// to the canonical (persons, organizations, employments, emails) so
// subsequent identical lookups across any tenant hit the cache for
// free; stale rows (>90 days) re-trigger a PDL call from the handler
// layer.
//
// Per docs/2026-05-07-real-product-release/issues/07-pdl-integration.md.
package pdl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// Doer is the HTTP-client seam — net/http.Client satisfies it, and
// tests pass a stub.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// StaleAfter is the 90-day cache TTL. After this, canonical rows are
// treated as a cache miss by the handler.
const StaleAfter = 90 * 24 * time.Hour

// VerificationMethod is the value written into emails.verification_method
// for write-through emails. The worker's send path can read this to
// distinguish PDL-verified emails from smtp-rcpt-verified.
const VerificationMethod = "pdl-verified"

// PDLIdentifierType keys person_identifiers rows that link a canonical
// person to its PDL provenance.
const PDLIdentifierType = "pdl_id"

// Common errors. The handler maps each to the right HTTP response;
// tests assert each one fires on the matching PDL status code.
var (
	ErrRateLimited     = errors.New("pdl: rate limited (429)")
	ErrCreditExhausted = errors.New("pdl: credit exhausted (402)")
	ErrUnauthorized    = errors.New("pdl: unauthorized (401)")
	ErrMalformed       = errors.New("pdl: malformed response")
	ErrNotConfigured   = errors.New("pdl: API key not configured")
)

// Module is the PDL deep module. Construct one per process with
// New(pool, apiKey, doer). All methods are safe for concurrent use.
type Module struct {
	pool    *pgxpool.Pool
	q       *repository.Queries
	apiKey  string
	doer    Doer
	baseURL string
}

// New returns a Module backed by the given pgxpool. Pass nil for
// `doer` to use the default *http.Client.
func New(pool *pgxpool.Pool, apiKey string, doer Doer) *Module {
	if doer == nil {
		doer = &http.Client{Timeout: 30 * time.Second}
	}
	return &Module{
		pool:    pool,
		q:       repository.New(pool),
		apiKey:  apiKey,
		doer:    doer,
		baseURL: "https://api.peopledatalabs.com/v5",
	}
}

// Configured reports whether the module has an API key and can call
// out to PDL. The handler uses this to fall through to canonical-only
// when no key is set in dev.
func (m *Module) Configured() bool { return m != nil && m.apiKey != "" }

// SetBaseURL overrides the PDL base URL. Tests use it to point at
// httptest.Server; production callers should leave the default.
func (m *Module) SetBaseURL(u string) { m.baseURL = u }

// Filters describes a PDL person-search request. The free-text
// Description carries over from the onboarding flow's ICP field and
// is appended to the structured Elasticsearch query.
type Filters struct {
	Titles      []string
	Industries  []string
	CompanySize string
	Locations   []string
	Description string
	Limit       int
}

// Person is the post-write-through canonical view returned by Search.
// IDs reference rows now present in persons / organizations.
type Person struct {
	PersonID         uuid.UUID
	Name             string
	FirstName        string
	LastName         string
	Title            string
	Location         string
	OrganizationID   uuid.UUID
	OrganizationName string
	Domain           string
	Industries       []string
	SizeRange        string
	Email            string
	EmailVerified    bool
	// HasEmail is true when PDL knows the person is emailable, even if
	// the address itself is gated behind a paid plan (free-tier signal).
	HasEmail bool
}

// Email is a verified email returned by Enrich.
type Email struct {
	Email    string
	Verified bool
	PersonID uuid.UUID
}

// SearchWithCache returns canonical persons matching f. It first
// queries the canonical for fresh rows (updated_at within the 90-day
// staleness window); on a cache miss it falls through to PDL.Search
// (which writes back through to the canonical) and re-queries. The
// second return value reports whether PDL was actually called —
// callers use it for observability and to surface "we just paid for
// credits" in logs.
//
// Cache hit: canonical has at least one fresh row matching f → PDL
// is not called. Stale cache: canonical has only rows whose
// updated_at is older than 90 days → PDL is called and canonical
// rows are refreshed.
func (m *Module) SearchWithCache(ctx context.Context, f Filters) ([]Person, bool, error) {
	cached, err := m.lookupCanonical(ctx, f, true)
	if err != nil {
		return nil, false, err
	}
	if len(cached) > 0 {
		return cached, false, nil
	}
	if !m.Configured() {
		// No PDL key in dev — fall back to canonical without the
		// freshness gate so existing seed data is still surfaced.
		stale, err := m.lookupCanonical(ctx, f, false)
		return stale, false, err
	}
	if _, err := m.Search(ctx, f); err != nil {
		return nil, true, err
	}
	out, err := m.lookupCanonical(ctx, f, false)
	return out, true, err
}

// Search calls PDL's person-search API, writes every returned record
// through to the canonical graph, and returns the canonical view.
func (m *Module) Search(ctx context.Context, f Filters) ([]Person, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}

	body, err := json.Marshal(searchRequest{
		Query: buildESQuery(f),
		Size:  searchSize(f.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("pdl: marshal request: %w", err)
	}

	resp, err := m.do(ctx, http.MethodPost, "/person/search", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if err := mapStatus(resp.StatusCode, raw); err != nil {
		return nil, err
	}

	var sr searchResponse
	if err := json.Unmarshal(raw, &sr); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}

	out := make([]Person, 0, len(sr.Data))
	for _, rec := range sr.Data {
		p, err := m.writeThrough(ctx, rec)
		if err != nil {
			log.Printf("pdl: write-through %s: %v", rec.ID, err)
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// Enrich calls PDL's person-enrichment API for an already-known
// canonical person. Used after Search returns a person without a
// verified email — the enrichment API can sometimes surface contact
// details that the search API doesn't include.
func (m *Module) Enrich(ctx context.Context, personID uuid.UUID) (Email, error) {
	if !m.Configured() {
		return Email{}, ErrNotConfigured
	}

	// Locate the PDL id we stored at write-through; without it we have
	// no stable key to pass to the enrichment endpoint.
	pdlID, err := m.lookupPDLID(ctx, personID)
	if err != nil {
		return Email{}, err
	}

	q := fmt.Sprintf("/person/enrich?pdl_id=%s", pdlID)
	resp, err := m.do(ctx, http.MethodGet, q, nil)
	if err != nil {
		return Email{}, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if err := mapStatus(resp.StatusCode, raw); err != nil {
		return Email{}, err
	}

	var er enrichResponse
	if err := json.Unmarshal(raw, &er); err != nil {
		return Email{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	email := pickEmail(er.Data)
	if email == "" {
		return Email{}, nil
	}

	row, err := m.q.UpsertEmailVerifiedFromPDL(ctx, repository.UpsertEmailVerifiedFromPDLParams{
		Lower:    email,
		PersonID: pgUUID(personID),
	})
	if err != nil {
		return Email{}, fmt.Errorf("pdl: enrich write-through: %w", err)
	}
	return Email{
		Email:    row.Email,
		Verified: row.VerifiedAt.Valid,
		PersonID: personID,
	}, nil
}

// writeThrough lands one PDL record in the canonical: organization →
// person → person_identifier → employment → email (if present).
// Returns the canonical view of the record.
func (m *Module) writeThrough(ctx context.Context, rec pdlPerson) (Person, error) {
	org, err := m.upsertOrganization(ctx, rec)
	if err != nil {
		return Person{}, err
	}
	person, isNew, err := m.upsertPerson(ctx, rec)
	if err != nil {
		return Person{}, err
	}
	if isNew {
		if err := m.q.CreatePersonIdentifier(ctx, repository.CreatePersonIdentifierParams{
			PersonID:        person.ID,
			IdentifierType:  PDLIdentifierType,
			IdentifierValue: rec.ID,
			IsPrimary:       pgtype.Bool{Bool: true, Valid: true},
		}); err != nil {
			return Person{}, fmt.Errorf("pdl: link pdl_id: %w", err)
		}
	}

	if err := m.upsertEmployment(ctx, person.ID, org.ID, rec.JobTitle); err != nil {
		return Person{}, err
	}

	out := Person{
		PersonID:         uuid.UUID(person.ID.Bytes),
		Name:             person.CanonicalName,
		Title:            rec.JobTitle,
		OrganizationID:   uuid.UUID(org.ID.Bytes),
		OrganizationName: org.CanonicalName,
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
	out.Industries = org.Industries
	if org.SizeRange.Valid {
		out.SizeRange = org.SizeRange.String
	}
	out.HasEmail = person.HasEmail

	if email := pickEmail(rec); email != "" {
		row, err := m.q.UpsertEmailVerifiedFromPDL(ctx, repository.UpsertEmailVerifiedFromPDLParams{
			Lower:    email,
			PersonID: person.ID,
		})
		if err != nil {
			return out, fmt.Errorf("pdl: upsert email: %w", err)
		}
		out.Email = row.Email
		out.EmailVerified = row.VerifiedAt.Valid
	}
	return out, nil
}

func (m *Module) upsertOrganization(ctx context.Context, rec pdlPerson) (repository.Organization, error) {
	domain := strings.ToLower(strings.TrimSpace(rec.JobCompanyWebsite))
	name := strings.TrimSpace(rec.JobCompanyName)
	if name == "" && domain == "" {
		// PDL person without any company hook — invent a placeholder
		// so the canonical row is still well-formed. Rare; logs would
		// help if it spikes.
		name = "Unknown"
	}

	if domain != "" {
		existing, err := m.q.FindOrganizationByPrimaryDomain(ctx, pgText(domain))
		if err == nil {
			return m.q.UpdateOrganizationFromPDL(ctx, repository.UpdateOrganizationFromPDLParams{
				ID:            existing.ID,
				CanonicalName: name,
				Industries:    industriesArg(rec.JobCompanyIndustry),
				SizeRange:     pgText(rec.JobCompanySize),
			})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return repository.Organization{}, fmt.Errorf("pdl: lookup org by domain: %w", err)
		}
	}

	return m.q.CreateOrganizationFromPDL(ctx, repository.CreateOrganizationFromPDLParams{
		CanonicalName: name,
		PrimaryDomain: pgText(domain),
		Industries:    industriesArg(rec.JobCompanyIndustry),
		SizeRange:     pgText(rec.JobCompanySize),
	})
}

// upsertPerson returns the canonical row and whether it was newly
// inserted (the caller writes the pdl_id identifier only on inserts).
func (m *Module) upsertPerson(ctx context.Context, rec pdlPerson) (repository.Person, bool, error) {
	emailable := hasEmail(rec)
	existing, err := m.q.FindPersonByPDLID(ctx, rec.ID)
	if err == nil {
		updated, err := m.q.UpdatePersonFromPDL(ctx, repository.UpdatePersonFromPDLParams{
			ID:             existing.ID,
			CanonicalName:  rec.FullName,
			FirstName:      pgText(rec.FirstName),
			LastName:       pgText(rec.LastName),
			NormalizedName: normalize(rec.FullName),
			Location:       pgText(rec.LocationName.Value),
			HasEmail:       emailable,
		})
		return updated, false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return repository.Person{}, false, fmt.Errorf("pdl: lookup person: %w", err)
	}

	created, err := m.q.CreatePersonFromPDL(ctx, repository.CreatePersonFromPDLParams{
		CanonicalName:  rec.FullName,
		FirstName:      pgText(rec.FirstName),
		LastName:       pgText(rec.LastName),
		NormalizedName: normalize(rec.FullName),
		Location:       pgText(rec.LocationName.Value),
		HasEmail:       emailable,
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
		return fmt.Errorf("pdl: lookup employment: %w", err)
	}
	_, err = m.q.CreateEmployment(ctx, repository.CreateEmploymentParams{
		PersonID:       personID,
		OrganizationID: orgID,
		Title:          pgText(title),
		IsCurrent:      pgtype.Bool{Bool: true, Valid: true},
	})
	return err
}

// lookupCanonical runs SearchPersons against the canonical graph,
// optionally gating on the 90-day freshness window. Returns the
// canonical view (without email attachment — callers that need that
// run ListBestEmailsForPersons themselves).
func (m *Module) lookupCanonical(ctx context.Context, f Filters, requireFresh bool) ([]Person, error) {
	rows, err := m.q.SearchPersons(ctx, repository.SearchPersonsParams{
		Titles:       trimSlice(f.Titles),
		WithEmail:    false,
		Industries:   lowerSlice(f.Industries),
		CompanySize:  pgText(strings.TrimSpace(f.CompanySize)),
		Locations:    trimSlice(f.Locations),
		RequireFresh: requireFresh,
		ResultLimit:  int32(searchSize(f.Limit)),
		ResultOffset: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("pdl: canonical search: %w", err)
	}
	out := make([]Person, 0, len(rows))
	personIDs := make([]pgtype.UUID, 0, len(rows))
	for _, r := range rows {
		personIDs = append(personIDs, r.PersonID)
	}
	emailByPerson := map[[16]byte]string{}
	if len(personIDs) > 0 {
		es, err := m.q.ListBestEmailsForPersons(ctx, personIDs)
		if err != nil {
			return nil, fmt.Errorf("pdl: list emails: %w", err)
		}
		for _, e := range es {
			emailByPerson[e.PersonID.Bytes] = e.Email
		}
	}
	for _, r := range rows {
		p := Person{
			PersonID:         uuid.UUID(r.PersonID.Bytes),
			Name:             r.PersonCanonicalName,
			OrganizationID:   uuid.UUID(r.OrganizationID.Bytes),
			OrganizationName: r.OrganizationName,
			Industries:       r.Industries,
		}
		if r.FirstName.Valid {
			p.FirstName = r.FirstName.String
		}
		if r.LastName.Valid {
			p.LastName = r.LastName.String
		}
		if r.Title.Valid {
			p.Title = r.Title.String
		}
		if r.Location.Valid {
			p.Location = r.Location.String
		}
		if r.PrimaryDomain.Valid {
			p.Domain = r.PrimaryDomain.String
		}
		if r.SizeRange.Valid {
			p.SizeRange = r.SizeRange.String
		}
		p.HasEmail = r.HasEmail
		if e, ok := emailByPerson[r.PersonID.Bytes]; ok {
			p.Email = e
			p.EmailVerified = true
		}
		out = append(out, p)
	}
	return out, nil
}

func trimSlice(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func lowerSlice(in []string) []string {
	out := trimSlice(in)
	for i := range out {
		out[i] = strings.ToLower(out[i])
	}
	return out
}

func (m *Module) lookupPDLID(ctx context.Context, personID uuid.UUID) (string, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT identifier_value FROM person_identifiers
		 WHERE person_id = $1 AND identifier_type = $2 LIMIT 1`,
		pgUUID(personID), PDLIdentifierType)
	if err != nil {
		return "", fmt.Errorf("pdl: lookup identifier: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return "", fmt.Errorf("pdl: no pdl_id linked to person %s", personID)
	}
	var v string
	if err := rows.Scan(&v); err != nil {
		return "", err
	}
	return v, nil
}

func (m *Module) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.baseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("pdl: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Api-Key", m.apiKey)
	resp, err := m.doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdl: do request: %w", err)
	}
	return resp, nil
}

func mapStatus(code int, raw []byte) error {
	switch code {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusPaymentRequired:
		return ErrCreditExhausted
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		if code >= 400 {
			return fmt.Errorf("pdl: http %d: %s", code, raw)
		}
		return nil
	}
}

// -------- PDL wire shapes --------------------------------------------------

type searchRequest struct {
	Query map[string]any `json:"query"`
	Size  int            `json:"size"`
}

type searchResponse struct {
	Status int         `json:"status"`
	Data   []pdlPerson `json:"data"`
	Error  struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type enrichResponse struct {
	Status int       `json:"status"`
	Data   pdlPerson `json:"data"`
}

type pdlPerson struct {
	ID                 string       `json:"id"`
	FullName           string       `json:"full_name"`
	FirstName          string       `json:"first_name"`
	LastName           string       `json:"last_name"`
	JobTitle           string       `json:"job_title"`
	JobCompanyName     string       `json:"job_company_name"`
	JobCompanyWebsite  string       `json:"job_company_website"`
	JobCompanyIndustry string       `json:"job_company_industry"`
	JobCompanySize     string       `json:"job_company_size"`
	LocationName       pdlOptString `json:"location_name"`
	WorkEmail          pdlOptString `json:"work_email"`
	Emails             pdlMails     `json:"emails"`
}

// pdlOptString decodes a PDL field that is normally a string but, on
// plans without contact-data access, comes back as a boolean presence
// flag instead (true = "a value exists here you can't see", false =
// none). It captures the address when present and the presence bit
// either way, so a redacted field degrades to "no address, known
// emailable" rather than 502-ing the whole search.
type pdlOptString struct {
	Value   string
	Present bool
}

func (s *pdlOptString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = pdlOptString{}
		return nil
	}
	switch b[0] {
	case '"':
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		v = strings.TrimSpace(v)
		*s = pdlOptString{Value: v, Present: v != ""}
		return nil
	case 't', 'f':
		var present bool
		if err := json.Unmarshal(b, &present); err != nil {
			return err
		}
		*s = pdlOptString{Present: present}
		return nil
	default:
		return fmt.Errorf("pdl: string-or-bool field: unexpected JSON %s", b)
	}
}

// pdlMails decodes PDL's emails field, normally an array of
// {address,type} but returned as a boolean presence flag on plans
// without contact access (same obfuscation as pdlOptString).
type pdlMails struct {
	Mails   []pdlMail
	Present bool
}

func (e *pdlMails) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*e = pdlMails{}
		return nil
	}
	switch b[0] {
	case '[':
		var ms []pdlMail
		if err := json.Unmarshal(b, &ms); err != nil {
			return err
		}
		*e = pdlMails{Mails: ms, Present: len(ms) > 0}
		return nil
	case 't', 'f':
		var present bool
		if err := json.Unmarshal(b, &present); err != nil {
			return err
		}
		*e = pdlMails{Present: present}
		return nil
	default:
		return fmt.Errorf("pdl: emails: unexpected JSON %s", b)
	}
}

type pdlMail struct {
	Address string `json:"address"`
	Type    string `json:"type"`
}

// buildESQuery shapes filters into PDL's Elasticsearch query DSL. PDL
// accepts either a structured `query` object or a free-text `query`
// string; we always send the bool form so the wire shape is stable
// across requests.
func buildESQuery(f Filters) map[string]any {
	var must []map[string]any
	for _, t := range f.Titles {
		if t = strings.TrimSpace(t); t != "" {
			must = append(must, map[string]any{"match": map[string]any{"job_title": t}})
		}
	}
	for _, ind := range f.Industries {
		if ind = strings.TrimSpace(ind); ind != "" {
			must = append(must, map[string]any{"term": map[string]any{"job_company_industry": strings.ToLower(ind)}})
		}
	}
	if cs := strings.TrimSpace(f.CompanySize); cs != "" {
		must = append(must, map[string]any{"term": map[string]any{"job_company_size": cs}})
	}
	for _, loc := range f.Locations {
		if loc = strings.TrimSpace(loc); loc != "" {
			must = append(must, map[string]any{"match": map[string]any{"location_name": loc}})
		}
	}
	if d := strings.TrimSpace(f.Description); d != "" {
		must = append(must, map[string]any{"query_string": map[string]any{"query": d}})
	}
	if len(must) == 0 {
		// PDL requires at least one clause; "exists work_email" is the
		// safest "any record" default.
		must = append(must, map[string]any{"exists": map[string]any{"field": "work_email"}})
	}
	return map[string]any{"bool": map[string]any{"must": must}}
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

func pickEmail(rec pdlPerson) string {
	if e := strings.TrimSpace(rec.WorkEmail.Value); e != "" {
		return strings.ToLower(e)
	}
	for _, m := range rec.Emails.Mails {
		if e := strings.TrimSpace(m.Address); e != "" {
			return strings.ToLower(e)
		}
	}
	return ""
}

// hasEmail reports whether PDL knows of an email for this person —
// true when a real address is present, or when a contact field came
// back as a gated presence flag (a paid plan would reveal the address).
// This is the signal persisted to persons.has_email so the canonical
// search can surface emailable prospects even on the free tier.
func hasEmail(rec pdlPerson) bool {
	return rec.WorkEmail.Present || rec.Emails.Present
}

func industriesArg(industry string) []string {
	industry = strings.TrimSpace(industry)
	if industry == "" {
		return nil
	}
	return []string{strings.ToLower(industry)}
}

// normalize is the same name-normalization used by the ingest path:
// lowercase + collapsed whitespace. Kept inline to avoid a circular
// import on internal/ingest.
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
