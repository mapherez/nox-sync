package main

import (
	"context"
	"log"
	"os"

	"github.com/mapherez/nox-sync/backend/internal/app"
)

func main() {
	cfg := app.Config{
		Addr:    getenv("NOX_SYNC_ADDR", ":8080"),
		DataDir: getenv("NOX_SYNC_DATA_DIR", "/data"),
		Version: getenv("NOX_SYNC_VERSION", "dev"),
	}

	if err := app.Run(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
