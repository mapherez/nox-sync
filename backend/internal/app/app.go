package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mapherez/nox-sync/backend/internal/storage"
)

// Config contains the minimal backend runtime configuration.
type Config struct {
	Addr    string
	DataDir string
	Version string
}

// Run starts the backend HTTP server.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "/data"
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}

	if err := ensureDataDirs(cfg.DataDir); err != nil {
		return err
	}

	store, err := storage.Open(ctx, cfg.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close sqlite database: %v", err)
		}
	}()

	server := NewServer(cfg, store)
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("nox-sync backend listening on %s data_dir=%s", cfg.Addr, cfg.DataDir)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func ensureDataDirs(dataDir string) error {
	dirs := []string{
		dataDir,
		filepath.Join(dataDir, "blobs"),
		filepath.Join(dataDir, "staging"),
		filepath.Join(dataDir, "logs"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create data directory %q: %w", dir, err)
		}
	}

	return nil
}
