package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-kit/kit/endpoint"
	httptransport "github.com/go-kit/kit/transport/http"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type getPaymentStatusRequest struct {
	MerchantID string
	PaymentID  string
}

type statusErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewGetPaymentStatusHandler создаёт HTTP handler для GET /payments/{payment_id}.
// MerchantID передаётся query-параметром: /payments/pay_123?merchant_id=merchant_12345.
func NewGetPaymentStatusHandler(store contracts.TransactionStore) http.Handler {
	getPaymentStatusEndpoint := endpoint.Endpoint(func(ctx context.Context, request interface{}) (interface{}, error) {
		req, ok := request.(getPaymentStatusRequest)
		if !ok {
			return statusErrorResponse{Code: "BAD_REQUEST", Message: "invalid request"}, nil
		}

		if req.MerchantID == "" {
			return statusErrorResponse{Code: "BAD_REQUEST", Message: "merchant_id is required"}, nil
		}
		if req.PaymentID == "" {
			return statusErrorResponse{Code: "BAD_REQUEST", Message: "payment_id is required"}, nil
		}

		_, payloadJSON, found, err := store.GetByPaymentID(ctx, req.MerchantID, req.PaymentID)
		if err != nil {
			return statusErrorResponse{Code: "STORAGE_ERROR", Message: err.Error()}, nil
		}
		if !found {
			return statusErrorResponse{Code: "PAYMENT_NOT_FOUND", Message: "payment not found"}, nil
		}

		var resp dto.PaymentResponse
		if err := json.Unmarshal([]byte(payloadJSON), &resp); err != nil {
			return statusErrorResponse{Code: "INVALID_STORED_RESPONSE", Message: err.Error()}, nil
		}

		return resp.Sanitized(), nil
	})

	decodeGetPaymentStatusRequest := func(_ context.Context, r *http.Request) (interface{}, error) {
		paymentID := strings.TrimPrefix(r.URL.Path, "/payments/")
		paymentID = strings.Trim(paymentID, "/")

		return getPaymentStatusRequest{
			MerchantID: r.URL.Query().Get("merchant_id"),
			PaymentID:  paymentID,
		}, nil
	}

	encodeGetPaymentStatusResponse := func(_ context.Context, w http.ResponseWriter, response interface{}) error {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		if errResp, ok := response.(statusErrorResponse); ok {
			switch errResp.Code {
			case "BAD_REQUEST":
				w.WriteHeader(http.StatusBadRequest)
			case "PAYMENT_NOT_FOUND":
				w.WriteHeader(http.StatusNotFound)
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
		}

		return json.NewEncoder(w).Encode(response)
	}

	h := httptransport.NewServer(
		getPaymentStatusEndpoint,
		decodeGetPaymentStatusRequest,
		encodeGetPaymentStatusResponse,
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(statusErrorResponse{Code: "METHOD_NOT_ALLOWED", Message: "use GET"})
			return
		}

		h.ServeHTTP(w, r)
	})
}
