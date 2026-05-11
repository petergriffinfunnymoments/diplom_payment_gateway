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

	"payment-gateway/internal/contracts"
	payments "payment-gateway/internal/httpapi/payments"
	refunds "payment-gateway/internal/httpapi/refunds"
	reports "payment-gateway/internal/httpapi/reports"
	webhooks "payment-gateway/internal/httpapi/webhooks"
	orchestratorSimple "payment-gateway/internal/orchestrator/simple"
	"payment-gateway/internal/subsystems/adapter"
	paymentlogging "payment-gateway/internal/subsystems/logging"
	"payment-gateway/internal/subsystems/merchantauth"
	paymentnotifications "payment-gateway/internal/subsystems/notifications"
	"payment-gateway/internal/subsystems/routing"
	"payment-gateway/internal/subsystems/storage"
	paymenttokenizer "payment-gateway/internal/subsystems/tokenizer"
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

	store := storage.NewInMemoryTransactionStore()
	var eventLogger contracts.EventLogger = paymentlogging.NewConsoleEventLogger(logger)
	var tokenizerService contracts.Tokenizer = paymenttokenizer.NewEphemeralTokenizer()
	var notificationService contracts.Notifications = paymentnotifications.NewNoOpNotifications()
	var routeStore contracts.PaymentRouteStore
	var refundStore contracts.RefundStore
	var reportStore contracts.TransactionReportStore
	authenticator := merchantauth.NewAuthenticator(merchantauth.NewStaticMerchantStoreFromEnv())
	if rs, ok := store.(contracts.RefundStore); ok {
		refundStore = rs
	}
	if rs, ok := store.(contracts.TransactionReportStore); ok {
		reportStore = rs
	}

	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		pgStore, err := storage.NewPostgresTransactionStoreAsContract(context.Background(), dsn)
		if err != nil {
			logger.Log("level", "error", "msg", "failed to connect postgres", "err", err.Error())
			os.Exit(1)
		}
		store = pgStore
		if rs, ok := pgStore.(contracts.RefundStore); ok {
			refundStore = rs
		}
		if rs, ok := pgStore.(contracts.TransactionReportStore); ok {
			reportStore = rs
		}
		logger.Log("level", "info", "msg", "postgres transaction store connected")

		env := os.Getenv("APP_ENV")
		if env == "" {
			env = "dev"
		}

		pgEventLogger, err := paymentlogging.NewPostgresEventLogger(context.Background(), dsn, env)
		if err != nil {
			logger.Log("level", "error", "msg", "failed to connect postgres event logger", "err", err.Error())
			os.Exit(1)
		}
		eventLogger = pgEventLogger
		logger.Log("level", "info", "msg", "postgres event logger connected")

		pgTokenizer, err := paymenttokenizer.NewPostgresTokenizer(context.Background(), dsn)
		if err != nil {
			logger.Log("level", "error", "msg", "failed to connect postgres tokenizer", "err", err.Error())
			os.Exit(1)
		}
		tokenizerService = pgTokenizer
		logger.Log("level", "info", "msg", "postgres tokenizer connected")

		merchantStore, err := merchantauth.NewPostgresMerchantStore(context.Background(), dsn)
		if err != nil {
			logger.Log("level", "error", "msg", "failed to initialize merchant authentication", "err", err.Error())
			os.Exit(1)
		}
		authenticator = merchantauth.NewAuthenticator(merchantStore)
		logger.Log("level", "info", "msg", "merchant authentication enabled")

		pgRouteStore, err := routing.NewPostgresPaymentRouteStoreAsContract(context.Background(), dsn)
		if err != nil {
			logger.Log("level", "error", "msg", "failed to initialize payment routes", "err", err.Error())
			os.Exit(1)
		}
		routeStore = pgRouteStore
		logger.Log("level", "info", "msg", "payment router storage connected")

		webhookNotifications, err := paymentnotifications.NewWebhookNotificationsFromEnv(context.Background(), dsn, eventLogger)
		if err != nil {
			logger.Log("level", "error", "msg", "failed to initialize merchant notifications", "err", err.Error())
			os.Exit(1)
		}
		notificationService = webhookNotifications
		if os.Getenv("MERCHANT_WEBHOOK_URL") != "" {
			logger.Log("level", "info", "msg", "merchant webhook notifications enabled")
		} else {
			logger.Log("level", "warn", "msg", "MERCHANT_WEBHOOK_URL is empty; merchant webhook notifications disabled")
		}
	} else {
		logger.Log("level", "warn", "msg", "DATABASE_URL is empty; using in-memory transaction store and console event logger")
	}

	orchestrator := orchestratorSimple.NewSimpleOrchestratorWithRouting(store, eventLogger, tokenizerService, notificationService, routeStore)
	mux.Handle("/payments", authenticator.Middleware(payments.NewCreatePaymentHandler(orchestrator, logger)))
	mux.Handle("/payments/", authenticator.Middleware(payments.NewGetPaymentStatusHandler(store)))
	refundHandler := refunds.NewRefundHandler(store, refundStore, adapter.NewFactoryFromEnv(), eventLogger)
	mux.Handle("/refunds/", authenticator.Middleware(refundHandler))
	mux.Handle("/reports/transactions", authenticator.Middleware(reports.NewTransactionReportHandler(reportStore)))
	mux.Handle("/webhooks/yookassa", webhooks.NewYooKassaWebhookHandlerWithNotifications(store, eventLogger, notificationService))
	mux.Handle("/webhooks/stripe", webhooks.NewStripeWebhookHandler(store, eventLogger, notificationService))
	mux.Handle("/merchant/webhook", webhooks.NewMerchantDemoWebhookHandler(eventLogger))

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
