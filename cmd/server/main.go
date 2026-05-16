package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jjtny1/iouapp/internal/api"
	"github.com/jjtny1/iouapp/internal/auth"
	"github.com/jjtny1/iouapp/internal/config"
	"github.com/jjtny1/iouapp/internal/db"
)

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	mailer, err := auth.NewSender(context.Background(), cfg.MailProvider, cfg.MailFrom, cfg.AWSRegion)
	if err != nil {
		log.Fatalf("mailer: %v", err)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewRouter(database, cfg, mailer),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("IOU listening on http://localhost:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
