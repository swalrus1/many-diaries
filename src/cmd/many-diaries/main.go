package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/swalrus1/many-diaries/internal/config"
	"github.com/swalrus1/many-diaries/internal/index"
	"github.com/swalrus1/many-diaries/internal/medium/instagram"
	"github.com/swalrus1/many-diaries/internal/medium/obsidian"
	"github.com/swalrus1/many-diaries/internal/storage"
	"github.com/swalrus1/many-diaries/internal/web"
)

func main() {
	configPath := flag.String("config", "many-diaries.yaml", "path to YAML config")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	st, err := storage.NewDisk(cfg.Storage.Disk.Root)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	idx, err := index.OpenSQLite(ctx, st)
	if err != nil {
		log.Fatal(err)
	}
	srv, err := web.New(idx)
	if err != nil {
		log.Fatal(err)
	}
	srv.AddMedium(obsidian.New(st, idx))
	srv.AddMedium(instagram.New(st, idx))

	httpSrv := &http.Server{Addr: *addr, Handler: srv.Handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx)
	}()
	log.Printf("listening on %s", *addr)
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
	if err := idx.Close(context.Background()); err != nil {
		log.Printf("index flush: %v", err)
	}
}
