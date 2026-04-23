package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/leads"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

// HandleDiscoverLeads processes a lead discovery task.
func HandleDiscoverLeads(pipeline *leads.Pipeline, queries *repository.Queries) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var payload struct {
			TenantID   string `json:"tenant_id"`
			PlayID     string `json:"play_id"`
			CampaignID string `json:"campaign_id"`
		}
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshal payload: %w", err)
		}

		log.Printf("=== Starting lead discovery for campaign %s ===", payload.CampaignID)

		playID, _ := uuid.Parse(payload.PlayID)
		tenantID, _ := uuid.Parse(payload.TenantID)
		campaignID, _ := uuid.Parse(payload.CampaignID)

		// Get the play's search query
		play, err := queries.GetPlay(ctx, repository.GetPlayParams{
			ID:       pgtype.UUID{Bytes: playID, Valid: true},
			TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("get play: %w", err)
		}

		// Parse search query
		var searchQuery leads.DiscoverRequest
		if err := json.Unmarshal(play.SearchQuery, &searchQuery); err != nil {
			return fmt.Errorf("parse search query: %w", err)
		}

		// Use play name as industry hint if empty
		if searchQuery.Industry == "" {
			searchQuery.Industry = play.Name
		}

		// Get business context from tenant profile
		tenant, err := queries.GetTenant(ctx, pgtype.UUID{Bytes: tenantID, Valid: true})
		if err == nil && tenant.BusinessProfile != nil {
			searchQuery.BusinessContext = string(tenant.BusinessProfile)
		}

		log.Printf("Search: titles=%q industry=%q location=%q", searchQuery.Title, searchQuery.Industry, searchQuery.Location)

		// Run the discovery pipeline
		leadIDs, err := pipeline.DiscoverLeads(ctx, searchQuery)
		if err != nil {
			log.Printf("ERROR: %v", err)
			return fmt.Errorf("discover leads: %w", err)
		}

		log.Printf("Discovered %d leads with emails", len(leadIDs))

		// Add leads to campaign
		added := 0
		for _, leadID := range leadIDs {
			err := queries.AddLeadToCampaign(ctx, repository.AddLeadToCampaignParams{
				CampaignID: pgtype.UUID{Bytes: campaignID, Valid: true},
				LeadID:     leadID,
			})
			if err != nil {
				log.Printf("Failed to add lead: %v", err)
			} else {
				added++
			}
		}

		log.Printf("=== Discovery complete: %d leads added to campaign %s ===", added, payload.CampaignID)
		return nil
	}
}
