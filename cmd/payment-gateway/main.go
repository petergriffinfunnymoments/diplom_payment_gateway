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
	paymentsecurity "payment-gateway/internal/httpapi/security"
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
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "dev"
	}
	transportSecurity := paymentsecurity.TransportConfigFromEnv()
	networkSecurity, networkErr := paymentsecurity.NetworkConfigFromEnv()
	if networkErr != nil {
		logger.Log("level", "error", "msg", "invalid network security configuration", "err", networkErr.Error())
		os.Exit(1)
	}
	transportSecurity.TrustedProxyCIDRs = networkSecurity.TrustedProxyCIDRs
	tlsCertFile := strings.TrimSpace(os.Getenv("TLS_CERT_FILE"))
	tlsKeyFile := strings.TrimSpace(os.Getenv("TLS_KEY_FILE"))
	if err := paymentsecurity.ValidateTLSConfig(tlsCertFile, tlsKeyFile); err != nil {
		logger.Log("level", "error", "msg", err.Error())
		os.Exit(1)
	}
	if transportSecurity.RequireHTTPS && tlsCertFile == "" && !transportSecurity.TrustProxyHeaders {
		logger.Log("level", "error", "msg", "HTTPS enforcement requires TLS_CERT_FILE/TLS_KEY_FILE or TRUST_PROXY_HEADERS=true")
		os.Exit(1)
	}
	if paymentsecurity.IsProductionEnv(appEnv) && transportSecurity.TrustProxyHeaders && len(networkSecurity.TrustedProxyCIDRs) == 0 {
		logger.Log("level", "error", "msg", "TRUSTED_PROXY_CIDRS must be set when TRUST_PROXY_HEADERS=true in production")
		os.Exit(1)
	}
	if err := paymentsecurity.ValidateOutboundURLs(transportSecurity.RequireHTTPS, map[string]string{
		"PAYMENT_RETURN_URL":   os.Getenv("PAYMENT_RETURN_URL"),
		"MERCHANT_WEBHOOK_URL": os.Getenv("MERCHANT_WEBHOOK_URL"),
	}); err != nil {
		logger.Log("level", "error", "msg", "insecure outbound URL configuration", "err", err.Error())
		os.Exit(1)
	}

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

		pgEventLogger, err := paymentlogging.NewPostgresEventLogger(context.Background(), dsn, appEnv)
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
	authenticated := func(next http.Handler) http.Handler {
		return authenticator.Middleware(paymentsecurity.AuthenticatedNetworkMiddleware(networkSecurity, eventLogger, next))
	}
	providerWebhook := func(next http.Handler) http.Handler {
		return paymentsecurity.WebhookNetworkMiddleware(networkSecurity, eventLogger, next)
	}

	mux.Handle("/payments", authenticated(payments.NewCreatePaymentHandler(orchestrator, eventLogger)))
	mux.Handle("/payments/", authenticated(payments.NewGetPaymentStatusHandlerWithLogger(store, eventLogger)))
	refundHandler := refunds.NewRefundHandler(store, refundStore, adapter.NewFactoryFromEnv(), eventLogger)
	mux.Handle("/refunds/", authenticated(refundHandler))
	mux.Handle("/reports/transactions", authenticated(reports.NewTransactionReportHandlerWithLogger(reportStore, eventLogger)))
	mux.Handle("/webhooks/yookassa", providerWebhook(webhooks.NewYooKassaWebhookHandlerWithNotifications(store, eventLogger, notificationService)))
	mux.Handle("/webhooks/stripe", providerWebhook(webhooks.NewStripeWebhookHandler(store, eventLogger, notificationService)))
	mux.Handle("/webhooks/robokassa", providerWebhook(webhooks.NewRobokassaWebhookHandler(store, eventLogger, notificationService)))
	digitalRubleSandboxHandler := providerWebhook(webhooks.NewDigitalRubleSandboxHandler(store, eventLogger, notificationService))
	mux.Handle("/sandbox/digital-ruble/scan", digitalRubleSandboxHandler)
	mux.Handle("/webhooks/digital-ruble", digitalRubleSandboxHandler)
	mux.Handle("/merchant/webhook", webhooks.NewMerchantDemoWebhookHandler(eventLogger))

	secured := paymentsecurity.Middleware(transportSecurity, mux)
	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		secured.ServeHTTP(w, r)
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

	logger.Log(
		"level", "info",
		"msg", "payment-gateway starting",
		"addr", addr,
		"app_env", appEnv,
		"require_https", transportSecurity.RequireHTTPS,
		"trust_proxy_headers", transportSecurity.TrustProxyHeaders,
		"tls_enabled", tlsCertFile != "",
	)
	var err error
	if tlsCertFile != "" {
		err = srv.ListenAndServeTLS(tlsCertFile, tlsKeyFile)
	} else {
		err = srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		logger.Log("level", "error", "msg", "server stopped", "err", err.Error())
	}
}
