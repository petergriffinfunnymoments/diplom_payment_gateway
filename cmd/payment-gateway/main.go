package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	httptransport "github.com/go-kit/kit/transport/http"

	"github.com/go-kit/kit/endpoint"
	kitlog "github.com/go-kit/kit/log"

	payments "payment-gateway/internal/httpapi/payments"
	orchestratorSimple "payment-gateway/internal/orchestrator/simple"
)

type healthResponse struct {
	Status string `json:"status"`
}

func main() {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	} else if !strings.Contains(addr, ":") {
		// поддержка "8080" вместо ":8080"
		addr = ":" + addr
	}

	logger := kitlog.NewLogfmtLogger(os.Stdout)

	healthEndpoint := endpoint.Endpoint(func(ctx context.Context, request interface{}) (interface{}, error) {
		_ = ctx
		return healthResponse{Status: "ok"}, nil
	})

	decodeHealthRequest := func(_ context.Context, _ *http.Request) (interface{}, error) {
		return struct{}{}, nil
	}

	encodeHealthResponse := func(_ context.Context, w http.ResponseWriter, resp interface{}) error {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(w)
		return enc.Encode(resp)
	}

	healthHandler := httptransport.NewServer(
		healthEndpoint,
		decodeHealthRequest,
		encodeHealthResponse,
		httptransport.ServerBefore(func(ctx context.Context, _ *http.Request) context.Context {
			return ctx
		}),
	)

	mux := http.NewServeMux()
	mux.Handle("/health", healthHandler)

	orchestrator := orchestratorSimple.NewSimpleOrchestrator()
	mux.Handle("/payments", payments.NewCreatePaymentHandler(orchestrator, logger))

	// Статика (web/index.html и web/static/*)
	mux.Handle("/", http.FileServer(http.Dir("web")))

	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		mux.ServeHTTP(w, r)
		logger.Log(
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", float64(time.Since(start).Milliseconds()),
		)
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           logged,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Log("level", "info", "msg", "payment-gateway starting", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Log("level", "error", "msg", "server stopped", "err", err.Error())
	}
}
