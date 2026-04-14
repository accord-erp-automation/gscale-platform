package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gscale-zebra/internal/mobileapi"
)

func main() {
	cfg := mobileapi.LoadConfig()
	srv := mobileapi.New(cfg)
	defer srv.Close()

	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: srv.Handler(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("mobile API listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("mobile API listen error: %v", err)
			stop()
		}
	}()

	go func() {
		if err := srv.ListenAndServeDiscovery(ctx); err != nil && ctx.Err() == nil {
			log.Printf("mobile API discovery warning: %v", err)
		}
	}()

	go func() {
		if err := srv.ListenAndServeBonjour(ctx); err != nil && ctx.Err() == nil {
			log.Printf("mobile API bonjour warning: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("mobile API shutdown warning: %v", err)
		os.Exit(1)
	}
}
