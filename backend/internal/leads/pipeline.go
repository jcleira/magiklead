package leads

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// TypeDiscoverLeads is the Asynq task type for lead discovery.
const TypeDiscoverLeads = "discover:leads"

// Pipeline orchestrates the full lead discovery flow:
// 1. AI picks target companies
// 2. Scrape company websites for people
// 3. Discover + verify emails
// 4. Store in DB
type Pipeline struct {
	queries      *repository.Queries
	anthropicKey string
}

func NewPipeline(queries *repository.Queries, anthropicKey string) *Pipeline {
	return &Pipeline{queries: queries, anthropicKey: anthropicKey}
}

type DiscoverRequest struct {
	Title           string `json:"title"`
	CompanySize     string `json:"company_size"`
	Industry        string `json:"industry"`
	Location        string `json:"location"`
	BusinessContext string `json:"business_context"`
}

// DiscoverLeads runs the full discovery pipeline.
func (p *Pipeline) DiscoverLeads(ctx context.Context, req DiscoverRequest) ([]pgtype.UUID, error) {
	queryHash := hashQuery(req)

	// Check cache
	cached, err := p.queries.GetCachedSearch(ctx, queryHash)
	if err == nil && cached.ResultLeadIds != nil {
		log.Printf("Cache hit for query %s: %d leads", queryHash[:8], len(cached.ResultLeadIds))
		return cached.ResultLeadIds, nil
	}

	// Step 1: AI picks target companies
	log.Printf("Step 1: AI company targeting...")
	companies, err := FindTargetCompanies(ctx, p.anthropicKey, req.BusinessContext, req.Title, req.Industry, req.Location)
	if err != nil {
		return nil, fmt.Errorf("company targeting: %w", err)
	}

	// Step 2: Scrape company websites for people
	log.Printf("Step 2: Scraping %d company websites...", len(companies))
	var allPeople []ScrapedPerson

	for _, company := range companies {
		if len(allPeople) >= 50 {
			break
		}

		people, err := ScrapeCompanyPeople(ctx, p.anthropicKey, company, req.Title)
		if err != nil {
			log.Printf("Scrape %s failed: %v", company.Domain, err)
			continue
		}

		allPeople = append(allPeople, people...)
		time.Sleep(1 * time.Second) // Be polite between sites
	}

	if len(allPeople) == 0 {
		return nil, fmt.Errorf("no people found on any company website")
	}

	log.Printf("Step 2 complete: found %d people across company websites", len(allPeople))

	// Step 3: Find + verify emails
	log.Printf("Step 3: Discovering emails...")

	// Group by domain and find emails per domain
	byDomain := map[string][]int{}
	for i, person := range allPeople {
		byDomain[person.Domain] = append(byDomain[person.Domain], i)
	}

	for domain, indices := range byDomain {
		// Collect people for this domain
		var domainPeople []ScrapedPerson
		for _, idx := range indices {
			domainPeople = append(domainPeople, allPeople[idx])
		}

		// Find emails (discovers pattern from first person, applies to rest)
		enriched := FindEmailsForDomain(domainPeople)

		// Write back
		for i, idx := range indices {
			if i < len(enriched) {
				allPeople[idx] = enriched[i]
			}
		}

		log.Printf("Email discovery for %s: %d people processed", domain, len(domainPeople))
		time.Sleep(500 * time.Millisecond)
	}

	// Count emails found
	withEmail := 0
	for _, p := range allPeople {
		if p.Email != "" {
			withEmail++
		}
	}
	log.Printf("Step 3 complete: %d/%d people have emails", withEmail, len(allPeople))

	// Step 4: Store in DB
	log.Printf("Step 4: Storing leads...")
	var leadIDs []pgtype.UUID

	for _, person := range allPeople {
		firstName, lastName := splitName(person.Name)
		if firstName == "" {
			continue
		}

		var linkedinURL pgtype.Text
		if person.LinkedIN != "" {
			linkedinURL = pgtype.Text{String: person.LinkedIN, Valid: true}
		}

		var emailText pgtype.Text
		if person.Email != "" {
			emailText = pgtype.Text{String: person.Email, Valid: true}
		}

		rawData, _ := json.Marshal(person)

		lead, err := p.queries.CreateLead(ctx, repository.CreateLeadParams{
			LinkedinUrl:   linkedinURL,
			FirstName:     firstName,
			LastName:      lastName,
			Title:         pgtype.Text{String: person.Title, Valid: person.Title != ""},
			Company:       pgtype.Text{String: person.Company, Valid: person.Company != ""},
			CompanyDomain: pgtype.Text{String: person.Domain, Valid: person.Domain != ""},
			Location:      pgtype.Text{Valid: false},
			Email:         emailText,
			EmailVerified: pgtype.Bool{Bool: person.EmailVerified, Valid: true},
			Source:        pgtype.Text{String: "website_scrape", Valid: true},
			RawData:       rawData,
		})
		if err != nil {
			log.Printf("Failed to store lead %s: %v", person.Name, err)
			continue
		}

		// Only add to campaign if they have a VERIFIED email
		if person.Email != "" && person.EmailVerified {
			leadIDs = append(leadIDs, lead.ID)
		}
	}

	// Cache results
	if len(leadIDs) > 0 {
		filtersJSON, _ := json.Marshal(req)
		p.queries.CreateLeadSearch(ctx, repository.CreateLeadSearchParams{
			QueryHash:     queryHash,
			Filters:       filtersJSON,
			ResultLeadIds: leadIDs,
		})
	}

	log.Printf("Discovery complete: %d leads with emails stored and linked to campaign", len(leadIDs))
	return leadIDs, nil
}

func hashQuery(req DiscoverRequest) string {
	normalized := fmt.Sprintf("%s|%s|%s|%s",
		strings.ToLower(strings.TrimSpace(req.Title)),
		strings.ToLower(strings.TrimSpace(req.CompanySize)),
		strings.ToLower(strings.TrimSpace(req.Industry)),
		strings.ToLower(strings.TrimSpace(req.Location)),
	)
	h := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", h)
}
