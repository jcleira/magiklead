package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/email"
	"github.com/jcleira/magiklead/backend/internal/ingest"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

// PrivacyHandler serves /api/v1/privacy/* — GDPR erasure with an
// email-based confirmation flow so identity verification happens
// out-of-band. Plan §T15.
type PrivacyHandler struct {
	queries     *repository.Queries
	pool        *pgxpool.Pool
	mailer      *email.Sender
	frontendURL string
}

func NewPrivacyHandler(q *repository.Queries, pool *pgxpool.Pool, mailer *email.Sender, frontendURL string) *PrivacyHandler {
	return &PrivacyHandler{queries: q, pool: pool, mailer: mailer, frontendURL: frontendURL}
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
	tokenByteLength        = 32
	tokenValidity          = 24 * time.Hour
)

// RequestErasure handles POST /api/v1/privacy/erasure/request.
//
// Validates that at least one identifier is present, stores a hashed
// token with 24h expiry, and mails the raw token as a confirmation
// link. The erasure itself does NOT run here — it waits for the
// caller to click the link. Always returns 202 so that whether an
// email exists in our records isn't leaked through the response.
func (h *PrivacyHandler) RequestErasure(w http.ResponseWriter, r *http.Request) {
	var req erasureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	linkedin := ingest.NormalizeLinkedInURL(req.LinkedInURL)
	name := ingest.NormalizeName(req.Name)
	company := strings.TrimSpace(req.Company)

	if email == "" && linkedin == "" && !(name != "" && company != "") {
		apierr.WriteError(w, apierr.APIError{
			Status:  http.StatusBadRequest,
			Code:    "no_identifier",
			Message: "at least one of email, linkedin_url, or (name + company) is required",
		})
		return
	}
	// Without an email there's no way to send the confirmation link;
	// the public form must collect one even when name/linkedin are the
	// actual matchers.
	if email == "" {
		apierr.WriteError(w, apierr.APIError{
			Status:  http.StatusBadRequest,
			Code:    "email_required",
			Message: "email is required so we can deliver the confirmation link",
		})
		return
	}

	rawToken, tokenHash, err := generateToken()
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "token_failed", Message: err.Error()})
		return
	}

	_, err = h.queries.InsertPrivacyRequest(r.Context(), repository.InsertPrivacyRequestParams{
		TokenHash:   tokenHash,
		Email:       pgtype.Text{String: email, Valid: true},
		LinkedinUrl: pgtype.Text{String: linkedin, Valid: linkedin != ""},
		Name:        pgtype.Text{String: name, Valid: name != ""},
		Company:     pgtype.Text{String: company, Valid: company != ""},
		Reason:      pgtype.Text{String: req.Reason, Valid: req.Reason != ""},
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(tokenValidity), Valid: true},
	})
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "request_failed", Message: err.Error()})
		return
	}

	confirmURL := h.buildConfirmURL(rawToken)
	body := fmt.Sprintf(
		"We received a data erasure request. If this was you, confirm the deletion by clicking the link below. The link expires in 24 hours.\n\n%s\n\nIf you didn't request this, you can ignore this email — no data will be deleted.",
		confirmURL,
	)
	if err := h.mailer.Send(email, "Confirm your MagikLead data erasure", body); err != nil {
		// Log but don't leak — the row is in the DB and can be resent
		// via an admin tool if needed.
		log.Printf("privacy: email send failed for %s: %v", email, err)
	}

	apierr.WriteJSON(w, http.StatusAccepted, map[string]string{
		"status": "pending_confirmation",
	})
}

// ConfirmErasure handles POST /api/v1/privacy/erasure/confirm?token=…
//
// Looks up the pending request by token hash, runs the same erasure
// logic the old unauthenticated endpoint ran, and marks the request
// confirmed. Tokens are single-use: already-confirmed rows return 410.
func (h *PrivacyHandler) ConfirmErasure(w http.ResponseWriter, r *http.Request) {
	rawToken := strings.TrimSpace(r.URL.Query().Get("token"))
	if rawToken == "" {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "token_required", Message: "missing token"})
		return
	}
	tokenHash := hashToken(rawToken)

	pr, err := h.queries.GetPrivacyRequestByTokenHash(r.Context(), tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apierr.WriteError(w, apierr.APIError{Status: 404, Code: "invalid_token", Message: "token not found"})
			return
		}
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "confirm_failed", Message: err.Error()})
		return
	}
	if pr.ConfirmedAt.Valid {
		apierr.WriteError(w, apierr.APIError{Status: 410, Code: "already_confirmed", Message: "this request has already been processed"})
		return
	}
	if pr.ExpiresAt.Time.Before(time.Now()) {
		apierr.WriteError(w, apierr.APIError{Status: 410, Code: "token_expired", Message: "token expired; submit a new request"})
		return
	}

	email := ""
	if pr.Email.Valid {
		email = pr.Email.String
	}
	linkedin := ""
	if pr.LinkedinUrl.Valid {
		linkedin = pr.LinkedinUrl.String
	}
	name := ""
	if pr.Name.Valid {
		name = pr.Name.String
	}
	company := ""
	if pr.Company.Valid {
		company = pr.Company.String
	}
	reason := ""
	if pr.Reason.Valid {
		reason = pr.Reason.String
	}
	hasNameCompany := name != "" && company != ""

	personIDs, blockedTypes, err := h.runErasure(r.Context(), email, linkedin, name, company, hasNameCompany, reason)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "erasure_failed", Message: err.Error()})
		return
	}

	if err := h.queries.ConfirmPrivacyRequest(r.Context(), repository.ConfirmPrivacyRequestParams{
		ID:             pr.ID,
		DeletedPersons: pgtype.Int4{Int32: int32(len(personIDs)), Valid: true},
	}); err != nil {
		log.Printf("privacy: confirm update failed for %x: %v", pr.ID.Bytes, err)
	}

	apierr.WriteJSON(w, http.StatusOK, erasureResponse{
		DeletedPersons:     len(personIDs),
		BlockedIdentifiers: len(blockedTypes),
		BlockedTypes:       blockedTypes,
	})
}

func (h *PrivacyHandler) buildConfirmURL(rawToken string) string {
	base := strings.TrimRight(h.frontendURL, "/")
	if base == "" {
		base = "http://localhost:3000"
	}
	return fmt.Sprintf("%s/privacy/erasure/confirm?token=%s", base, rawToken)
}

func generateToken() (raw string, hashed []byte, err error) {
	buf := make([]byte, tokenByteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) []byte {
	h := sha256.Sum256([]byte(raw))
	return h[:]
}

// runErasure resolves identifiers → person IDs, writes blocklist
// rows, and hard-deletes everything inside one tx. Returns the
// distinct deleted person IDs and the identifier types that landed
// on the blocklist (omits name+company because name is too
// collision-prone to safely block unrelated future ingests).
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
