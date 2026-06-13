package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nl2sql-executor-go-prod/internal/api"
	"nl2sql-executor-go-prod/internal/cache"
	"nl2sql-executor-go-prod/internal/config"
	"nl2sql-executor-go-prod/internal/datasource"
	"nl2sql-executor-go-prod/internal/job"
	"nl2sql-executor-go-prod/internal/schema"
	"nl2sql-executor-go-prod/internal/telegram"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "configs/config.example.yaml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := os.MkdirAll(cfg.Storage.CacheDir, 0755); err != nil {
		log.Fatalf("create cache dir: %v", err)
	}
	if err := os.MkdirAll(cfg.Storage.ResultDir, 0755); err != nil {
		log.Fatalf("create result dir: %v", err)
	}
	if err := os.MkdirAll(cfg.Storage.JobDir, 0755); err != nil {
		log.Fatalf("create job dir: %v", err)
	}

	cat, err := schema.LoadCatalog(cfg.Schema.CatalogPath)
	if err != nil {
		log.Printf("schema catalog disabled: %v", err)
		cat = schema.NewEmptyCatalog()
	}

	dsManager, err := datasource.NewManager(cfg.Datasources)
	if err != nil {
		log.Fatalf("datasource manager: %v", err)
	}
	defer dsManager.Close()

	tg := telegram.NewClient(cfg.Telegram)
	localCache := cache.NewFileCache(cfg.Storage.CacheDir, time.Duration(cfg.Cache.TTLSeconds)*time.Second)

	mgr := job.NewManager(cfg, dsManager, tg, localCache, cat)
	mgr.Start(context.Background())

	srv := api.NewServer(cfg, mgr, dsManager)
	httpServer := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: time.Duration(cfg.Server.ReadHeaderTimeoutSec) * time.Second,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
		IdleTimeout:       time.Duration(cfg.Server.IdleTimeoutSec) * time.Second,
	}

	go func() {
		log.Printf("nl2sql executor listening on %s", cfg.Server.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	mgr.Stop()
	log.Println("shutdown complete")
}
