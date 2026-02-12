package main

import (
	"log/slog"
	"net/http"
	"os"

	httpadapter "github.com/jailtonjunior/pomelo/internal/adapters/input/http"
	"github.com/jailtonjunior/pomelo/internal/adapters/output/memory"
	application "github.com/jailtonjunior/pomelo/internal/application"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repo := memory.NewRepository()
	svc := application.NewService(repo)
	handler := httpadapter.NewHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	addr := ":8080"
	log.Info("pomelo webhook server listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Error("server failed", "err", err)
		os.Exit(1)
	}
}
