package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// DefaultResolver reconciles source records with the canonical tables.
//
// Matching strategy for persons, in order:
//
//  1. Strong identifiers (SEC CIK, then LinkedIn URL). Exact hit →
//     auto-merge at 100. These are the T08 cross-source glue — every
//     source that exposes either one deduplicates for free.
//  2. Fuzzy name match scoped to the current organization via pg_trgm.
//     Auto-merges at ≥ 0.95, review-queues at ≥ 0.70.
//  3. Fuzzy normalized-name match across ALL organizations (T08). The
//     normalized form (lowercase, punctuation stripped, tokens sorted
//     alphabetically) makes "Musk Elon" / "Elon Musk" match. Uses a
//     higher 0.95 threshold because org context isn't verifying the
//     match.
//  4. No match → new canonical row.
//
// Organizations follow a simpler identifier-or-create path (CIK,
// domain, LinkedIn URL). Employments dedupe on (person, org, title)
// so re-ingesting the same Form 4 in a later quarter doesn't produce
// parallel rows.
type DefaultResolver struct {
	autoMergeThreshold float32
	reviewThreshold    float32
	crossOrgThreshold  float32
}

// NewDefaultResolver returns a resolver with the thresholds from
// research.md §1.
func NewDefaultResolver() *DefaultResolver {
	return &DefaultResolver{
		autoMergeThreshold: 0.95,
		reviewThreshold:    0.70,
		crossOrgThreshold:  0.95,
	}
}

const (
	identifierCIK      = "cik"
	identifierLinkedIn = "linkedin"
	identifierDomain   = "domain"
)

func (r *DefaultResolver) Resolve(ctx context.Context, q *repository.Queries, in ResolveInput) error {
	orgID, orgConfidence, err := r.resolveOrganization(ctx, q, in)
	if err != nil {
		return fmt.Errorf("resolve organization: %w", err)
	}

	personID, personConfidence, personConflict, err := r.resolvePerson(ctx, q, in, orgID)
	if err != nil {
		return fmt.Errorf("resolve person: %w", err)
	}
	if !personID.Valid {
		return nil
	}

	if orgID.Valid {
		if err := r.ensureEmployment(ctx, q, personID, orgID, in); err != nil {
			return fmt.Errorf("ensure employment: %w", err)
		}
	}

	if _, err := q.CreateEvidence(ctx, repository.CreateEvidenceParams{
		CanonicalTable: "persons",
		CanonicalID:    personID,
		SourceRecordID: in.SourceRecordID,
		Confidence:     personConfidence,
		ConflictFlag:   pgtype.Bool{Bool: personConflict, Valid: true},
	}); err != nil {
		return fmt.Errorf("create person evidence: %w", err)
	}

	if orgID.Valid {
		if _, err := q.CreateEvidence(ctx, repository.CreateEvidenceParams{
			CanonicalTable: "organizations",
			CanonicalID:    orgID,
			SourceRecordID: in.SourceRecordID,
			Confidence:     orgConfidence,
			ConflictFlag:   pgtype.Bool{Bool: false, Valid: true},
		}); err != nil {
			return fmt.Errorf("create org evidence: %w", err)
		}
	}

	if personConflict {
		if _, err := q.CreateConflictQueueItem(ctx, repository.CreateConflictQueueItemParams{
			CanonicalTable:    "persons",
			CanonicalID:       personID,
			NewSourceRecordID: in.SourceRecordID,
			Status:            "pending",
		}); err != nil {
			return fmt.Errorf("enqueue conflict: %w", err)
		}
	}

	return nil
}

// ---------------------------------------------------------------------
// Organization resolution
// ---------------------------------------------------------------------

func (r *DefaultResolver) resolveOrganization(ctx context.Context, q *repository.Queries, in ResolveInput) (pgtype.UUID, int16, error) {
	name := firstString(in.Record.Fields, "company", "organization", "employer")
	if name == "" {
		return pgtype.UUID{}, 0, nil
	}

	identifiers := orgIdentifiers(in.Record.Fields)

	// Tier 1 — identifier lookup, in priority order.
	for _, id := range identifiers {
		org, err := q.FindOrganizationByIdentifier(ctx, repository.FindOrganizationByIdentifierParams{
			IdentifierType:  id.typ,
			IdentifierValue: id.value,
		})
		if err == nil {
			if err := r.augmentOrganization(ctx, q, org, name, identifiers); err != nil {
				return pgtype.UUID{}, 0, err
			}
			return org.ID, 100, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, 0, err
		}
	}

	// Tier 2 — create a fresh organization with every identifier we have.
	primaryDomain := ""
	for _, id := range identifiers {
		if id.typ == identifierDomain {
			primaryDomain = id.value
			break
		}
	}
	newOrg, err := q.CreateOrganization(ctx, repository.CreateOrganizationParams{
		CanonicalName: name,
		PrimaryDomain: pgtype.Text{String: primaryDomain, Valid: primaryDomain != ""},
	})
	if err != nil {
		return pgtype.UUID{}, 0, err
	}
	for _, id := range identifiers {
		if err := q.CreateOrganizationIdentifier(ctx, repository.CreateOrganizationIdentifierParams{
			OrganizationID:  newOrg.ID,
			IdentifierType:  id.typ,
			IdentifierValue: id.value,
			IsPrimary:       pgtype.Bool{Bool: id.typ == identifierCIK, Valid: true},
		}); err != nil {
			return pgtype.UUID{}, 0, err
		}
	}
	return newOrg.ID, 100, nil
}

// augmentOrganization handles a hit on an existing org: records any new
// identifiers we've learned and files the current label as an alias if
// the canonical name differs.
func (r *DefaultResolver) augmentOrganization(ctx context.Context, q *repository.Queries, org repository.Organization, name string, ids []identifier) error {
	if !strings.EqualFold(org.CanonicalName, name) {
		if err := q.CreateOrganizationAlias(ctx, repository.CreateOrganizationAliasParams{
			OrganizationID: org.ID,
			Alias:          name,
			AliasType:      "source-name",
		}); err != nil {
			return err
		}
	}
	for _, id := range ids {
		existing, err := q.FindOrganizationByIdentifier(ctx, repository.FindOrganizationByIdentifierParams{
			IdentifierType:  id.typ,
			IdentifierValue: id.value,
		})
		if err == nil && existing.ID == org.ID {
			continue
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil && existing.ID != org.ID {
			// Identifier already points somewhere else — skip rather than
			// clobber. A real conflict, left for the admin panel to
			// resolve once it exists.
			continue
		}
		if err := q.CreateOrganizationIdentifier(ctx, repository.CreateOrganizationIdentifierParams{
			OrganizationID:  org.ID,
			IdentifierType:  id.typ,
			IdentifierValue: id.value,
			IsPrimary:       pgtype.Bool{Bool: false, Valid: true},
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------
// Person resolution
// ---------------------------------------------------------------------

func (r *DefaultResolver) resolvePerson(ctx context.Context, q *repository.Queries, in ResolveInput, orgID pgtype.UUID) (pgtype.UUID, int16, bool, error) {
	name := firstString(in.Record.Fields, "name", "full_name", "canonical_name")
	if name == "" {
		return pgtype.UUID{}, 0, false, nil
	}
	ids := personIdentifiers(in.Record.Fields)

	// Blocklist gate (T16). Check the source's strong identifiers
	// against deletion_blocklist BEFORE any tier — fuzzy name matches
	// could otherwise re-attach an erased person's email/linkedin to
	// a different canonical and defeat the erasure.
	if hashes := blocklistHashes(in.Record.Fields, ids); len(hashes) > 0 {
		blocked, err := q.AnyHashBlocked(ctx, hashes)
		if err != nil {
			return pgtype.UUID{}, 0, false, err
		}
		if blocked {
			return pgtype.UUID{}, 0, false, nil
		}
	}

	// Tier 1 — identifier lookup (CIK, then LinkedIn).
	for _, id := range ids {
		person, err := q.FindPersonByIdentifier(ctx, repository.FindPersonByIdentifierParams{
			IdentifierType:  id.typ,
			IdentifierValue: id.value,
		})
		if err == nil {
			if err := r.augmentPerson(ctx, q, person, name, ids); err != nil {
				return pgtype.UUID{}, 0, false, err
			}
			return person.ID, 100, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, 0, false, err
		}
	}

	// Tier 2 — fuzzy name match at the current org.
	if orgID.Valid {
		candidates, err := q.FindPersonsByNameAtOrg(ctx, repository.FindPersonsByNameAtOrgParams{
			QueryName:      name,
			OrganizationID: orgID,
		})
		if err != nil {
			return pgtype.UUID{}, 0, false, err
		}
		if len(candidates) > 0 {
			best := candidates[0]
			switch {
			case best.Sim >= r.autoMergeThreshold:
				person, err := q.FindPersonByIdentifier(ctx, repository.FindPersonByIdentifierParams{})
				_ = person
				_ = err
				if err := r.augmentPersonByID(ctx, q, best.ID, name, ids); err != nil {
					return pgtype.UUID{}, 0, false, err
				}
				return best.ID, int16(best.Sim*100 + 0.5), false, nil
			case best.Sim >= r.reviewThreshold:
				if err := r.augmentPersonByID(ctx, q, best.ID, name, ids); err != nil {
					return pgtype.UUID{}, 0, false, err
				}
				return best.ID, int16(best.Sim*100 + 0.5), true, nil
			}
		}
	}

	// Tier 3 — cross-org fuzzy on normalized name. Tight threshold
	// because org context isn't grounding the match.
	normalized := NormalizeName(name)
	if normalized != "" {
		candidates, err := q.FindPersonsByNormalizedName(ctx, normalized)
		if err != nil {
			return pgtype.UUID{}, 0, false, err
		}
		if len(candidates) > 0 && candidates[0].Sim >= r.crossOrgThreshold {
			if err := r.augmentPersonByID(ctx, q, candidates[0].ID, name, ids); err != nil {
				return pgtype.UUID{}, 0, false, err
			}
			return candidates[0].ID, int16(candidates[0].Sim*100 + 0.5), false, nil
		}
	}

	// Tier 4 — create a new canonical person.
	first, last := splitName(name, in.Record.Fields)
	newPerson, err := q.CreatePerson(ctx, repository.CreatePersonParams{
		CanonicalName:  name,
		FirstName:      pgtype.Text{String: first, Valid: first != ""},
		LastName:       pgtype.Text{String: last, Valid: last != ""},
		NormalizedName: normalized,
	})
	if err != nil {
		return pgtype.UUID{}, 0, false, err
	}
	for _, id := range ids {
		if err := q.CreatePersonIdentifier(ctx, repository.CreatePersonIdentifierParams{
			PersonID:        newPerson.ID,
			IdentifierType:  id.typ,
			IdentifierValue: id.value,
			IsPrimary:       pgtype.Bool{Bool: id.typ == identifierCIK, Valid: true},
		}); err != nil {
			return pgtype.UUID{}, 0, false, err
		}
	}
	return newPerson.ID, 100, false, nil
}

// augmentPerson is augmentPersonByID with the canonical row already in
// hand (skips one DB roundtrip).
func (r *DefaultResolver) augmentPerson(ctx context.Context, q *repository.Queries, person repository.Person, name string, ids []identifier) error {
	if !strings.EqualFold(person.CanonicalName, name) {
		if err := q.CreatePersonAlias(ctx, repository.CreatePersonAliasParams{
			PersonID:  person.ID,
			Alias:     name,
			AliasType: "source-name",
		}); err != nil {
			return err
		}
	}
	return r.attachPersonIdentifiers(ctx, q, person.ID, ids)
}

// augmentPersonByID attaches aliases + identifiers given only the UUID.
func (r *DefaultResolver) augmentPersonByID(ctx context.Context, q *repository.Queries, personID pgtype.UUID, name string, ids []identifier) error {
	if err := q.CreatePersonAlias(ctx, repository.CreatePersonAliasParams{
		PersonID:  personID,
		Alias:     name,
		AliasType: "source-name",
	}); err != nil {
		return err
	}
	return r.attachPersonIdentifiers(ctx, q, personID, ids)
}

func (r *DefaultResolver) attachPersonIdentifiers(ctx context.Context, q *repository.Queries, personID pgtype.UUID, ids []identifier) error {
	for _, id := range ids {
		existing, err := q.FindPersonByIdentifier(ctx, repository.FindPersonByIdentifierParams{
			IdentifierType:  id.typ,
			IdentifierValue: id.value,
		})
		if err == nil && existing.ID == personID {
			continue
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil && existing.ID != personID {
			// Same identifier already points to a different canonical —
			// don't clobber. Admin review will reconcile.
			continue
		}
		if err := q.CreatePersonIdentifier(ctx, repository.CreatePersonIdentifierParams{
			PersonID:        personID,
			IdentifierType:  id.typ,
			IdentifierValue: id.value,
			IsPrimary:       pgtype.Bool{Bool: false, Valid: true},
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------
// Employment
// ---------------------------------------------------------------------

func (r *DefaultResolver) ensureEmployment(ctx context.Context, q *repository.Queries, personID, orgID pgtype.UUID, in ResolveInput) error {
	title := firstString(in.Record.Fields, "title", "role", "position")
	titleParam := pgtype.Text{String: title, Valid: title != ""}

	if _, err := q.FindCurrentEmploymentByTitle(ctx, repository.FindCurrentEmploymentByTitleParams{
		PersonID:       personID,
		OrganizationID: orgID,
		Title:          titleParam,
	}); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	_, err := q.CreateEmployment(ctx, repository.CreateEmploymentParams{
		PersonID:       personID,
		OrganizationID: orgID,
		Title:          titleParam,
		IsCurrent:      pgtype.Bool{Bool: true, Valid: true},
	})
	return err
}

// ---------------------------------------------------------------------
// Identifier extraction
// ---------------------------------------------------------------------

type identifier struct {
	typ   string
	value string
}

// personIdentifiers collects all identifiers discoverable in a source
// record's fields, in priority order (most reliable first).
func personIdentifiers(fields map[string]any) []identifier {
	var out []identifier
	if v := firstString(fields, "person_cik"); v != "" {
		out = append(out, identifier{identifierCIK, v})
	}
	if v := NormalizeLinkedInURL(firstString(fields, "linkedin_url", "linkedin")); v != "" {
		out = append(out, identifier{identifierLinkedIn, v})
	}
	return out
}

func orgIdentifiers(fields map[string]any) []identifier {
	var out []identifier
	if v := firstString(fields, "issuer_cik"); v != "" {
		out = append(out, identifier{identifierCIK, v})
	}
	if v := NormalizeDomain(firstString(fields, "company_domain", "domain")); v != "" {
		out = append(out, identifier{identifierDomain, v})
	}
	if v := NormalizeLinkedInURL(firstString(fields, "company_linkedin_url", "linkedin_url_company")); v != "" {
		out = append(out, identifier{identifierLinkedIn, v})
	}
	return out
}

// ---------------------------------------------------------------------
// Normalization helpers
// ---------------------------------------------------------------------

// NormalizeLinkedInURL coerces any linkedin.com profile URL to
// "linkedin.com/<path>" (lowercase, no scheme, no trailing slash, no
// www). Non-LinkedIn URLs return "".
func NormalizeLinkedInURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Tolerate bare paths like "/in/timcook" — treat them as linkedin.com.
	if strings.HasPrefix(raw, "/") {
		raw = "https://linkedin.com" + raw
	} else if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	if !strings.HasSuffix(host, "linkedin.com") {
		return ""
	}
	path := strings.ToLower(strings.Trim(u.Path, "/"))
	if path == "" {
		return ""
	}
	return "linkedin.com/" + path
}

// NormalizeDomain returns a bare lowercase hostname with the leading
// "www." stripped. Accepts bare hosts or full URLs.
func NormalizeDomain(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(u.Host, "www.")
}

// NormalizeName returns a tokens-sorted-alphabetically form suitable
// for order-insensitive trgm matching. "Musk Elon" and "Elon Musk"
// both produce "elon musk".
func NormalizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsSpace(r) || r == ',' || r == '.' || r == '-' || r == '_':
			b.WriteByte(' ')
		}
	}
	tokens := strings.Fields(b.String())
	if len(tokens) == 0 {
		return ""
	}
	sort.Strings(tokens)
	return strings.Join(tokens, " ")
}

// firstString returns the first non-empty string value under any of
// the given keys.
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch s := v.(type) {
		case string:
			if s = strings.TrimSpace(s); s != "" {
				return s
			}
		case fmt.Stringer:
			if ss := strings.TrimSpace(s.String()); ss != "" {
				return ss
			}
		}
	}
	return ""
}

// splitName returns (first, last). See resolver_test.go for the
// shapes it covers.
func splitName(full string, fields map[string]any) (first, last string) {
	if f := firstString(fields, "first_name"); f != "" {
		return f, firstString(fields, "last_name")
	}
	full = strings.TrimSpace(full)
	if full == "" {
		return "", ""
	}
	if i := strings.Index(full, ","); i != -1 {
		last = strings.TrimSpace(full[:i])
		first = strings.TrimSpace(strings.TrimRight(full[i+1:], "."))
		return first, last
	}
	parts := strings.Fields(full)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return strings.Join(parts[1:], " "), parts[0]
}

// ---------------------------------------------------------------------
// Deletion blocklist helpers (T16)
// ---------------------------------------------------------------------

// HashIdentifier returns the hex SHA-256 used in deletion_blocklist.
// Shared between the privacy/erasure handler (writes) and the
// resolver (reads) so both ends agree on the keying.
func HashIdentifier(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// blocklistHashes derives the SHA-256 hashes of every strong
// identifier in a source record (email + LinkedIn URL — name+company
// is intentionally excluded; see plan §T16). Used by the resolver to
// check if this record was previously erased.
func blocklistHashes(fields map[string]any, ids []identifier) []string {
	var out []string
	if email := strings.ToLower(strings.TrimSpace(firstString(fields, "email"))); email != "" {
		out = append(out, HashIdentifier(email))
	}
	for _, id := range ids {
		if id.typ == identifierLinkedIn {
			out = append(out, HashIdentifier(id.value))
		}
	}
	return out
}
