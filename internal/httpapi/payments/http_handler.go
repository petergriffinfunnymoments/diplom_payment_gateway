package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/endpoint"
	httptransport "github.com/go-kit/kit/transport/http"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/merchantauth"
)

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewCreatePaymentHandler(
	orchestrator contracts.PaymentOrchestrator,
	logger interface{},
) http.Handler {
	_ = logger
	eventLogger, _ := logger.(contracts.EventLogger)

	createPaymentEndpoint := endpoint.Endpoint(func(ctx context.Context, request interface{}) (interface{}, error) {
		req, ok := request.(dto.CreatePaymentRequest)
		if !ok {
			return errorResponse{Code: dto.ErrorBadRequest, Message: "invalid request payload"}, nil
		}
		authMerchant, ok := merchantauth.MerchantFromContext(ctx)
		if !ok {
			return errorResponse{Code: dto.ErrorAuthContextMissing, Message: "authenticated merchant context is required"}, nil
		}
		if !merchantauth.CanWriteMerchantData(authMerchant, req.MerchantID) {
			merchantauth.LogAuthorizationFailed(ctx, eventLogger, authMerchant, req.MerchantID, "POST /payments", "payment creation is not allowed for this role or merchant")
			return errorResponse{Code: dto.ErrorForbidden, Message: "payment creation is not allowed for this role or merchant"}, nil
		}

		resp, err := orchestrator.CreatePayment(ctx, req)
		if err != nil {
			return errorResponse{Code: dto.ErrorNotImplemented, Message: err.Error()}, err
		}
		return resp, nil
	})

	decodeCreatePaymentRequest := func(_ context.Context, r *http.Request) (interface{}, error) {
		var req dto.CreatePaymentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, fmt.Errorf("invalid json")
		}
		return req, nil
	}

	encodeCreatePaymentResponse := func(_ context.Context, w http.ResponseWriter, response interface{}) error {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if errResp, ok := response.(errorResponse); ok {
			switch errResp.Code {
			case dto.ErrorBadRequest:
				w.WriteHeader(http.StatusBadRequest)
			case dto.ErrorAuthContextMissing:
				w.WriteHeader(http.StatusUnauthorized)
			case dto.ErrorForbidden:
				w.WriteHeader(http.StatusForbidden)
			}
		}
		return json.NewEncoder(w).Encode(response)
	}

	serverBefore := httptransport.ServerBefore(func(ctx context.Context, r *http.Request) context.Context {
		return ctx
	})

	serverAfter := httptransport.ServerAfter(func(ctx context.Context, w http.ResponseWriter) context.Context {
		_ = ctx
		_ = w
		return ctx
	})

	h := httptransport.NewServer(
		createPaymentEndpoint,
		decodeCreatePaymentRequest,
		encodeCreatePaymentResponse,
		serverBefore,

		serverAfter,
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() { _ = start }()

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(errorResponse{Code: dto.ErrorMethodNotAllowed, Message: "use POST"})
			return
		}

		h.ServeHTTP(w, r)
	})
}
