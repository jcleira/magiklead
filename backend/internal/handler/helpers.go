package handler

import (
	"context"

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
