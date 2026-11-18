package main

import (
	"context"
	"log"
	"net/http"

	"techpedia/internal/auth"
	"techpedia/internal/config"
	"techpedia/internal/server"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

func main() {
	cfg := config.FromEnv()

	store, err := storage.OpenBadgerDocumentStore(cfg.BadgerDir())
	if err != nil {
		log.Fatalf("failed to open badger document store: %v", err)
	}
	defer store.Close()
	authStore, err := auth.Open(cfg.AuthDir())
	if err != nil {
		log.Fatalf("failed to open authentication store: %v", err)
	}
	defer authStore.Close()
	if err := authStore.EnsureBootstrapAdmin(context.Background(), cfg.BootstrapAdminUsername, cfg.BootstrapAdminPassword); err != nil {
		log.Fatalf("failed to create bootstrap administrator: %v", err)
	}

	parser := techmark.NewParser()

	app := server.New(cfg, store, authStore, parser)

	httpServer := &http.Server{
		Addr:    cfg.Addr,
		Handler: app.Handler(),
	}

	log.Printf("TechPedia listening on %s", cfg.Addr)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
