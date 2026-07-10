package handler

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/middleware"
)

// getTenantID extracts the tenant ID from the request context.
func getTenantID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(middleware.TenantIDKey).(uuid.UUID)
	return id
}

// pgUUID converts a google/uuid.UUID to pgtype.UUID.
func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil}
}

// tsOrZero converts a nullable Postgres timestamp to a time.Time, using
// the zero time when the column is NULL — the form the pacer reads as
// "no window/anchor set."
func tsOrZero(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}
