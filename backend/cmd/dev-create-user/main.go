// Local-dev shortcut that mirrors the user.created Clerk webhook
// handler. Use it to bootstrap a sign-up when you don't want to set
// up a public-tunnel webhook for the dev devpod.
//
// Flow:
//   1. Sign up at the frontend; Clerk creates the auth user and
//      shows you a `user_xxx` ID in its dashboard.
//   2. Run this CLI with that ID and the email you used:
//
//      devpods exec api go run ./cmd/dev-create-user \
//          --clerk-id=user_2abc... \
//          --email=you@example.com \
//          --name="Your Name"
//
// Inserts users + tenants + user_tenants + free subscription rows,
// the exact same set the production webhook does. Idempotent — if
// the clerk_id already maps to a user, it logs and exits 0.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// freePlanLimits mirrors handler.PlanLimits["free"]; duplicated here
// to keep the CLI from importing the handler package (which would
// drag in Stripe / chi / Clerk SDK).
var freePlanLimits = struct {
	Leads     int32
	Sequences int32
}{100, 300}

func main() {
	clerkID := flag.String("clerk-id", "", "Clerk user ID, e.g. user_2abc...")
	email := flag.String("email", "", "primary email matching the Clerk account")
	name := flag.String("name", "", "display name (optional)")
	flag.Parse()

	if *clerkID == "" || *email == "" {
		log.Fatal("--clerk-id and --email are required")
	}

	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal("connect: ", err)
	}
	defer pool.Close()

	q := repository.New(pool)

	if existing, err := q.GetUserByClerkID(ctx, *clerkID); err == nil {
		log.Printf("user %s already exists (id=%x), nothing to do", *clerkID, existing.ID.Bytes)
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		log.Fatal("lookup user: ", err)
	}

	user, err := q.CreateUser(ctx, repository.CreateUserParams{
		ClerkID: *clerkID,
		Email:   *email,
		Name:    pgtype.Text{String: *name, Valid: *name != ""},
	})
	if err != nil {
		log.Fatal("create user: ", err)
	}

	workspaceName := *name + "'s Workspace"
	if *name == "" {
		workspaceName = *email + "'s Workspace"
	}
	tenant, err := q.CreateTenant(ctx, repository.CreateTenantParams{Name: workspaceName})
	if err != nil {
		log.Fatal("create tenant: ", err)
	}

	if err := q.CreateUserTenant(ctx, repository.CreateUserTenantParams{
		UserID:   user.ID,
		TenantID: tenant.ID,
		Role:     pgtype.Text{String: "owner", Valid: true},
	}); err != nil {
		log.Fatal("link user→tenant: ", err)
	}

	if _, err := q.CreateSubscription(ctx, repository.CreateSubscriptionParams{
		TenantID:       tenant.ID,
		Plan:           "free",
		LeadsLimit:     freePlanLimits.Leads,
		SequencesLimit: freePlanLimits.Sequences,
	}); err != nil {
		log.Fatal("create subscription: ", err)
	}

	log.Printf("created user %x + tenant %x for %s", user.ID.Bytes, tenant.ID.Bytes, *email)
}
