package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/ingest"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

// PrivacyHandler serves /api/v1/privacy/* — currently just the GDPR
// erasure endpoint. Plan §T16.
//
// The route is intentionally public (no Clerk auth). GDPR data
// subjects don't have accounts on our system; the appropriate way to
// authenticate them is an out-of-band confirmation flow (email link,
// signed token), which the MVP doesn't yet implement. For now,
// requests are taken at face value and logged for audit.
type PrivacyHandler struct {
	queries *repository.Queries
	pool    *pgxpool.Pool
}

func NewPrivacyHandler(q *repository.Queries, pool *pgxpool.Pool) *PrivacyHandler {
	return &PrivacyHandler{queries: q, pool: pool}
}

type erasureRequest struct {
	Email       string `json:"email"`
	LinkedInURL string `json:"linkedin_url"`
	Name        string `json:"name"`
	Company     string `json:"company"`
	Reason      string `json:"reason"`
}

type erasureResponse struct {
	DeletedPersons     int      `json:"deleted_persons"`
	BlockedIdentifiers int      `json:"blocked_identifiers"`
	BlockedTypes       []string `json:"blocked_types"`
}

const (
	identifierTypeEmail    = "email"
	identifierTypeLinkedIn = "linkedin"
)

// Erasure handles POST /api/v1/privacy/erasure.
//
// Flow per plan §T16:
//  1. Identify canonical person(s) via email / linkedin / name+company
//  2. Hash strong identifiers (email, linkedin) into deletion_blocklist
//     so the resolver refuses to recreate them on the next ingest
//  3. Hard-delete every matched person (CASCADE clears related rows)
//
// Name+company is used for *identification* but NOT hashed into the
// blocklist — name is too collision-prone to safely block future
// ingests of unrelated people who share the name.
func (h *PrivacyHandler) Erasure(w http.ResponseWriter, r *http.Request) {
	var req erasureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	linkedin := ingest.NormalizeLinkedInURL(req.LinkedInURL)
	name := ingest.NormalizeName(req.Name)
	company := strings.TrimSpace(req.Company)
	hasNameCompany := name != "" && company != ""

	if email == "" && linkedin == "" && !hasNameCompany {
		apierr.WriteError(w, apierr.APIError{
			Status:  http.StatusBadRequest,
			Code:    "no_identifier",
			Message: "at least one of email, linkedin_url, or (name + company) is required",
		})
		return
	}

	personIDs, blockedTypes, err := h.runErasure(r.Context(), email, linkedin, name, company, hasNameCompany, req.Reason)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{
			Status:  http.StatusInternalServerError,
			Code:    "erasure_failed",
			Message: err.Error(),
		})
		return
	}

	apierr.WriteJSON(w, http.StatusOK, erasureResponse{
		DeletedPersons:     len(personIDs),
		BlockedIdentifiers: len(blockedTypes),
		BlockedTypes:       blockedTypes,
	})
}

// runErasure resolves identifiers → person IDs, writes blocklist
// rows, and hard-deletes everything inside one tx. Returns the
// distinct deleted person IDs and the identifier types that landed
// on the blocklist (omits name+company per the design note above).
func (h *PrivacyHandler) runErasure(
	ctx context.Context,
	email, linkedin, name, company string,
	hasNameCompany bool,
	reason string,
) (personIDs []pgtype.UUID, blockedTypes []string, err error) {
	err = pgx.BeginFunc(ctx, h.pool, func(tx pgx.Tx) error {
		q := h.queries.WithTx(tx)

		seen := map[[16]byte]struct{}{}
		add := func(ids []pgtype.UUID) {
			for _, id := range ids {
				if _, ok := seen[id.Bytes]; !ok {
					seen[id.Bytes] = struct{}{}
					personIDs = append(personIDs, id)
				}
			}
		}

		if email != "" {
			ids, err := q.FindPersonIDsByEmail(ctx, email)
			if err != nil {
				return fmt.Errorf("find by email: %w", err)
			}
			add(ids)
			if err := q.InsertBlocklistEntry(ctx, repository.InsertBlocklistEntryParams{
				IdentifierType: identifierTypeEmail,
				IdentifierHash: ingest.HashIdentifier(email),
				Reason:         pgtype.Text{String: reason, Valid: reason != ""},
			}); err != nil {
				return fmt.Errorf("blocklist email: %w", err)
			}
			blockedTypes = append(blockedTypes, identifierTypeEmail)
		}

		if linkedin != "" {
			ids, err := q.FindPersonIDsByLinkedIn(ctx, linkedin)
			if err != nil {
				return fmt.Errorf("find by linkedin: %w", err)
			}
			add(ids)
			if err := q.InsertBlocklistEntry(ctx, repository.InsertBlocklistEntryParams{
				IdentifierType: identifierTypeLinkedIn,
				IdentifierHash: ingest.HashIdentifier(linkedin),
				Reason:         pgtype.Text{String: reason, Valid: reason != ""},
			}); err != nil {
				return fmt.Errorf("blocklist linkedin: %w", err)
			}
			blockedTypes = append(blockedTypes, identifierTypeLinkedIn)
		}

		if hasNameCompany {
			ids, err := q.FindPersonIDsByNormalizedNameAndOrg(ctx,
				repository.FindPersonIDsByNormalizedNameAndOrgParams{
					NormalizedName: name,
					Company:        company,
				})
			if err != nil {
				return fmt.Errorf("find by name+company: %w", err)
			}
			add(ids)
		}

		for _, id := range personIDs {
			if err := q.DeletePerson(ctx, id); err != nil {
				return fmt.Errorf("delete person %x: %w", id.Bytes, err)
			}
		}
		return nil
	})
	return personIDs, blockedTypes, err
}

