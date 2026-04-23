// Command verify-emails runs the SMTP verification pipeline over every
// `emails` row that still has verified_at = NULL. It never invents
// addresses — pattern-guessing is banned (research.md §3). Emails
// enter the queue when a source lands one, and this command resolves
// them.
//
//	go run ./cmd/verify-emails --limit 100
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/jcleira/magiklead/backend/internal/leads"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

func main() {
	_ = godotenv.Load()

	limit := flag.Int("limit", 100, "max unverified emails to process this run")
	flag.Parse()

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	helo := envDefault("SMTP_HELO", "magiklead.local")
	from := envDefault("SMTP_MAIL_FROM", "verify@"+helo)

	q := repository.New(pool)
	v := leads.NewSMTPVerifier(q, helo, from)

	emails, err := q.ListUnverifiedEmails(ctx, int32(*limit))
	if err != nil {
		log.Fatalf("list unverified: %v", err)
	}
	log.Printf("verifying %d emails (helo=%s from=%s)", len(emails), helo, from)

	var verified, catchall, bounced, errored int
	for _, e := range emails {
		res, err := v.Verify(ctx, e.Email)
		if err != nil {
			log.Printf("  %s: error: %v", e.Email, err)
			errored++
			continue
		}

		bounce := int32(0)
		if e.BounceCount.Valid {
			bounce = e.BounceCount.Int32
		}
		if !res.Verified && !res.IsCatchAll && res.Method == "smtp-rcpt" {
			bounce++
			bounced++
		}
		if res.Verified {
			verified++
		}
		if res.IsCatchAll {
			catchall++
		}

		if err := q.UpdateEmailVerification(ctx, repository.UpdateEmailVerificationParams{
			ID:                 e.ID,
			VerificationMethod: pgtype.Text{String: res.Method, Valid: true},
			BounceCount:        pgtype.Int4{Int32: bounce, Valid: true},
			IsCatchall:         pgtype.Bool{Bool: res.IsCatchAll, Valid: true},
		}); err != nil {
			log.Printf("  %s: update failed: %v", e.Email, err)
			errored++
			continue
		}
		log.Printf("  %s: verified=%v method=%s catchall=%v", e.Email, res.Verified, res.Method, res.IsCatchAll)
	}

	log.Printf("done: verified=%d catchall=%d bounced=%d errored=%d total=%d",
		verified, catchall, bounced, errored, len(emails))
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
