package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/jcleira/magiklead/backend/internal/ai"
	"github.com/jcleira/magiklead/backend/internal/email"
	gmailpkg "github.com/jcleira/magiklead/backend/internal/gmail"
	"github.com/jcleira/magiklead/backend/internal/handler"
	"github.com/jcleira/magiklead/backend/internal/leads/pdl"
	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

// clerkKeyPattern is Clerk's documented secret-key shape:
// sk_test_ or sk_live_ followed by at least 20 alphanumerics. The
// non-empty check upstream let `sk_test_YOUR_CLERK_SECRET_KEY_HERE`
// pass startup and only fail later in request handling.
var clerkKeyPattern = regexp.MustCompile(`^sk_(test|live)_[A-Za-z0-9]{20,}$`)

func validateClerkKey(key string) error {
	if key == "" {
		return errors.New("CLERK_SECRET_KEY is required")
	}
	if !clerkKeyPattern.MatchString(key) {
		return fmt.Errorf("CLERK_SECRET_KEY does not match Clerk's documented format (sk_test_… or sk_live_… followed by 20+ alphanumerics); got %q", key)
	}
	return nil
}

func main() {
	_ = godotenv.Load()

	clerkSecret := os.Getenv("CLERK_SECRET_KEY")
	if err := validateClerkKey(clerkSecret); err != nil {
		log.Fatal(err)
	}
	clerk.SetKey(clerkSecret)

	pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("failed to connect to database:", err)
	}
	defer pool.Close()

	redisOpt := asynq.RedisClientOpt{Addr: os.Getenv("REDIS_URL")}
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	queries := repository.New(pool)
	supp := suppression.New(pool)
	aiClient := ai.NewClient(os.Getenv("ANTHROPIC_API_KEY"))

	// Gmail service
	gmailOAuth := gmailpkg.NewOAuthConfig(
		os.Getenv("GOOGLE_CLIENT_ID"),
		os.Getenv("GOOGLE_CLIENT_SECRET"),
		os.Getenv("GOOGLE_REDIRECT_URI"),
	)
	gmailSvc := gmailpkg.NewService(gmailOAuth, queries, os.Getenv("FRONTEND_URL"))

	// Signs the Gmail OAuth `state` param (HS256). The callback is a
	// public route — Google redirects there with no session — so the
	// tenant + user binding rides in this signed state instead. Required
	// only when Gmail OAuth is configured; pure-dev runs without
	// GOOGLE_CLIENT_ID skip the connect flow entirely. Distinct from
	// UNSUBSCRIBE_SIGNING_SECRET so the two token types can't be crossed.
	gmailStateSecret := os.Getenv("GMAIL_OAUTH_STATE_SECRET")
	if os.Getenv("GOOGLE_CLIENT_ID") != "" && gmailStateSecret == "" {
		log.Fatal("GMAIL_OAUTH_STATE_SECRET is required when GOOGLE_CLIENT_ID is set (signs the Gmail OAuth state param)")
	}

	// PDL — real-data spine behind the canonical person graph (issue #7).
	// The handler tolerates a nil module so dev runs without
	// PDL_API_KEY still serve canonical-only results.
	var pdlModule *pdl.Module
	if pdlKey := os.Getenv("PDL_API_KEY"); pdlKey != "" {
		pdlModule = pdl.New(pool, pdlKey, nil)
	} else {
		log.Print("PDL_API_KEY not set — lead search falls through to canonical-only mode")
	}

	// Unipile — LinkedIn send rail (docs/2026-06-09-linkedin-only-outreach).
	// Wired unconditionally; the handler degrades (like PDL) when
	// UNIPILE_API_KEY is absent. UNIPILE_WEBHOOK_SECRET signs both the
	// metadata token embedded in the hosted-auth link and the inbound
	// webhook body HMAC, so it is required once the api key is set
	// (the public webhook itself still returns 503 if it is missing).
	unipileKey := os.Getenv("UNIPILE_API_KEY")
	unipileWebhookSecret := os.Getenv("UNIPILE_WEBHOOK_SECRET")
	if unipileKey != "" && unipileWebhookSecret == "" {
		log.Fatal("UNIPILE_WEBHOOK_SECRET is required when UNIPILE_API_KEY is set (signs the metadata token and verifies inbound webhooks)")
	}
	unipileModule := unipile.New(unipileKey, os.Getenv("UNIPILE_DSN"), []byte(unipileWebhookSecret), nil)
	if unipileKey == "" {
		log.Print("UNIPILE_API_KEY not set — LinkedIn connect/send features disabled")
	}

	// Handlers
	websiteH := handler.NewWebsiteHandler(aiClient, queries)
	playH := handler.NewPlayHandler(aiClient, queries)
	campaignH := handler.NewCampaignHandler(queries, supp)
	leadH := handler.NewLeadHandler(queries, asynqClient)
	leadSearchH := handler.NewLeadSearchHandler(queries, pdlModule)
	gmailH := handler.NewGmailHandler(gmailSvc, queries, []byte(gmailStateSecret), os.Getenv("FRONTEND_URL"))
	unipileH := handler.NewUnipileHandler(unipileModule, queries, supp, []byte(unipileWebhookSecret), os.Getenv("FRONTEND_URL"), os.Getenv("APP_URL"))
	sequenceH := handler.NewSequenceHandler(aiClient, queries)
	emailAccH := handler.NewEmailAccountHandler(queries)
	deliverH := handler.NewDeliverabilityHandler()
	billingH := handler.NewBillingHandler(queries, os.Getenv("STRIPE_SECRET_KEY"), os.Getenv("STRIPE_WEBHOOK_SECRET"), os.Getenv("FRONTEND_URL"))
	clerkH := handler.NewClerkHandler(queries, pool, os.Getenv("CLERK_WEBHOOK_SECRET"))
	adminH := handler.NewAdminHandler(queries, pool)
	systemMailer := email.NewFromEnv()
	privacyH := handler.NewPrivacyHandler(queries, pool, systemMailer, os.Getenv("FRONTEND_URL"))
	tenantLeadH := handler.NewTenantLeadHandler(queries)
	settingsH := handler.NewSettingsHandler(queries)
	accountH := handler.NewAccountHandler(queries, pool, os.Getenv("CLERK_SECRET_KEY"))
	// Public unsubscribe (issue #6) — recipients clicking from their
	// inbox have no Clerk session; authentication is the signed token.
	// Same secret is consumed by the worker when minting tokens, so
	// the worker and api MUST share UNSUBSCRIBE_SIGNING_SECRET.
	unsubscribeSecret := os.Getenv("UNSUBSCRIBE_SIGNING_SECRET")
	if unsubscribeSecret == "" {
		log.Fatal("UNSUBSCRIBE_SIGNING_SECRET is required (must match the worker's value)")
	}
	publicUnsubH := handler.NewPublicUnsubscribeHandler([]byte(unsubscribeSecret), supp)
	adminEmails := strings.Split(os.Getenv("ADMIN_EMAILS"), ",")

	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{os.Getenv("FRONTEND_URL")},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	r.Route("/api/v1", func(r chi.Router) {
		// Public webhook routes
		r.Post("/webhooks/clerk", clerkH.HandleWebhook)
		r.Post("/webhooks/stripe", billingH.HandleWebhook)
		// Unipile webhook (public): Unipile reaches it with no Clerk
		// session — authentication is the HMAC signature over the body
		// plus the signed metadata in the account.connected payload.
		r.Post("/webhooks/unipile", unipileH.Webhook)

		// GDPR right-to-erasure (public) — request/confirm flow:
		// request stores a hashed 24h token and mails a link; confirm
		// runs the deletion only after the caller clicks the link.
		r.Post("/privacy/erasure/request", privacyH.RequestErasure)
		r.Post("/privacy/erasure/confirm", privacyH.ConfirmErasure)

		// RFC 8058 one-click unsubscribe (issue #6). POST is the
		// one-click action; GET catches email-client pre-fetches
		// and hand-typed link visits.
		r.Post("/public/unsubscribe", publicUnsubH.Handle)
		r.Get("/public/unsubscribe", publicUnsubH.Handle)

		// Gmail OAuth callback (public): Google redirects the user's
		// browser here with ?code&state and no Authorization header, so
		// it cannot sit behind ClerkAuth. The tenant + user binding
		// rides in the signed `state` minted by the protected
		// /gmail/auth-url; Callback verifies it.
		r.Get("/gmail/callback", gmailH.Callback)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.ClerkAuth)
			r.Use(middleware.EnsureTenant(middleware.NewLookup(queries), middleware.NewBootstrap(pool)))

			// Websites
			r.Post("/websites/analyze", websiteH.Analyze)

			// Plays
			r.Post("/plays/generate", playH.Generate)
			r.Get("/plays", playH.List)
			r.Get("/plays/{id}", playH.Get)
			r.Delete("/plays/{id}", playH.Delete)

			// Campaigns
			r.Post("/campaigns", campaignH.Create)
			r.Get("/campaigns", campaignH.List)
			r.Get("/campaigns/{id}", campaignH.Get)
			r.Get("/campaigns/{id}/metrics", campaignH.Metrics)
			r.Get("/campaigns/{id}/linkedin-metrics", campaignH.LinkedInMetrics)
			r.Post("/campaigns/{id}/start", campaignH.Start)
			r.Post("/campaigns/{id}/pause", campaignH.Pause)
			r.Get("/campaigns/{id}/leads", campaignH.ListLeads)
			r.Post("/campaigns/{id}/leads", campaignH.AddLeads)
			r.Post("/campaigns/{id}/leads/{lead_id}/reengage", campaignH.Reengage)

			// Leads
			r.Post("/leads/discover", leadH.Discover)
			r.Post("/leads/search", leadSearchH.Search)
			r.Get("/leads", leadH.List)
			r.Get("/leads/{id}", leadH.Get)

			// Saved (tenant) leads — canonical person-indexed.
			r.Post("/tenant_leads", tenantLeadH.Add)
			r.Get("/tenant_leads", tenantLeadH.List)
			r.Patch("/tenant_leads/{person_id}", tenantLeadH.Update)
			r.Delete("/tenant_leads/{person_id}", tenantLeadH.Delete)

			// Sequences
			r.Post("/sequences/generate", sequenceH.Generate)

			// Gmail (OAuth). The callback is public (see above); minting
			// the signed state requires the Clerk session, so auth-url
			// stays here.
			r.Get("/gmail/auth-url", gmailH.AuthURL)
			r.Get("/gmail/accounts", gmailH.ListAccounts)
			r.Delete("/gmail/accounts/{id}", gmailH.DeleteAccount)

			// LinkedIn (Unipile hosted-auth). The webhook is public
			// (see above); minting the signed metadata requires the
			// Clerk session, so auth-url stays here.
			r.Get("/linkedin/auth-url", unipileH.AuthURL)
			r.Get("/linkedin/accounts", unipileH.ListAccounts)
			r.Get("/linkedin/capacity", unipileH.Capacity)
			r.Delete("/linkedin/accounts/{id}", unipileH.DeleteAccount)

			// Email Accounts (SMTP + Gmail)
			r.Post("/email-accounts/smtp", emailAccH.ConnectSMTP)
			r.Get("/email-accounts", emailAccH.List)
			r.Delete("/email-accounts/{id}", emailAccH.Delete)

			// Settings — consolidated tenant/plan/usage/email-accounts.
			r.Get("/settings", settingsH.Get)

			// Account self-service (issue #11): GDPR-adjacent export +
			// delete. Both are tenant-scoped behind ClerkAuth; the
			// handler walks the cascade in dependency order inside a tx.
			r.Post("/account/export", accountH.Export)
			r.Delete("/account", accountH.Delete)

			// Deliverability
			r.Get("/deliverability/check", deliverH.CheckDomain)

			// Billing
			r.Post("/billing/checkout", billingH.CreateCheckout)
			r.Post("/billing/portal", billingH.CreatePortal)
			r.Get("/billing/subscription", billingH.GetSubscription)

			// Admin (gated by AdminAuth — env var ADMIN_EMAILS)
			r.Group(func(r chi.Router) {
				r.Use(middleware.AdminAuth(adminEmails))
				r.Get("/admin/conflicts", adminH.ListConflicts)
				r.Post("/admin/conflicts/{id}/merge", adminH.MergeConflict)
				r.Post("/admin/conflicts/{id}/reject", adminH.RejectConflict)
				r.Get("/admin/persons/{id}", adminH.GetPerson)
				r.Post("/admin/persons/{id}/delete", adminH.DeletePerson)
			})
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		log.Printf("API server starting on :%s", port)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
}
