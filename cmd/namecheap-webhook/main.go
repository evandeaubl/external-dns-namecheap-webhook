package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/evandeaubl/external-dns-namecheap-webhook"
)

func main() {
	cfg := webhook.ParseFlags()

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	env := "sandbox"
	if cfg.Production {
		env = "production"
	}
	log.Printf("Starting Namecheap webhook server (environment: %s)", env)
	log.Printf("API URL: %s", cfg.APIURL())
	log.Printf("Webhook listen address: %s", cfg.ListenAddr)
	log.Printf("Healthz listen address: %s", cfg.HealthzAddr)

	server, err := webhook.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	webhookServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: server.Routes(),
	}

	healthzServer := &http.Server{
		Addr:    cfg.HealthzAddr,
		Handler: server.Routes(),
	}

	go func() {
		log.Printf("Webhook server listening on %s", cfg.ListenAddr)
		if err := webhookServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Webhook server error: %v", err)
		}
	}()

	go func() {
		log.Printf("Healthz server listening on %s", cfg.HealthzAddr)
		if err := healthzServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Healthz server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down servers...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := webhookServer.Shutdown(ctx); err != nil {
		log.Printf("Webhook server shutdown error: %v", err)
	}
	if err := healthzServer.Shutdown(ctx); err != nil {
		log.Printf("Healthz server shutdown error: %v", err)
	}

	log.Println("Servers shut down")
}
