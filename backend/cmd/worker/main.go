package main

import (
	"context"
	"log"
	"os"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/jcleira/magiklead/backend/internal/gmail"
	"github.com/jcleira/magiklead/backend/internal/gmail/poller"
	"github.com/jcleira/magiklead/backend/internal/leads"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
	"github.com/jcleira/magiklead/backend/internal/worker"
)

func main() {
	_ = godotenv.Load()

	pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("failed to connect to database:", err)
	}
	defer pool.Close()

	queries := repository.New(pool)
	supp := suppression.New(pool)

	// Gmail send path — issue #3 wires this in front of the existing
	// suppression gate so outbound goes through the user's connected
	// Gmail OAuth account, not bare SMTP.
	gmailOAuth := gmail.NewOAuthConfig(
		os.Getenv("GOOGLE_CLIENT_ID"),
		os.Getenv("GOOGLE_CLIENT_SECRET"),
		os.Getenv("GOOGLE_REDIRECT_URI"),
	)
	gmailSender := gmail.NewSender(gmailOAuth)

	// Gmail History API poller — issue #4 wires reply (and bounce-
	// classification) detection per connected mailbox on a 2-minute
	// cadence alongside the existing send loop.
	gmailPoller := poller.New(gmailOAuth, worker.NewSentLookup(queries))

	// Lead discovery — AI + web scraping + SMTP email verification
	pipeline := leads.NewPipeline(queries, os.Getenv("ANTHROPIC_API_KEY"))

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: os.Getenv("REDIS_URL")},
		asynq.Config{
			Concurrency: 5,
			Queues:      map[string]int{"default": 1},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(leads.TypeDiscoverLeads, worker.HandleDiscoverLeads(pipeline, queries))

	// Unsubscribe header config (issue #6). Both the api and worker
	// must agree on the signing secret, otherwise tokens minted here
	// won't verify at /api/v1/public/unsubscribe.
	unsubSecret := os.Getenv("UNSUBSCRIBE_SIGNING_SECRET")
	if unsubSecret == "" {
		log.Fatal("UNSUBSCRIBE_SIGNING_SECRET is required (must match the api's value)")
	}
	unsubMailDomain := os.Getenv("UNSUBSCRIBE_MAIL_DOMAIN")
	if unsubMailDomain == "" {
		log.Fatal("UNSUBSCRIBE_MAIL_DOMAIN is required (e.g. mail.magiklead.com)")
	}
	unsubCfg := worker.UnsubscribeConfig{
		Secret:     []byte(unsubSecret),
		AppURL:     os.Getenv("APP_URL"),
		MailDomain: unsubMailDomain,
	}

	// Start email send loop in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go worker.StartSendLoop(ctx, queries, supp, gmailSender.Send, unsubCfg)
	go worker.StartPollLoop(ctx, queries, supp, gmailPoller.Tick)

	log.Println("Worker starting...")
	if err := srv.Run(mux); err != nil {
		log.Fatal("failed to start worker:", err)
	}
}
