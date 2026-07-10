package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/linkedin/pacer"
	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

// LinkedInSendFunc is the seam between the worker and unipile.SendInvitation
// — the LinkedIn analogue of SendFunc. Keeping it a function shape makes
// the test fake one line; production wires unipile.Module.SendInvitation.
type LinkedInSendFunc func(ctx context.Context, p unipile.InviteParams) (*unipile.InviteResult, error)

// LinkedInMessageFunc is the seam between the worker and
// unipile.SendMessage — the DM analogue of LinkedInSendFunc. Production
// wires unipile.Module.SendMessage; the DM tick (issue #5) uses it to send
// the first message once an invite is accepted.
type LinkedInMessageFunc func(ctx context.Context, p unipile.MessageParams) (*unipile.MessageResult, error)

// StartLinkedInLoop runs every 60 seconds and drives the LinkedIn
// sequence: sendInvite delivers paced step-0 connection invites for leads
// sitting at step 0 (issue #4); sendMessage delivers the direct messages
// for leads that have since been accepted and advanced past step 0 (issue
// #5). Both queues run each tick. resolve turns a prospect's profile URL
// into the Unipile member id every send is addressed by (issue #6), a
// read-through cache on the person (issue #5). limits is the invite
// capacity policy — pacer.Standard() in production (issue #7 swaps in a
// warmup ramp); DMs to already-connected prospects are not invite-capped.
func StartLinkedInLoop(ctx context.Context, queries *repository.Queries, supp *suppression.Module, sendInvite LinkedInSendFunc, sendMessage LinkedInMessageFunc, resolve resolveMemberIDFunc, limits pacer.Limits) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	log.Println("LinkedIn loop started (checking every 60s)")

	processLinkedInQueue(ctx, queries, sendInvite, resolve, limits)
	processLinkedInDMQueue(ctx, queries, supp, sendMessage, resolve)

	for {
		select {
		case <-ctx.Done():
			log.Println("LinkedIn loop stopping")
			return
		case <-ticker.C:
			processLinkedInQueue(ctx, queries, sendInvite, resolve, limits)
			processLinkedInDMQueue(ctx, queries, supp, sendMessage, resolve)
		}
	}
}

func processLinkedInQueue(ctx context.Context, queries *repository.Queries, send LinkedInSendFunc, resolve resolveMemberIDFunc, limits pacer.Limits) {
	dueLeads, err := queries.GetDueLinkedInInviteLeads(ctx, 10)
	if err != nil {
		log.Printf("LinkedIn loop: error getting due leads: %v", err)
		return
	}
	if len(dueLeads) == 0 {
		return
	}

	log.Printf("LinkedIn loop: %d leads due for invite", len(dueLeads))

	// Per-tick cache of each account's trailing-7-day acceptance signal so
	// the breaker query runs once per account even when several leads share
	// it, and the pause/resume flag is applied at most once per tick.
	acceptance := map[string]pacer.Counters{}

	for _, lead := range dueLeads {
		// Can't invite a prospect we have no LinkedIn identifier for. This
		// shouldn't happen for LinkedIn-sourced persons, but the canonical
		// graph can hold persons from other sources too.
		if lead.LinkedinUrl == "" {
			log.Printf("LinkedIn loop: lead %s has no linkedin_url — skipping", fmtID(lead.ID))
			continue
		}

		campaign, err := getCampaignForLead(ctx, queries, lead.CampaignID)
		if err != nil {
			log.Printf("LinkedIn loop: can't get campaign for lead %s: %v", fmtID(lead.ID), err)
			continue
		}

		var sequence []LinkedinStep
		if err := json.Unmarshal(campaign.LinkedinSequence, &sequence); err != nil || len(sequence) == 0 {
			log.Printf("LinkedIn loop: lead %s campaign has no usable linkedin_sequence", fmtID(lead.ID))
			continue
		}
		// Step 0 is positional: the connection-request note (issue #3
		// validates this shape at authoring time).
		note := sequence[0]

		// Pick the tenant's connected account. Free tier = one account per
		// workspace. Re-read per lead so its counters reflect invites
		// already sent earlier in this same tick.
		account, ok := pickLinkedInAccount(ctx, queries, lead.TenantID)
		if !ok {
			log.Printf("LinkedIn loop: no usable LinkedIn account for tenant %s", fmtID(lead.TenantID))
			continue
		}

		now := time.Now()

		// Warmup anchor (issue #7): a connected account lands without one
		// (issue #1 sets it active directly), so the first invite starts its
		// ramp clock. COALESCE in the query fixes it once and never resets;
		// we mirror it locally so this tick's allowance already sees week 1.
		if !account.WarmupStartedAt.Valid {
			if err := queries.StartLinkedInWarmup(ctx, account.ID); err != nil {
				log.Printf("LinkedIn loop: start warmup for account %s: %v", fmtID(account.ID), err)
			} else {
				account.WarmupStartedAt = pgtype.Timestamptz{Time: now, Valid: true}
			}
		}

		// Acceptance-rate breaker (issue #7): fold the account's trailing-
		// 7-day accept signal into its counters and, once per tick, flip the
		// paused flag if the breaker's verdict changed so the UI can surface
		// it. A breached account's Allowance returns 0 in the gate below.
		counters := toCounters(account)
		accID := fmtID(account.ID)
		stat, seen := acceptance[accID]
		if !seen {
			stat = acceptanceCounters(ctx, queries, account.ID)
			acceptance[accID] = stat
			applyAcceptanceBreaker(ctx, queries, account, stat)
		}
		counters.InvitesSent7d = stat.InvitesSent7d
		counters.Accepted7d = stat.Accepted7d

		// Resolve the prospect's Unipile member id before the pacing gate —
		// every LinkedIn send is addressed by member id, not profile URL
		// (issue #6). Read-through cached on the person (issue #5): a hit is a
		// local lookup; a miss calls Unipile once and persists. A false return
		// means skip this lead this tick — the resolver has already left it
		// queued (transient upstream trouble) or failed it terminally (an
		// unresolvable profile), so no pacing allowance is consumed either way.
		memberID, ok := resolveMemberID(ctx, queries, resolve, resolveInput{
			CampaignLeadID: lead.ID,
			PersonID:       lead.PersonID,
			LinkedinURL:    lead.LinkedinUrl,
			AccountID:      account.UnipileAccountID,
			CurrentStep:    lead.CurrentStep.Int32,
		})
		if !ok {
			continue
		}

		// Pacing gate. An exhausted budget — or a tripped acceptance breaker
		// — simply sends fewer this tick; the lead stays queued and is
		// retried next tick. Nothing over-sends because each successful send
		// increments the stored counters that the next iteration's
		// pickLinkedInAccount re-reads.
		if allowance := limits.Allowance(counters, now); allowance <= 0 {
			log.Printf("LinkedIn loop: account %s at capacity this tick — leaving lead %s queued", fmtID(account.ID), fmtID(lead.ID))
			continue
		}

		title := ""
		if lead.Title.Valid {
			title = lead.Title.String
		}
		body := RenderLinkedinStep(note, lead.FirstName, lead.LastName, lead.Company, title)

		result, sendErr := send(ctx, unipile.InviteParams{
			AccountID: account.UnipileAccountID,
			Recipient: memberID,
			Note:      body,
		})
		if sendErr != nil {
			reason := classifyLinkedInSendError(sendErr)
			log.Printf("LinkedIn invite FAILED for lead %s: %v (reason=%s)", fmtID(lead.ID), sendErr, reason)
			meta, _ := json.Marshal(map[string]string{
				"reason": reason,
				"error":  sendErr.Error(),
			})
			queries.CreateLinkedInEvent(ctx, repository.CreateLinkedInEventParams{
				CampaignLeadID: lead.ID,
				EventType:      "failed",
				Step:           lead.CurrentStep.Int32,
				Metadata:       meta,
			})
			// A restricted sentinel escalates the account to 'restricted'
			// (issue #8) so Settings can prompt a reconnect and the rest of
			// this tick's pickLinkedInAccount skips it. The lead itself is
			// not advanced — it stays queued and resumes once the account is
			// healthy again.
			escalateRestriction(ctx, queries, account.UnipileAccountID, sendErr)
			continue
		}

		log.Printf("LinkedIn invite sent for lead %s (invitation=%s)", fmtID(lead.ID), result.InvitationID)

		queries.CreateLinkedInEvent(ctx, repository.CreateLinkedInEventParams{
			CampaignLeadID:   lead.ID,
			EventType:        "invite_sent",
			Step:             lead.CurrentStep.Int32,
			UnipileMessageID: pgText(result.InvitationID),
		})
		queries.MarkLinkedInInviteSent(ctx, repository.MarkLinkedInInviteSentParams{
			ID:                   lead.ID,
			LinkedinAccountID:    account.ID,
			LinkedinInvitationID: pgText(result.InvitationID),
		})
		queries.IncrementLinkedInInviteCounters(ctx, account.ID)
	}
}

// processLinkedInDMQueue sends the next direct message for accepted leads
// (issue #5). GetDueLinkedInDMLeads returns leads that have moved past the
// step-0 invite — active, due, and bound to the account that sent their
// invite — so each DM goes from that same account into the prospect's
// chat. The first DM starts the chat; on success we record a dm_sent event
// with the returned message + chat ids and advance the lead to the next
// step's jittered delay (or exhaust it), storing the chat id for follow-ups
// (issue #6). A send failure writes a classified failed event and leaves
// the lead where it is for a retry next tick, mirroring the invite path.
func processLinkedInDMQueue(ctx context.Context, queries *repository.Queries, supp *suppression.Module, send LinkedInMessageFunc, resolve resolveMemberIDFunc) {
	dueLeads, err := queries.GetDueLinkedInDMLeads(ctx, 10)
	if err != nil {
		log.Printf("LinkedIn DM loop: error getting due leads: %v", err)
		return
	}
	if len(dueLeads) == 0 {
		return
	}

	log.Printf("LinkedIn DM loop: %d leads due for a message", len(dueLeads))

	for _, lead := range dueLeads {
		if lead.LinkedinUrl == "" {
			log.Printf("LinkedIn DM loop: lead %s has no linkedin_url — skipping", fmtID(lead.ID))
			continue
		}

		// Person-suppression gate (issue #6): never DM a person who replied
		// or unsubscribed. GetDueLinkedInDMLeads already excludes this lead's
		// own replied state via the status filter, so this catches
		// person-keyed suppression that landed through a *different*
		// campaign_lead — the zero-DMs-to-replied invariant across campaigns.
		// On a hit we write a 'skipped' event and halt the lead (suppressed),
		// mirroring the email sender's gate.
		if lead.PersonID.Valid {
			suppressed, reason, err := supp.IsSuppressedPerson(ctx, uuid.UUID(lead.TenantID.Bytes), uuid.UUID(lead.PersonID.Bytes))
			if err != nil {
				log.Printf("LinkedIn DM loop: suppression check failed for lead %s: %v — skipping this tick", fmtID(lead.ID), err)
				continue
			}
			if suppressed {
				meta, _ := json.Marshal(map[string]string{"reason": reason})
				queries.CreateLinkedInEvent(ctx, repository.CreateLinkedInEventParams{
					CampaignLeadID: lead.ID,
					EventType:      "skipped",
					Step:           lead.CurrentStep.Int32,
					Metadata:       meta,
				})
				queries.SuppressLinkedInLead(ctx, lead.ID)
				log.Printf("LinkedIn DM loop: skipped lead %s (%s)", fmtID(lead.ID), reason)
				continue
			}
		}

		campaign, err := getCampaignForLead(ctx, queries, lead.CampaignID)
		if err != nil {
			log.Printf("LinkedIn DM loop: can't get campaign for lead %s: %v", fmtID(lead.ID), err)
			continue
		}

		var sequence []LinkedinStep
		if err := json.Unmarshal(campaign.LinkedinSequence, &sequence); err != nil || len(sequence) == 0 {
			log.Printf("LinkedIn DM loop: lead %s campaign has no usable linkedin_sequence", fmtID(lead.ID))
			continue
		}

		currentStep := int(lead.CurrentStep.Int32)
		if currentStep >= len(sequence) {
			// Past the last step — exhaust so the tick stops selecting it.
			queries.AdvanceLinkedInDMStep(ctx, repository.AdvanceLinkedInDMStepParams{
				ID:          lead.ID,
				CurrentStep: lead.CurrentStep,
				Status:      pgtype.Text{String: "exhausted", Valid: true},
			})
			continue
		}
		step := sequence[currentStep]

		title := ""
		if lead.Title.Valid {
			title = lead.Title.String
		}
		body := RenderLinkedinStep(step, lead.FirstName, lead.LastName, lead.Company, title)

		// Resolve the prospect's member id — reused from the cache the invite
		// populated, so this is a local lookup with no second Unipile call
		// (issue #6). A false return skips the lead this tick (the resolver
		// leaves it queued on transient trouble or fails it on a permanent
		// one); the DM path has no pacing gate, so nothing is consumed.
		memberID, ok := resolveMemberID(ctx, queries, resolve, resolveInput{
			CampaignLeadID: lead.ID,
			PersonID:       lead.PersonID,
			LinkedinURL:    lead.LinkedinUrl,
			AccountID:      lead.UnipileAccountID,
			CurrentStep:    int32(currentStep),
		})
		if !ok {
			continue
		}

		result, sendErr := send(ctx, unipile.MessageParams{
			AccountID: lead.UnipileAccountID,
			Recipient: memberID,
			Text:      body,
		})
		if sendErr != nil {
			reason := classifyLinkedInSendError(sendErr)
			log.Printf("LinkedIn DM FAILED for lead %s: %v (reason=%s)", fmtID(lead.ID), sendErr, reason)
			meta, _ := json.Marshal(map[string]string{
				"reason": reason,
				"error":  sendErr.Error(),
			})
			queries.CreateLinkedInEvent(ctx, repository.CreateLinkedInEventParams{
				CampaignLeadID: lead.ID,
				EventType:      "failed",
				Step:           int32(currentStep),
				Metadata:       meta,
			})
			// Lead not advanced — retried next tick. A restricted sentinel
			// also escalates the account to 'restricted' (issue #8), which
			// drops its in-flight DM leads out of GetDueLinkedInDMLeads until
			// it is reconnected.
			escalateRestriction(ctx, queries, lead.UnipileAccountID, sendErr)
			continue
		}

		log.Printf("LinkedIn DM sent for lead %s (message=%s chat=%s)", fmtID(lead.ID), result.MessageID, result.ChatID)

		queries.CreateLinkedInEvent(ctx, repository.CreateLinkedInEventParams{
			CampaignLeadID:   lead.ID,
			EventType:        "dm_sent",
			Step:             int32(currentStep),
			UnipileMessageID: pgText(result.MessageID),
			UnipileChatID:    pgText(result.ChatID),
		})

		// Advance to the next step's jittered delay, or exhaust the
		// sequence. Mirrors the email sender's advance (jitter ±2h).
		nextStep := currentStep + 1
		var nextSendAt pgtype.Timestamptz
		var nextStatus pgtype.Text
		if nextStep >= len(sequence) {
			nextStatus = pgtype.Text{String: "exhausted", Valid: true}
		} else {
			delayDays := sequence[nextStep].DelayDays
			if delayDays < 1 {
				delayDays = 1
			}
			jitter := time.Duration(rand.Intn(4)-2) * time.Hour
			nextTime := time.Now().Add(time.Duration(delayDays)*24*time.Hour + jitter)
			nextSendAt = pgtype.Timestamptz{Time: nextTime, Valid: true}
			nextStatus = pgtype.Text{String: "active", Valid: true}
		}

		queries.AdvanceLinkedInDMStep(ctx, repository.AdvanceLinkedInDMStepParams{
			ID:          lead.ID,
			CurrentStep: pgtype.Int4{Int32: int32(nextStep), Valid: true},
			NextSendAt:  nextSendAt,
			Status:      nextStatus,
			ChatID:      result.ChatID,
		})
	}
}

// LinkedInConnectionsFunc is the seam between the reconcile loop and Unipile's
// connections list — it returns, for one connected account, the member ids
// currently connected. Each ~45-min tick intersects this set with the
// account's awaiting_accept leads to detect acceptances (issue #7): the sole
// acceptance path, since the invitation.accepted notification lags ~8h.
// Production wires unipile.Module.AccountConnections; tests pass a stub.
type LinkedInConnectionsFunc func(ctx context.Context, unipileAccountID string) ([]string, error)

// LinkedInActivityFunc is the seam between the reconcile loop and Unipile's
// chat list — it returns, for one connected account, the inbound replies
// Unipile currently shows (acceptance is detected separately, by the
// connections check — see LinkedInConnectionsFunc). Production wires
// unipile.Module.AccountActivity; tests pass a stub.
type LinkedInActivityFunc func(ctx context.Context, unipileAccountID string) (unipile.AccountActivity, error)

// StartLinkedInReconcileLoop runs every 45 minutes (the PRD's ~30–60 min
// backstop) and applies any acceptance or reply the webhooks didn't deliver.
// It is idempotent with the webhook path: each accept goes through the
// awaiting_accept-gated MarkLinkedInAcceptedByMember, each reply through the
// status<>'replied'-gated RecordReplyByPerson, so a transition already made is
// a no-op here.
func StartLinkedInReconcileLoop(ctx context.Context, queries *repository.Queries, supp *suppression.Module, connections LinkedInConnectionsFunc, activity LinkedInActivityFunc) {
	ticker := time.NewTicker(45 * time.Minute)
	defer ticker.Stop()

	log.Println("LinkedIn reconcile loop started (checking every 45m)")

	reconcileLinkedInOnce(ctx, queries, supp, connections, activity)

	for {
		select {
		case <-ctx.Done():
			log.Println("LinkedIn reconcile loop stopping")
			return
		case <-ticker.C:
			reconcileLinkedInOnce(ctx, queries, supp, connections, activity)
		}
	}
}

// reconcileLinkedInOnce sweeps every account that can still act: it detects
// acceptances from the account's current connections and applies any reply the
// webhook missed. One account's listing failure never affects the others, and
// a connections failure never blocks the reply pass that follows.
func reconcileLinkedInOnce(ctx context.Context, queries *repository.Queries, supp *suppression.Module, connections LinkedInConnectionsFunc, activity LinkedInActivityFunc) {
	accounts, err := queries.ListLinkedInAccountsForReconcile(ctx)
	if err != nil {
		log.Printf("LinkedIn reconcile: list accounts: %v", err)
		return
	}
	for _, acc := range accounts {
		reconcileLinkedInAccepts(ctx, queries, connections, acc)

		act, err := activity(ctx, acc.UnipileAccountID)
		if err != nil {
			log.Printf("LinkedIn reconcile: activity for account %s: %v", acc.UnipileAccountID, err)
			continue
		}
		for _, rep := range act.Replies {
			applyLinkedInReply(ctx, queries, supp, rep)
		}
	}
}

// reconcileLinkedInAccepts lists one account's current connections and flips
// every awaiting_accept lead whose cached member id now appears among them
// (issue #7). Errors are logged and swallowed so a single account never blocks
// the sweep — nothing here is fatal to the reply pass that follows.
func reconcileLinkedInAccepts(ctx context.Context, queries *repository.Queries, connections LinkedInConnectionsFunc, acc repository.LinkedinAccount) {
	memberIDs, err := connections(ctx, acc.UnipileAccountID)
	if err != nil {
		log.Printf("LinkedIn reconcile: connections for account %s: %v", acc.UnipileAccountID, err)
		return
	}
	awaiting, err := queries.GetLinkedInAwaitingAcceptByAccount(ctx, acc.ID)
	if err != nil {
		log.Printf("LinkedIn reconcile: awaiting-accept leads for account %s: %v", fmtID(acc.ID), err)
		return
	}
	if len(awaiting) == 0 {
		return
	}
	connSet := make(map[string]struct{}, len(memberIDs))
	for _, id := range memberIDs {
		connSet[id] = struct{}{}
	}
	for _, lead := range matchAcceptedLeads(connSet, awaiting) {
		applyLinkedInAccept(ctx, queries, acc.ID, lead.MemberID)
	}
}

// matchAcceptedLeads is the pure core of the accept check: given an account's
// current connection member ids and its awaiting_accept leads, it returns the
// leads whose cached member id is now connected. Total and side-effect-free —
// a lead not in the connections set, or a connection with no awaiting lead,
// both fall out here; the caller flips exactly what it returns.
func matchAcceptedLeads(connections map[string]struct{}, awaiting []repository.GetLinkedInAwaitingAcceptByAccountRow) []repository.GetLinkedInAwaitingAcceptByAccountRow {
	matched := make([]repository.GetLinkedInAwaitingAcceptByAccountRow, 0, len(awaiting))
	for _, lead := range awaiting {
		if _, ok := connections[lead.MemberID]; ok {
			matched = append(matched, lead)
		}
	}
	return matched
}

// applyLinkedInAccept flips every awaiting_accept lead on account that belongs
// to the accepted member id — active/step-1 with the first DM scheduled now —
// and writes one 'accepted' event per lead. MarkLinkedInAcceptedByMember's
// awaiting_accept gate makes it idempotent: a lead a prior tick (or the
// webhook) already flipped matches no row and returns no id, so no duplicate
// event is written. One member id can flip more than one lead (the same person
// in two campaigns against this account), so RETURNING is a set.
func applyLinkedInAccept(ctx context.Context, queries *repository.Queries, accountID pgtype.UUID, memberID string) {
	leadIDs, err := queries.MarkLinkedInAcceptedByMember(ctx, repository.MarkLinkedInAcceptedByMemberParams{
		MemberID:  memberID,
		AccountID: accountID,
	})
	if err != nil {
		log.Printf("LinkedIn reconcile: mark accepted for member %s: %v", memberID, err)
		return
	}
	for _, leadID := range leadIDs {
		if _, err := queries.CreateLinkedInEvent(ctx, repository.CreateLinkedInEventParams{
			CampaignLeadID:   leadID,
			EventType:        "accepted",
			Step:             1,
			UnipileMessageID: pgText(memberID),
		}); err != nil {
			log.Printf("LinkedIn reconcile: write accepted event for lead %s: %v", fmtID(leadID), err)
		}
	}
}

// applyLinkedInReply applies a reconciled reply: map the chat back to its
// lead and hand off to the suppression module, whose status<>'replied' gate
// makes it idempotent with the webhook. An unknown chat is a safe no-op.
func applyLinkedInReply(ctx context.Context, queries *repository.Queries, supp *suppression.Module, rep unipile.InboundReply) {
	if rep.ChatID == "" {
		return
	}
	leadID, err := queries.GetLinkedInLeadByChatID(ctx, rep.ChatID)
	if errors.Is(err, pgx.ErrNoRows) {
		return // a chat we don't track — nothing to do
	}
	if err != nil {
		log.Printf("LinkedIn reconcile: lookup lead for chat %s: %v", rep.ChatID, err)
		return
	}
	if err := supp.RecordReplyByPerson(ctx, uuid.UUID(leadID.Bytes), rep.MessageID); err != nil {
		log.Printf("LinkedIn reconcile: record reply for chat %s (lead %s): %v", rep.ChatID, fmtID(leadID), err)
	}
}

// LinkedInCancelFunc is the seam between the withdrawal sweep and
// unipile.CancelInvitation — the withdrawal analogue of LinkedInSendFunc.
// Production wires unipile.Module.CancelInvitation; tests pass a stub.
type LinkedInCancelFunc func(ctx context.Context, accountID, invitationID string) error

// StartLinkedInWithdrawalLoop runs hourly and withdraws connection invites
// that have gone unaccepted past the 21-day horizon (issue #7). It is a
// DB-driven sweep over parked leads, distinct from the reconcile loop's
// per-account Unipile listings, so it runs on its own ticker; hourly is
// ample for a 21-day horizon.
func StartLinkedInWithdrawalLoop(ctx context.Context, queries *repository.Queries, cancel LinkedInCancelFunc) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	log.Println("LinkedIn withdrawal loop started (checking hourly)")

	processLinkedInWithdrawals(ctx, queries, cancel)

	for {
		select {
		case <-ctx.Done():
			log.Println("LinkedIn withdrawal loop stopping")
			return
		case <-ticker.C:
			processLinkedInWithdrawals(ctx, queries, cancel)
		}
	}
}

// processLinkedInWithdrawals withdraws connection invites that have gone
// unaccepted past the 21-day horizon (issue #7). LinkedIn counts a pile of
// stale pending invites against an account's standing, so each one is
// cancelled at Unipile, the lead closed 'not_accepted', and a 'withdrawn'
// event written for the audit trail. A cancel failure leaves the lead
// awaiting_accept for the next sweep — we never close a lead whose invite is
// still live at LinkedIn — mirroring how the send path leaves a lead on error.
func processLinkedInWithdrawals(ctx context.Context, queries *repository.Queries, cancel LinkedInCancelFunc) {
	stale, err := queries.GetStaleLinkedInInvites(ctx, 50)
	if err != nil {
		log.Printf("LinkedIn withdrawal: error getting stale invites: %v", err)
		return
	}
	if len(stale) == 0 {
		return
	}

	log.Printf("LinkedIn withdrawal: %d stale invites to withdraw", len(stale))

	for _, lead := range stale {
		// The query already excludes NULL invitation ids; stay defensive.
		if !lead.LinkedinInvitationID.Valid {
			continue
		}
		if err := cancel(ctx, lead.UnipileAccountID, lead.LinkedinInvitationID.String); err != nil {
			log.Printf("LinkedIn withdrawal: cancel for lead %s failed: %v — leaving awaiting_accept", fmtID(lead.ID), err)
			continue
		}
		if _, err := queries.CreateLinkedInEvent(ctx, repository.CreateLinkedInEventParams{
			CampaignLeadID:   lead.ID,
			EventType:        "withdrawn",
			Step:             0,
			UnipileMessageID: lead.LinkedinInvitationID,
		}); err != nil {
			log.Printf("LinkedIn withdrawal: write withdrawn event for lead %s: %v", fmtID(lead.ID), err)
		}
		if err := queries.MarkLinkedInNotAccepted(ctx, lead.ID); err != nil {
			log.Printf("LinkedIn withdrawal: mark not_accepted for lead %s: %v", fmtID(lead.ID), err)
			continue
		}
		log.Printf("LinkedIn withdrawal: withdrew stale invite for lead %s", fmtID(lead.ID))
	}
}

// pickLinkedInAccount returns the tenant's first usable LinkedIn account.
// Only active/warming accounts may send; connecting/restricted/disconnected
// ones are skipped. Re-fetched per lead so its rolling counters are fresh.
func pickLinkedInAccount(ctx context.Context, queries *repository.Queries, tenantID pgtype.UUID) (repository.LinkedinAccount, bool) {
	accounts, err := queries.ListLinkedInAccounts(ctx, tenantID)
	if err != nil {
		log.Printf("LinkedIn loop: list accounts for tenant %s: %v", fmtID(tenantID), err)
		return repository.LinkedinAccount{}, false
	}
	for _, a := range accounts {
		if a.Status == "active" || a.Status == "warming" {
			return a, true
		}
	}
	return repository.LinkedinAccount{}, false
}

// toCounters projects a linkedin_accounts row onto the pacer's pure-logic
// view. The two window timestamps are window-START markers; a NULL one
// becomes the zero time, which the pacer reads as "no window yet → full
// budget."
func toCounters(a repository.LinkedinAccount) pacer.Counters {
	return pacer.Counters{
		WeeklyCount:       int(a.WeeklyInviteCount),
		WeeklyWindowStart: tsOrZero(a.WeeklyWindowStartedAt),
		DailyCount:        int(a.DailyInviteCount),
		DailyWindowStart:  tsOrZero(a.DailyResetAt),
		WarmupStartedAt:   tsOrZero(a.WarmupStartedAt),
	}
}

// acceptanceCounters reads an account's trailing-7-day invite outcomes for
// the pacer's acceptance breaker (issue #7). A read error yields an empty
// signal, which the pacer reads as "not enough data" — the breaker abstains
// rather than pausing the account on a transient DB hiccup, the safe direction.
func acceptanceCounters(ctx context.Context, queries *repository.Queries, accountID pgtype.UUID) pacer.Counters {
	stats, err := queries.GetLinkedInAcceptanceStats(ctx, accountID)
	if err != nil {
		log.Printf("LinkedIn loop: acceptance stats for account %s: %v", fmtID(accountID), err)
		return pacer.Counters{}
	}
	return pacer.Counters{InvitesSent7d: int(stats.InvitesSent), Accepted7d: int(stats.Accepted)}
}

// applyAcceptanceBreaker flips an account's paused flag when the breaker's
// verdict changes (issue #7), recording the computed trailing-7-day rate so
// the UI can show why. It writes only on a transition — pause when a
// struggling account first breaches the floor, resume when it recovers — so
// there is no per-tick churn. The pacer's Allowance independently returns 0
// while breached, so this flag is for surfacing, not for gating sends.
func applyAcceptanceBreaker(ctx context.Context, queries *repository.Queries, account repository.LinkedinAccount, c pacer.Counters) {
	tripped := c.BreakerTripped()
	if tripped == account.AcceptancePaused {
		return
	}
	rate, _ := c.AcceptanceRate()
	if err := queries.SetLinkedInAcceptanceState(ctx, repository.SetLinkedInAcceptanceStateParams{
		ID:               account.ID,
		AcceptancePaused: tripped,
		AcceptanceRate:   rate,
	}); err != nil {
		log.Printf("LinkedIn loop: set acceptance state for account %s: %v", fmtID(account.ID), err)
		return
	}
	if tripped {
		log.Printf("LinkedIn loop: PAUSED account %s — 7d acceptance %.0f%% below %.0f%% floor (%d/%d)",
			fmtID(account.ID), rate*100, pacer.BreakerThreshold*100, c.Accepted7d, c.InvitesSent7d)
	} else {
		log.Printf("LinkedIn loop: resumed account %s — 7d acceptance recovered to %.0f%%", fmtID(account.ID), rate*100)
	}
}

func tsOrZero(t pgtype.Timestamptz) time.Time {
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

// escalateRestriction flips a connected account to 'restricted' when a send
// returned the restricted sentinel (issue #8), recording the error so Settings
// can surface it and prompt a reconnect. Only the restricted sentinel
// escalates: a rate-limit is transient and an auth failure is a Unipile-config
// problem, not something LinkedIn did to this account — both leave the status
// untouched so the lead simply retries next tick. Keyed by the Unipile account
// id (shared by the invite and DM ticks), so it mirrors the account-status
// webhook's path (handleAccountStatus) and stays idempotent on repeat errors.
func escalateRestriction(ctx context.Context, queries *repository.Queries, unipileAccountID string, sendErr error) {
	if !errors.Is(sendErr, unipile.ErrAccountRestricted) {
		return
	}
	if err := queries.SetLinkedInAccountStatusByUnipileID(ctx, repository.SetLinkedInAccountStatusByUnipileIDParams{
		UnipileAccountID: unipileAccountID,
		Status:           "restricted",
		LastError:        pgText(sendErr.Error()),
	}); err != nil {
		log.Printf("LinkedIn loop: escalate restriction for account %s: %v", unipileAccountID, err)
	}
}

// classifyLinkedInSendError turns a unipile.SendInvitation error into a
// short reason string for the failed linkedin_event — the LinkedIn
// analogue of classifyWorkerSendError. Keeps metadata queryable by reason
// without parsing free-text error messages.
func classifyLinkedInSendError(err error) string {
	switch {
	case errors.Is(err, unipile.ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, unipile.ErrUnauthorized):
		return "auth"
	case errors.Is(err, unipile.ErrAccountRestricted):
		return "restricted"
	default:
		return "network"
	}
}
