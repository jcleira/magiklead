package handler

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

// ensureLeadsQuota returns an APIError suitable for apierr.WriteError
// when the tenant's leads_used has reached its leads_limit, and nil
// otherwise. Tenants with no subscription row behave like a fresh
// free plan (not blocked) — the Clerk webhook path creates the
// subscription but backfills may lag.
//
// Plan §T14: the gate is exact (>=), and the unused counter is NULL
// on legacy rows — treat NULL as 0.
func ensureLeadsQuota(ctx context.Context, q *repository.Queries, tenantID uuid.UUID) (apierr.APIError, bool) {
	sub, err := q.GetSubscription(ctx, pgUUID(tenantID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apierr.APIError{}, true
		}
		return apierr.APIError{Status: 500, Code: "quota_lookup_failed", Message: err.Error()}, false
	}
	used := int32(0)
	if sub.LeadsUsed.Valid {
		used = sub.LeadsUsed.Int32
	}
	if used >= sub.LeadsLimit {
		return apierr.ErrQuotaExceeded, false
	}
	return apierr.APIError{}, true
}

func ensureSequencesQuota(ctx context.Context, q *repository.Queries, tenantID uuid.UUID) (apierr.APIError, bool) {
	sub, err := q.GetSubscription(ctx, pgUUID(tenantID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apierr.APIError{}, true
		}
		return apierr.APIError{Status: 500, Code: "quota_lookup_failed", Message: err.Error()}, false
	}
	used := int32(0)
	if sub.SequencesUsed.Valid {
		used = sub.SequencesUsed.Int32
	}
	if used >= sub.SequencesLimit {
		return apierr.ErrQuotaExceeded, false
	}
	return apierr.APIError{}, true
}
