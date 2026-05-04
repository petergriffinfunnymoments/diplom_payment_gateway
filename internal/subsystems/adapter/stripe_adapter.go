package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

// StripeAdapter создаёт платеж через Stripe Checkout Session.
// Важно: внутренний токен платежного шлюза не отправляется в Stripe как платёжный реквизит.
// Stripe получает запрос на создание Checkout Session, а данные карты пользователь вводит на странице Stripe Checkout.
type StripeAdapter struct {
	secretKey        string
	checkoutURL      string
	successURL       string
	cancelURL        string
	currencyOverride string
	amountMultiplier float64
	client           *http.Client
}

func NewStripeAdapterFromEnv() (contracts.PaymentAdapter, error) {
	secretKey := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	if secretKey == "" {
		// На всякий случай поддерживаем имя, которое иногда используют в .env.
		secretKey = strings.TrimSpace(os.Getenv("STRIPE_API_KEY"))
	}
	if secretKey == "" {
		return nil, errors.New("STRIPE_SECRET_KEY is required")
	}

	checkoutURL := strings.TrimSpace(os.Getenv("STRIPE_CHECKOUT_SESSIONS_URL"))
	if checkoutURL == "" {
		checkoutURL = "https://api.stripe.com/v1/checkout/sessions"
	}

	returnURL := strings.TrimSpace(os.Getenv("PAYMENT_RETURN_URL"))
	if returnURL == "" {
		returnURL = "http://localhost:8080"
	}

	successURL := strings.TrimSpace(os.Getenv("STRIPE_SUCCESS_URL"))
	if successURL == "" {
		successURL = strings.TrimRight(returnURL, "/") + "/?stripe_payment=success&payment_id={CHECKOUT_SESSION_ID}"
	}

	cancelURL := strings.TrimSpace(os.Getenv("STRIPE_CANCEL_URL"))
	if cancelURL == "" {
		cancelURL = strings.TrimRight(returnURL, "/") + "/?stripe_payment=cancelled"
	}

	amountMultiplier := 100.0
	if raw := strings.TrimSpace(os.Getenv("STRIPE_AMOUNT_MULTIPLIER")); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
			amountMultiplier = v
		}
	}

	return &StripeAdapter{
		secretKey:        secretKey,
		checkoutURL:      checkoutURL,
		successURL:       successURL,
		cancelURL:        cancelURL,
		currencyOverride: strings.ToLower(strings.TrimSpace(os.Getenv("STRIPE_CURRENCY_OVERRIDE"))),
		amountMultiplier: amountMultiplier,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}, nil
}

func (a *StripeAdapter) Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (contracts.AdapterResult, error) {
	_ = token

	currency := strings.ToLower(string(req.PaymentInfo.Amount.Currency))
	if a.currencyOverride != "" {
		currency = a.currencyOverride
	}
	if currency == "" {
		currency = "usd"
	}

	unitAmount := int64(math.Round(req.PaymentInfo.Amount.Value * a.amountMultiplier))
	if unitAmount <= 0 {
		return contracts.AdapterResult{
			PaymentSystem: "STRIPE",
			Status:        string(dto.StatusFailed),
			ErrorMessage:  "amount must be greater than zero for Stripe Checkout",
		}, nil
	}

	productName := req.PaymentInfo.Description
	if strings.TrimSpace(productName) == "" {
		productName = "Payment " + req.PaymentID
	}

	form := url.Values{}
	form.Set("mode", "payment")
	form.Set("success_url", a.successURL)
	form.Set("cancel_url", a.cancelURL)
	form.Set("client_reference_id", req.PaymentID)
	form.Set("payment_method_types[0]", "card")
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", currency)
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(unitAmount, 10))
	form.Set("line_items[0][price_data][product_data][name]", truncate(productName, 120))

	// Metadata нужна, чтобы webhook Stripe можно было связать с внутренним платежом шлюза.
	form.Set("metadata[merchant_id]", req.MerchantID)
	form.Set("metadata[payment_id]", req.PaymentID)
	form.Set("metadata[idempotency_key]", req.IdempotencyKey)
	form.Set("payment_intent_data[metadata][merchant_id]", req.MerchantID)
	form.Set("payment_intent_data[metadata][payment_id]", req.PaymentID)
	form.Set("payment_intent_data[metadata][idempotency_key]", req.IdempotencyKey)

	if email := strings.TrimSpace(req.PaymentInfo.CustomerData.Email); email != "" {
		form.Set("customer_email", email)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.checkoutURL, strings.NewReader(form.Encode()))
	if err != nil {
		return contracts.AdapterResult{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.secretKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Idempotency-Key", req.IdempotencyKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return contracts.AdapterResult{}, err
	}
	defer resp.Body.Close()

	var sResp stripeCheckoutSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sResp); err != nil {
		return contracts.AdapterResult{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || sResp.Error.Message != "" {
		msg := sResp.Error.Message
		if msg == "" {
			msg = fmt.Sprintf("stripe returned HTTP %d", resp.StatusCode)
		}
		return contracts.AdapterResult{
			PaymentSystem:  "STRIPE",
			Status:         string(dto.StatusFailed),
			ProviderStatus: sResp.Error.Type,
			ErrorMessage:   msg,
		}, nil
	}

	return contracts.AdapterResult{
		ExternalTransactionID: sResp.ID,
		PaymentSystem:         "STRIPE",
		Status:                mapStripeCheckoutStatus(sResp.Status, sResp.PaymentStatus),
		ProviderStatus:        firstNonEmpty(sResp.PaymentStatus, sResp.Status),
		PaymentURL:            sResp.URL,
		ErrorMessage:          "",
	}, nil
}

type stripeCheckoutSessionResponse struct {
	ID            string `json:"id"`
	Object        string `json:"object"`
	Status        string `json:"status"`
	PaymentStatus string `json:"payment_status"`
	URL           string `json:"url"`
	PaymentIntent string `json:"payment_intent"`
	Error         struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func mapStripeCheckoutStatus(status string, paymentStatus string) string {
	paymentStatus = strings.ToLower(strings.TrimSpace(paymentStatus))
	status = strings.ToLower(strings.TrimSpace(status))

	switch paymentStatus {
	case "paid", "no_payment_required":
		return string(dto.StatusCaptured)
	case "unpaid":
		if status == "expired" {
			return string(dto.StatusDeclined)
		}
		return string(dto.StatusPending)
	}

	switch status {
	case "complete":
		return string(dto.StatusCaptured)
	case "expired":
		return string(dto.StatusDeclined)
	case "open":
		return string(dto.StatusPending)
	default:
		return string(dto.StatusPending)
	}
}
