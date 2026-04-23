package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/storage"
)

// Runner drives a single ingestion pass for one source.
type Runner struct {
	pool     *pgxpool.Pool
	queries  *repository.Queries
	storage  storage.RawStorage
	resolver Resolver
}

// NewRunner wires the orchestrator with its collaborators.
func NewRunner(pool *pgxpool.Pool, store storage.RawStorage, resolver Resolver) *Runner {
	return &Runner{
		pool:     pool,
		queries:  repository.New(pool),
		storage:  store,
		resolver: resolver,
	}
}

// RunResult summarises what happened in one pass.
type RunResult struct {
	Source          string
	BatchesFetched  int
	BatchesSkipped  int
	BatchesIngested int
	RecordsIngested int
	Errors          []error
}

// Run fetches all batches from src that changed since `since`, uploads
// each batch to object storage, persists raw_ingests + source_records,
// and calls the resolver for every record. Batch-level errors are
// collected and returned so one bad batch doesn't abort the whole run.
func (r *Runner) Run(ctx context.Context, src Source, since time.Time) (*RunResult, error) {
	result := &RunResult{Source: src.Name()}

	sourceRow, err := r.upsertSource(ctx, src)
	if err != nil {
		return result, fmt.Errorf("upsert source: %w", err)
	}

	batches, err := src.Fetch(ctx, since)
	if err != nil {
		return result, fmt.Errorf("fetch: %w", err)
	}
	result.BatchesFetched = len(batches)

	for _, batch := range batches {
		ingested, records, err := r.runBatch(ctx, src, sourceRow.ID, batch)
		switch {
		case err != nil:
			log.Printf("ingest: batch %q failed: %v", batch.FileName, err)
			result.Errors = append(result.Errors, fmt.Errorf("batch %s: %w", batch.FileName, err))
		case !ingested:
			result.BatchesSkipped++
		default:
			result.BatchesIngested++
			result.RecordsIngested += records
		}
	}

	if err := r.queries.TouchSourceLastIngested(ctx, sourceRow.ID); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("touch source: %w", err))
	}

	return result, nil
}

// upsertSource looks up sources.name = src.Name() and inserts a row if
// missing. Avoids ON CONFLICT so the RETURNING projection works across
// both the found and created branches.
func (r *Runner) upsertSource(ctx context.Context, src Source) (repository.Source, error) {
	existing, err := r.queries.GetSourceByName(ctx, src.Name())
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return repository.Source{}, err
	}
	return r.queries.CreateSource(ctx, repository.CreateSourceParams{
		Name:        src.Name(),
		Type:        src.Type(),
		ApiEndpoint: pgtype.Text{Valid: false},
	})
}

// runBatch persists one batch end-to-end. Returns (ingested, recordCount,
// err): ingested is false when the batch was skipped as a duplicate.
func (r *Runner) runBatch(ctx context.Context, src Source, sourceID pgtype.UUID, batch RawBatch) (bool, int, error) {
	// 1. Checksum the payload and skip if we've seen an identical one.
	//    Dedup at the raw_ingests level keeps re-runs idempotent and
	//    makes retries safe after a transient failure.
	sum := sha256.Sum256(batch.Content)
	checksum := hex.EncodeToString(sum[:])

	existing, err := r.queries.GetRawIngestByChecksum(ctx, repository.GetRawIngestByChecksumParams{
		SourceID: sourceID,
		Checksum: checksum,
	})
	if err == nil && existing.ID.Valid {
		log.Printf("ingest: skip duplicate batch %s (checksum=%s)", batch.FileName, checksum[:12])
		return false, 0, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, 0, fmt.Errorf("dedup check: %w", err)
	}

	// 2. Parse before uploading so we fail fast on malformed batches
	//    without leaving orphan objects in S3.
	records, err := src.Parse(ctx, batch)
	if err != nil {
		return false, 0, fmt.Errorf("parse: %w", err)
	}

	// 3. Upload the raw payload. Done outside the DB transaction because
	//    S3 IO shouldn't hold Postgres locks.
	url, err := r.storage.Upload(ctx, src.Name(), batch.FileName, bytes.NewReader(batch.Content))
	if err != nil {
		return false, 0, fmt.Errorf("upload: %w", err)
	}

	metaJSON, err := marshalJSON(batch.Meta)
	if err != nil {
		return false, 0, fmt.Errorf("marshal meta: %w", err)
	}

	// 4. Persist raw_ingest + source_records + resolver writes in one tx.
	//    Either the whole batch lands or none of it does.
	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.queries.WithTx(tx)

		ingest, err := q.CreateRawIngest(ctx, repository.CreateRawIngestParams{
			SourceID:  sourceID,
			FileUrl:   url,
			Checksum:  checksum,
			SizeBytes: int64(len(batch.Content)),
			Meta:      metaJSON,
		})
		if err != nil {
			return fmt.Errorf("create raw_ingest: %w", err)
		}

		for _, rec := range records {
			fieldsJSON, err := marshalJSON(rec.Fields)
			if err != nil {
				return fmt.Errorf("marshal fields: %w", err)
			}
			sr, err := q.CreateSourceRecord(ctx, repository.CreateSourceRecordParams{
				SourceID:    sourceID,
				RawIngestID: ingest.ID,
				ExternalID:  pgtype.Text{String: rec.ExternalID, Valid: rec.ExternalID != ""},
				Fields:      fieldsJSON,
			})
			if err != nil {
				return fmt.Errorf("create source_record: %w", err)
			}

			if err := r.resolver.Resolve(ctx, q, ResolveInput{
				SourceID:       sourceID,
				SourceName:     src.Name(),
				SourceRecordID: sr.ID,
				Record:         rec,
			}); err != nil {
				return fmt.Errorf("resolve: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return false, 0, err
	}

	return true, len(records), nil
}

// marshalJSON returns a JSONB-safe payload. An empty map marshals to
// "{}" rather than "null" so the column default holds.
func marshalJSON(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}
