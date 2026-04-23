// Command ingest runs a single ingestion pass against one source.
//
//	go run ./cmd/ingest/... toy
//	go run ./cmd/ingest/... sec-edgar --since 2026-01-01
//
// Sources are registered in sourceRegistry below; add a case when a new
// provider lands (see docs/2026-04-21-lead-database-architecture/plan.md
// T04+ for the roadmap).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/jcleira/magiklead/backend/internal/ingest"
	"github.com/jcleira/magiklead/backend/internal/ingest/sources"
	"github.com/jcleira/magiklead/backend/internal/storage"
)

func main() {
	_ = godotenv.Load()

	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		printUsage()
		os.Exit(2)
	}
	sourceName := os.Args[1]

	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	since := fs.String("since", "", "only ingest records changed since this date (YYYY-MM-DD)")
	if err := fs.Parse(os.Args[2:]); err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	sinceTime := time.Time{}
	if *since != "" {
		t, err := time.Parse("2006-01-02", *since)
		if err != nil {
			log.Fatalf("invalid --since %q: %v", *since, err)
		}
		sinceTime = t
	}

	src, err := resolveSource(sourceName)
	if err != nil {
		log.Fatalf("%v", err)
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	store, err := storage.NewS3Storage(ctx)
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}

	runner := ingest.NewRunner(pool, store, ingest.NewDefaultResolver())
	result, err := runner.Run(ctx, src, sinceTime)
	if err != nil {
		log.Fatalf("run failed: %v", err)
	}

	log.Printf("ingest %s: fetched=%d ingested=%d skipped=%d records=%d errors=%d",
		result.Source,
		result.BatchesFetched,
		result.BatchesIngested,
		result.BatchesSkipped,
		result.RecordsIngested,
		len(result.Errors),
	)
	for _, e := range result.Errors {
		log.Printf("  - %v", e)
	}
	if len(result.Errors) > 0 {
		os.Exit(1)
	}
}

func resolveSource(name string) (ingest.Source, error) {
	switch name {
	case "toy":
		return sources.NewToySource(), nil
	case "sec-edgar":
		return sources.NewEdgarSource()
	case "wikidata":
		return sources.NewWikidataSource()
	case "crunchbase":
		return sources.NewCrunchbaseSource()
	case "teampages":
		return sources.NewTeamPageSource()
	default:
		return nil, fmt.Errorf("unknown source %q (known: toy, sec-edgar, wikidata, crunchbase, teampages)", name)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: ingest <source> [--since YYYY-MM-DD]")
	fmt.Fprintln(os.Stderr, "sources: toy, sec-edgar, wikidata, crunchbase, teampages")
}
