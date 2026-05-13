package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

func main() {
	_ = godotenv.Load()

	clerkSecret := os.Getenv("CLERK_SECRET_KEY")
	if clerkSecret == "" {
		log.Fatal("CLERK_SECRET_KEY is required")
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
	aiClient := ai.NewClient(os.Getenv("ANTHROPIC_API_KEY"))

	// Gmail service
	gmailOAuth := gmailpkg.NewOAuthConfig(
		os.Getenv("GOOGLE_CLIENT_ID"),
		os.Getenv("GOOGLE_CLIENT_SECRET"),
		os.Getenv("GOOGLE_REDIRECT_URI"),
	)
	gmailSvc := gmailpkg.NewService(gmailOAuth, queries, os.Getenv("FRONTEND_URL"))

	// Handlers
	websiteH := handler.NewWebsiteHandler(aiClient, queries)
	playH := handler.NewPlayHandler(aiClient, queries)
	campaignH := handler.NewCampaignHandler(queries)
	leadH := handler.NewLeadHandler(queries, asynqClient)
	leadSearchH := handler.NewLeadSearchHandler(queries)
	gmailH := handler.NewGmailHandler(gmailSvc, queries)
	sequenceH := handler.NewSequenceHandler(aiClient, queries)
	emailAccH := handler.NewEmailAccountHandler(queries)
	deliverH := handler.NewDeliverabilityHandler()
	billingH := handler.NewBillingHandler(queries, os.Getenv("STRIPE_SECRET_KEY"), os.Getenv("STRIPE_WEBHOOK_SECRET"), os.Getenv("FRONTEND_URL"))
	clerkH := handler.NewClerkHandler(queries, pool, os.Getenv("CLERK_WEBHOOK_SECRET"))
	adminH := handler.NewAdminHandler(queries, pool)
	systemMailer := email.NewFromEnv()
	privacyH := handler.NewPrivacyHandler(queries, pool, systemMailer, os.Getenv("FRONTEND_URL"))
	tenantLeadH := handler.NewTenantLeadHandler(queries)
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

		// GDPR right-to-erasure (public) — request/confirm flow:
		// request stores a hashed 24h token and mails a link; confirm
		// runs the deletion only after the caller clicks the link.
		r.Post("/privacy/erasure/request", privacyH.RequestErasure)
		r.Post("/privacy/erasure/confirm", privacyH.ConfirmErasure)

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
			r.Post("/campaigns/{id}/start", campaignH.Start)
			r.Post("/campaigns/{id}/pause", campaignH.Pause)
			r.Get("/campaigns/{id}/leads", campaignH.ListLeads)
			r.Post("/campaigns/{id}/leads", campaignH.AddLeads)

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

			// Gmail (OAuth)
			r.Get("/gmail/auth-url", gmailH.AuthURL)
			r.Get("/gmail/callback", gmailH.Callback)
			r.Get("/gmail/accounts", gmailH.ListAccounts)
			r.Delete("/gmail/accounts/{id}", gmailH.DeleteAccount)

			// Email Accounts (SMTP + Gmail)
			r.Post("/email-accounts/smtp", emailAccH.ConnectSMTP)
			r.Get("/email-accounts", emailAccH.List)
			r.Delete("/email-accounts/{id}", emailAccH.Delete)

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
