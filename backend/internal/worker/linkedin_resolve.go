package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

// memberIDIdentifierType is the person_identifiers type under which a
// prospect's resolved Unipile member id (the ACoAA… key every LinkedIn send
// is addressed by) is cached, permanently, on the person. The existing
// 'linkedin_url' identifier is the resolution input; this is the output.
const memberIDIdentifierType = "linkedin_member_id"

// resolveMemberIDFunc turns a prospect's LinkedIn profile URL into their
// Unipile member id through a given connected account, or returns a
// classified error (unipile.IsPermanentResolveError separates the permanent
// "this profile will never resolve" case from transient upstream trouble).
// It is the seam onto unipile.Module.ResolveMemberID, faked in tests.
type resolveMemberIDFunc func(ctx context.Context, profileURL, accountID string) (string, error)

// resolveInput is the minimal lead context the member-id resolver needs: the
// campaign_lead it acts for (to fail terminally / attribute an event), the
// person to cache against, the profile URL to resolve, the Unipile account to
// resolve through, and the step the lead sits at (for event attribution).
type resolveInput struct {
	CampaignLeadID pgtype.UUID
	PersonID       pgtype.UUID
	LinkedinURL    string
	AccountID      string
	CurrentStep    int32
}

// resolveMemberID is a read-through cache from a prospect's LinkedIn profile
// URL to their Unipile member id (issue #5). It checks the person's cached
// linkedin_member_id first and returns it without touching Unipile on a hit;
// on a miss it calls Unipile exactly once and persists the result, so the
// same person is never resolved twice, even across campaigns.
//
// The bool reports whether the caller has a member id to send with. false
// means skip this lead this tick — and the resolver has already applied the
// right side effect: on a transient failure nothing is written and the lead
// stays queued for the next tick (no failure event, no pacing consumed); on a
// permanent failure a 'failed' linkedin_event is recorded and the lead is
// moved to a terminal 'failed' status so it is never retried.
func resolveMemberID(ctx context.Context, q *repository.Queries, resolve resolveMemberIDFunc, in resolveInput) (string, bool) {
	// Cache read: a hit returns immediately, no upstream call.
	cached, err := q.GetPersonIdentifier(ctx, repository.GetPersonIdentifierParams{
		PersonID:       in.PersonID,
		IdentifierType: memberIDIdentifierType,
	})
	if err == nil && cached != "" {
		return cached, true
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		// A read error is infrastructure trouble, not an unresolvable
		// prospect — treat it as transient and retry next tick.
		log.Printf("worker: member-id cache read for lead %x: %v", in.CampaignLeadID.Bytes, err)
		return "", false
	}

	// Miss: resolve upstream exactly once.
	memberID, err := resolve(ctx, in.LinkedinURL, in.AccountID)
	if err != nil {
		if unipile.IsPermanentResolveError(err) {
			meta, _ := json.Marshal(map[string]string{"reason": "member_id_unresolved", "error": err.Error()})
			if _, evErr := q.CreateLinkedInEvent(ctx, repository.CreateLinkedInEventParams{
				CampaignLeadID: in.CampaignLeadID,
				EventType:      "failed",
				Step:           in.CurrentStep,
				Metadata:       meta,
			}); evErr != nil {
				log.Printf("worker: record resolve-failed event for lead %x: %v", in.CampaignLeadID.Bytes, evErr)
			}
			if failErr := q.MarkLinkedInFailed(ctx, in.CampaignLeadID); failErr != nil {
				log.Printf("worker: mark lead %x failed: %v", in.CampaignLeadID.Bytes, failErr)
			}
			return "", false
		}
		// Transient: leave the lead queued, write nothing, consume no pacing.
		log.Printf("worker: transient member-id resolve for lead %x: %v", in.CampaignLeadID.Bytes, err)
		return "", false
	}

	// Persist-once so the same person is never resolved again.
	if err := q.CreatePersonIdentifier(ctx, repository.CreatePersonIdentifierParams{
		PersonID:        in.PersonID,
		IdentifierType:  memberIDIdentifierType,
		IdentifierValue: memberID,
		IsPrimary:       pgtype.Bool{Bool: false, Valid: true},
	}); err != nil {
		// The member id resolved fine; only the cache write failed. Return it
		// so this tick still proceeds — a later miss will persist again.
		log.Printf("worker: persist member id for person %x: %v", in.PersonID.Bytes, err)
	}
	return memberID, true
}
