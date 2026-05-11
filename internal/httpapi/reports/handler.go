package reports

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/merchantauth"
)

const defaultLimit = 100
const maxLimit = 500

type Handler struct {
	store contracts.TransactionReportStore
}

func NewTransactionReportHandler(store contracts.TransactionReportStore) http.Handler {
	return &Handler{store: store}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, dto.TransactionReportResponse{
			Success: false,
			Error:   &dto.GatewayError{Code: "METHOD_NOT_ALLOWED", Message: "use GET"},
		})
		return
	}
	if r.URL.Path != "/reports/transactions" {
		writeJSON(w, http.StatusNotFound, dto.TransactionReportResponse{
			Success: false,
			Error:   &dto.GatewayError{Code: "NOT_FOUND", Message: "report endpoint not found"},
		})
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusInternalServerError, dto.TransactionReportResponse{
			Success: false,
			Error:   &dto.GatewayError{Code: "REPORT_STORE_UNAVAILABLE", Message: "report store is not configured"},
		})
		return
	}

	filter, err := decodeTransactionReportFilter(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, dto.TransactionReportResponse{
			Success: false,
			Error:   &dto.GatewayError{Code: "BAD_REQUEST", Message: err.Error()},
		})
		return
	}

	headerMerchantID := strings.TrimSpace(r.Header.Get(merchantauth.HeaderMerchantID))
	if headerMerchantID != "" && filter.MerchantID != headerMerchantID {
		writeJSON(w, http.StatusForbidden, dto.TransactionReportResponse{
			Success: false,
			Error:   &dto.GatewayError{Code: "MERCHANT_SCOPE_MISMATCH", Message: "merchant_id must match X-Merchant-ID"},
		})
		return
	}

	report, err := h.store.BuildTransactionReport(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.TransactionReportResponse{
			Success: false,
			Error:   &dto.GatewayError{Code: "REPORT_STORAGE_ERROR", Message: err.Error()},
		})
		return
	}

	writeJSON(w, http.StatusOK, dto.TransactionReportResponse{Data: &report, Success: true})
}

func decodeTransactionReportFilter(r *http.Request) (dto.TransactionReportFilter, error) {
	q := r.URL.Query()
	filter := dto.TransactionReportFilter{
		MerchantID:    strings.TrimSpace(q.Get("merchant_id")),
		Status:        strings.TrimSpace(q.Get("status")),
		PaymentSystem: strings.TrimSpace(firstNonEmpty(q.Get("payment_system"), q.Get("provider"))),
		PaymentMethod: dto.PaymentMethodType(strings.TrimSpace(q.Get("payment_method"))),
		Limit:         defaultLimit,
	}
	if filter.MerchantID == "" {
		return filter, errors.New("merchant_id is required")
	}

	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return filter, errors.New("limit must be an integer")
		}
		if limit <= 0 {
			return filter, errors.New("limit must be greater than zero")
		}
		filter.Limit = limit
	}
	if filter.Limit > maxLimit {
		filter.Limit = maxLimit
	}

	dateFrom, err := parseOptionalReportTime(q.Get("date_from"), false)
	if err != nil {
		return filter, err
	}
	filter.DateFrom = dateFrom

	dateTo, err := parseOptionalReportTime(q.Get("date_to"), true)
	if err != nil {
		return filter, err
	}
	filter.DateTo = dateTo

	return filter, nil
}

func parseOptionalReportTime(value string, endOfDay bool) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	if t, err := time.Parse(time.RFC3339, value); err == nil {
		utc := t.UTC()
		return &utc, nil
	}

	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, errors.New("date_from/date_to must be RFC3339 or YYYY-MM-DD")
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	utc := t.UTC()
	return &utc, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

var _ http.Handler = (*Handler)(nil)
