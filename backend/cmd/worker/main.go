package main

import (
	"context"
	"log"
	"os"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

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

	// Start email send loop in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go worker.StartSendLoop(ctx, queries, supp)

	log.Println("Worker starting...")
	if err := srv.Run(mux); err != nil {
		log.Fatal("failed to start worker:", err)
	}
}
