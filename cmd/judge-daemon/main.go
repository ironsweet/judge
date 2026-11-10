package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/ironsweet/hookfacet"
	"github.com/ironsweet/judge"
	"github.com/ironsweet/judge/facet"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	socket := getenv("JUDGE_SOCKET", "/tmp/judge.sock")
	workspace := getenv("JUDGE_WORKSPACE", filepath.Join(os.Getenv("HOME"), ".judge"))

	p := judge.NewFilePersister(workspace)
	llmFn := buildLLMFn()
	j := judge.New(llmFn, judge.WithLogger(logger))
	h := facet.New(j, p, facet.WithLogger(logger))

	r := hookfacet.NewRunner(h)
	err := hookfacet.Serve(ctx, socket, r, logger)
	if err != nil {
		logger.Error("failed to start judge server", "error", err)
		os.Exit(1)
	}
}

func getenv(key, defaultValue string) string {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	return v
}

func buildLLMFn() judge.LLMFunc {
	return func(ctx context.Context, prompt string) (string, error) {
		panic("niy")
	}
}
