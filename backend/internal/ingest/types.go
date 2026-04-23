// Package ingest orchestrates pulling data from external sources into
// MagikLead's canonical lead database. Each Source describes how to fetch
// and parse one provider (SEC EDGAR, Wikidata, CrunchBase, …); the Runner
// uploads the raw payload to object storage, persists the parsed records,
// and calls a Resolver to reconcile them with the canonical tables.
//
// See docs/2026-04-21-lead-database-architecture/plan.md §T03 for the
// framework contract this package implements.
package ingest

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// Source is the contract every external data provider implements.
type Source interface {
	// Name returns a stable machine slug (e.g. "sec-edgar"). Used as the
	// S3 path prefix and the key in the `sources` table.
	Name() string

	// Type describes the provider shape — "api", "csv", "sparql", "scraped",
	// etc. Stored on the `sources` row for ops/debugging.
	Type() string

	// Fetch returns the batches to ingest, optionally restricted to
	// records changed after `since`. Each batch is one archival blob
	// (one CSV file, one API page, one XBRL document).
	Fetch(ctx context.Context, since time.Time) ([]RawBatch, error)

	// Parse turns a raw batch into structured records ready for entity
	// resolution. Implementations should be pure — no IO, no DB writes.
	Parse(ctx context.Context, raw RawBatch) ([]SourceRecord, error)
}

// RawBatch is one archival unit from a source. Holding `Content` as a
// byte slice keeps the runner simple: we hash it, upload it to S3, and
// hand it to Parse, all without rewinding a stream. Sources that pull
// multi-GB payloads (Common Crawl WARC files) will need a streaming
// variant; see T09 in the plan.
type RawBatch struct {
	FileName string
	Content  []byte
	// Meta is free-form provider metadata persisted on the raw_ingests
	// row (e.g. API cursor, filing accession number, crawl ID).
	Meta map[string]any
}

// SourceRecord is a parsed unit ready for entity resolution. `Fields`
// carries arbitrary provider-specific data; the resolver decides which
// canonical entities it describes.
type SourceRecord struct {
	// ExternalID is the source's stable ID for this record, if any
	// (SEC accession number, CrunchBase UUID, …). Empty for sources
	// that don't expose a stable ID.
	ExternalID string
	Fields     map[string]any
}

// ResolveInput bundles everything a Resolver needs to map one source
// record onto the canonical tables.
type ResolveInput struct {
	SourceID       pgtype.UUID
	SourceName     string
	SourceRecordID pgtype.UUID
	Record         SourceRecord
}

// Resolver reconciles source records with the canonical persons /
// organizations / employments tables and emits evidence rows.
//
// Implementations receive a tx-bound `*repository.Queries` so their
// writes participate in the batch's transaction — either the whole
// batch lands or none of it does.
type Resolver interface {
	Resolve(ctx context.Context, q *repository.Queries, in ResolveInput) error
}
