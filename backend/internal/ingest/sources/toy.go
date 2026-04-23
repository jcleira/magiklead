// Package sources holds one file per external data provider. The toy
// source is a stand-in used to exercise the ingestion framework
// end-to-end before any real provider (SEC EDGAR, Wikidata, …) is wired
// up — see T03 in docs/2026-04-21-lead-database-architecture/plan.md.
package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jcleira/magiklead/backend/internal/ingest"
)

// ToySource emits a deterministic JSON batch with a handful of fake
// execs. Deterministic so re-running the CLI exercises raw_ingests
// checksum dedup.
type ToySource struct{}

func NewToySource() *ToySource { return &ToySource{} }

func (ToySource) Name() string { return "toy" }
func (ToySource) Type() string { return "fixture" }

func (ToySource) Fetch(ctx context.Context, since time.Time) ([]ingest.RawBatch, error) {
	payload, err := json.Marshal([]map[string]any{
		{"name": "Alice Example", "title": "CEO", "company": "Example Co", "company_domain": "example.com"},
		{"name": "Bob Example", "title": "CTO", "company": "Example Co", "company_domain": "example.com"},
		{"name": "Carol Example", "title": "Head of Sales", "company": "Example Co"},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal toy payload: %w", err)
	}
	return []ingest.RawBatch{{
		FileName: "toy.json",
		Content:  payload,
		Meta:     map[string]any{"fixture": true},
	}}, nil
}

func (ToySource) Parse(ctx context.Context, raw ingest.RawBatch) ([]ingest.SourceRecord, error) {
	var rows []map[string]any
	if err := json.Unmarshal(raw.Content, &rows); err != nil {
		return nil, fmt.Errorf("unmarshal toy payload: %w", err)
	}
	records := make([]ingest.SourceRecord, 0, len(rows))
	for i, row := range rows {
		records = append(records, ingest.SourceRecord{
			ExternalID: fmt.Sprintf("toy-%d", i),
			Fields:     row,
		})
	}
	return records, nil
}
