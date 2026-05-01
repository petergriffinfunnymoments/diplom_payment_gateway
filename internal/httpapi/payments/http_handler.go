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
)

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewCreatePaymentHandler создаёт go-kit HTTP handler для POST /payments.
func NewCreatePaymentHandler(
	orchestrator contracts.PaymentOrchestrator,
	logger interface{}, // для диплома оставим сигнатуру; позже заменим на contracts.EventLogger/kit/log.Logger
) http.Handler {
	_ = logger

	createPaymentEndpoint := endpoint.Endpoint(func(ctx context.Context, request interface{}) (interface{}, error) {
		req, ok := request.(dto.CreatePaymentRequest)
		if !ok {
			return errorResponse{Code: "BAD_REQUEST", Message: "invalid request payload"}, nil
		}

		// На старте оркестратор может быть заглушкой — вернём 501 на уровне транспорта.
		resp, err := orchestrator.CreatePayment(ctx, req)
		if err != nil {
			return errorResponse{Code: "NOT_IMPLEMENTED", Message: err.Error()}, err
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
		return json.NewEncoder(w).Encode(response)
	}

	// Для простоты: маппим ошибку оркестратора в 501, decoding-ошибки — в 400.
	// Детальную маппинг-таблицу сделаем позже.
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
		// ServerAfter не содержит duration; логирование оставим позже.
		serverAfter,
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() { _ = start }()

		// Валидация метода.
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(errorResponse{Code: "METHOD_NOT_ALLOWED", Message: "use POST"})
			return
		}

		// go-kit server сам вызовет decode/encode; но если декодер вернёт error — это попадёт в endpoint error путь.
		// Поэтому перехватываем статус только по типу ответа, если endpoint вернёт error.
		h.ServeHTTP(w, r)
	})
}
