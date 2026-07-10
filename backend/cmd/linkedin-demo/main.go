// Demo-mode LinkedIn prospect source. Scrapes company team/about pages
// (internal/leads/scraper.go — plain HTTP to the sites + Claude
// extraction; NO RapidAPI key) and writes the people it finds through
// the canonical person graph as LinkedIn prospects: a synthetic
// linkedin_url identifier per person, no email. This exercises the same
// write-through the production RapidAPI source uses, so a fresh devpod
// can demo LinkedIn search without any external LinkedIn key.
//
// Invoke from the api container:
//
//	devpods exec api go run ./cmd/linkedin-demo [domain ...]
//
// With no args it falls back to TEAMPAGES_DOMAINS, then anthropic.com.
package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/jcleira/magiklead/backend/internal/leads"
	linkedinsearch "github.com/jcleira/magiklead/backend/internal/leads/linkedin_search"
)

func main() {
	_ = godotenv.Load()
	ctx := context.Background()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required (the demo scraper extracts people with Claude)")
	}

	domains := os.Args[1:]
	if len(domains) == 0 {
		if env := strings.TrimSpace(os.Getenv("TEAMPAGES_DOMAINS")); env != "" {
			domains = strings.Split(env, ",")
		} else {
			domains = []string{"anthropic.com"}
		}
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	mod := linkedinsearch.New(pool, "", nil) // demo: no RapidAPI key

	total := 0
	for _, d := range domains {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		target := leads.CompanyTarget{Name: friendlyName(d), Domain: d}
		people, err := leads.ScrapeCompanyPeople(ctx, anthropicKey, target,
			"founder, ceo, head of, vp, director, engineer, marketing, sales")
		if err != nil {
			log.Printf("scrape %s: %v", d, err)
			continue
		}

		profiles := make([]linkedinsearch.Profile, 0, len(people))
		for _, p := range people {
			if strings.TrimSpace(p.Name) == "" {
				continue
			}
			profiles = append(profiles, linkedinsearch.Profile{
				FullName:    p.Name,
				Title:       p.Title,
				CompanyName: target.Name,
				Domain:      d,
				// The scraper yields names + titles, not profile links, so
				// synthesize a stable linkedin_url to key the canonical on.
				LinkedInURL: linkedinsearch.SyntheticURL(p.Name, d),
			})
		}

		written, err := mod.WriteThrough(ctx, profiles)
		if err != nil {
			log.Printf("write-through %s: %v", d, err)
			continue
		}
		log.Printf("%s: scraped %d people, wrote %d LinkedIn prospects", d, len(people), len(written))
		total += len(written)
	}

	log.Printf("done: %d LinkedIn prospects in the canonical (linkedin_url identifiers, no email)", total)
}

// friendlyName turns a bare domain into a presentable org name
// (anthropic.com → Anthropic).
func friendlyName(domain string) string {
	label := strings.SplitN(strings.TrimSpace(domain), ".", 2)[0]
	if label == "" {
		return domain
	}
	return strings.ToUpper(label[:1]) + label[1:]
}
