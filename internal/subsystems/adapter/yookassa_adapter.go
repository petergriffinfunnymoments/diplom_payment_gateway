package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type YooKassaAdapter struct {
	shopID    string
	secretKey string
	apiURL    string
	refundURL string
	returnURL string
	client    *http.Client
}

func NewYooKassaAdapterFromEnv() (contracts.PaymentAdapter, error) {
	shopID := strings.TrimSpace(os.Getenv("YOOKASSA_SHOP_ID"))
	secretKey := strings.TrimSpace(os.Getenv("YOOKASSA_SECRET_KEY"))
	if shopID == "" || secretKey == "" {
		return nil, errors.New("YOOKASSA_SHOP_ID and YOOKASSA_SECRET_KEY are required")
	}

	apiURL := strings.TrimSpace(os.Getenv("YOOKASSA_API_URL"))
	if apiURL == "" {
		apiURL = "https://api.yookassa.ru/v3/payments"
	}
	refundURL := strings.TrimSpace(os.Getenv("YOOKASSA_REFUNDS_URL"))
	if refundURL == "" {
		refundURL = "https://api.yookassa.ru/v3/refunds"
	}

	returnURL := strings.TrimSpace(os.Getenv("PAYMENT_RETURN_URL"))
	if returnURL == "" {
		returnURL = strings.TrimSpace(os.Getenv("YOOKASSA_RETURN_URL"))
	}
	if returnURL == "" {
		returnURL = "http://localhost:8080"
	}

	return &YooKassaAdapter{
		shopID:    shopID,
		secretKey: secretKey,
		apiURL:    apiURL,
		refundURL: refundURL,
		returnURL: returnURL,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}, nil
}

func (a *YooKassaAdapter) Refund(ctx context.Context, req contracts.RefundRequest) (contracts.RefundResult, error) {
	body := map[string]any{
		"amount": map[string]string{
			"value":    fmt.Sprintf("%.2f", req.Amount.Value),
			"currency": string(req.Amount.Currency),
		},
		"payment_id":  req.ExternalPaymentID,
		"description": truncate(req.Reason, 128),
		"metadata": map[string]string{
			"merchant_id":     req.MerchantID,
			"payment_id":      req.PaymentID,
			"refund_id":       req.RefundID,
			"idempotency_key": req.IdempotencyKey,
		},
	}
	if strings.TrimSpace(req.Reason) == "" {
		delete(body, "description")
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return contracts.RefundResult{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.refundURL, bytes.NewReader(payload))
	if err != nil {
		return contracts.RefundResult{}, err
	}
	httpReq.SetBasicAuth(a.shopID, a.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotence-Key", req.IdempotencyKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return contracts.RefundResult{}, err
	}
	defer resp.Body.Close()

	var yResp yookassaRefundResponse
	if err := json.NewDecoder(resp.Body).Decode(&yResp); err != nil {
		return contracts.RefundResult{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := yResp.Description
		if msg == "" {
			msg = fmt.Sprintf("yookassa refund returned HTTP %d", resp.StatusCode)
		}
		return contracts.RefundResult{
			PaymentSystem:  "YOOKASSA",
			Status:         string(dto.RefundStatusFail),
			ProviderStatus: yResp.Type,
			ErrorMessage:   msg,
		}, nil
	}

	errorMessage := yResp.CancellationDetails.Reason
	return contracts.RefundResult{
		ExternalRefundID: yResp.ID,
		PaymentSystem:    "YOOKASSA",
		Status:           mapYooKassaRefundStatus(yResp.Status),
		ProviderStatus:   yResp.Status,
		ErrorMessage:     errorMessage,
	}, nil
}

func (a *YooKassaAdapter) Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (contracts.AdapterResult, error) {
	_ = token // внутренний токен шлюза не отправляем во внешнюю платежную форму

	body := map[string]any{
		"amount": map[string]string{
			"value":    fmt.Sprintf("%.2f", req.PaymentInfo.Amount.Value),
			"currency": string(req.PaymentInfo.Amount.Currency),
		},
		"capture": true,
		"confirmation": map[string]string{
			"type":       "redirect",
			"return_url": a.returnURL,
		},
		"description": truncate(req.PaymentInfo.Description, 128),
		"metadata": map[string]string{
			"merchant_id":     req.MerchantID,
			"payment_id":      req.PaymentID,
			"idempotency_key": req.IdempotencyKey,
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return contracts.AdapterResult{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiURL, bytes.NewReader(payload))
	if err != nil {
		return contracts.AdapterResult{}, err
	}
	httpReq.SetBasicAuth(a.shopID, a.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotence-Key", req.IdempotencyKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return contracts.AdapterResult{}, err
	}
	defer resp.Body.Close()

	var yResp yookassaPaymentResponse
	if err := json.NewDecoder(resp.Body).Decode(&yResp); err != nil {
		return contracts.AdapterResult{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := yResp.Description
		if msg == "" {
			msg = fmt.Sprintf("yookassa returned HTTP %d", resp.StatusCode)
		}
		return contracts.AdapterResult{
			PaymentSystem:  "YOOKASSA",
			Status:         string(dto.StatusFailed),
			ProviderStatus: yResp.Type,
			ErrorCode:      dto.ErrorProviderUnavailable,
			ErrorMessage:   msg,
		}, nil
	}

	status := mapYooKassaStatus(yResp.Status)
	errorMessage := yResp.CancellationDetails.Reason
	if errorMessage == "" && status == string(dto.StatusDeclined) {
		errorMessage = "payment was canceled by YooKassa"
	}

	return contracts.AdapterResult{
		ExternalTransactionID: yResp.ID,
		PaymentSystem:         "YOOKASSA",
		Status:                status,
		ProviderStatus:        yResp.Status,
		PaymentURL:            yResp.Confirmation.ConfirmationURL,
		ErrorCode:             yookassaAdapterErrorCode(status, yResp.CancellationDetails.Reason),
		ErrorMessage:          errorMessage,
	}, nil
}

func yookassaAdapterErrorCode(status string, reason string) string {
	if status != string(dto.StatusDeclined) && status != string(dto.StatusFailed) {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(reason), "fraud_suspected") {
		return dto.ErrorYooKassaFraudSuspected
	}
	return dto.ErrorYooKassaPaymentDeclined
}

type yookassaPaymentResponse struct {
	Type         string `json:"type"`
	ID           string `json:"id"`
	Status       string `json:"status"`
	Paid         bool   `json:"paid"`
	Description  string `json:"description"`
	Confirmation struct {
		Type            string `json:"type"`
		ConfirmationURL string `json:"confirmation_url"`
	} `json:"confirmation"`
	CancellationDetails struct {
		Party  string `json:"party"`
		Reason string `json:"reason"`
	} `json:"cancellation_details"`
}

type yookassaRefundResponse struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Status      string `json:"status"`
	Description string `json:"description"`
	Amount      struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	PaymentID           string `json:"payment_id"`
	CreatedAt           string `json:"created_at"`
	CancellationDetails struct {
		Party  string `json:"party"`
		Reason string `json:"reason"`
	} `json:"cancellation_details"`
}

func mapYooKassaStatus(status string) string {
	switch strings.ToLower(status) {
	case "succeeded":
		return string(dto.StatusCaptured)
	case "pending", "waiting_for_capture":
		return string(dto.StatusPending)
	case "canceled":
		return string(dto.StatusDeclined)
	default:
		return string(dto.StatusPending)
	}
}

func mapYooKassaRefundStatus(status string) string {
	switch strings.ToLower(status) {
	case "succeeded":
		return string(dto.RefundStatusSuccess)
	case "pending":
		return string(dto.RefundStatusProcess)
	case "canceled":
		return string(dto.RefundStatusFail)
	default:
		return string(dto.RefundStatusProcess)
	}
}
