package adapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type TBankAdapter struct {
	terminalKey     string
	password        string
	apiToken        string
	initURL         string
	successURL      string
	failURL         string
	notificationURL string
	client          *http.Client
}

func NewTBankAdapterFromEnv() (contracts.PaymentAdapter, error) {
	terminalKey := strings.TrimSpace(os.Getenv("TBANK_TERMINAL_KEY"))
	if terminalKey == "" {
		return nil, errors.New("TBANK_TERMINAL_KEY is required")
	}

	password := strings.TrimSpace(os.Getenv("TBANK_PASSWORD"))
	apiToken := strings.TrimSpace(os.Getenv("TBANK_API_TOKEN"))
	if password == "" && apiToken == "" {
		return nil, errors.New("TBANK_PASSWORD or TBANK_API_TOKEN is required")
	}

	initURL := strings.TrimSpace(os.Getenv("TBANK_INIT_URL"))
	if initURL == "" {
		initURL = "https://securepay.tinkoff.ru/v2/Init"
	}

	returnURL := strings.TrimSpace(os.Getenv("PAYMENT_RETURN_URL"))
	if returnURL == "" {
		returnURL = "http://localhost:8080"
	}

	successURL := strings.TrimSpace(os.Getenv("TBANK_SUCCESS_URL"))
	if successURL == "" {
		successURL = returnURL
	}
	failURL := strings.TrimSpace(os.Getenv("TBANK_FAIL_URL"))
	if failURL == "" {
		failURL = returnURL
	}

	return &TBankAdapter{
		terminalKey:     terminalKey,
		password:        password,
		apiToken:        apiToken,
		initURL:         initURL,
		successURL:      successURL,
		failURL:         failURL,
		notificationURL: strings.TrimSpace(os.Getenv("TBANK_NOTIFICATION_URL")),
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}, nil
}

func (a *TBankAdapter) Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (contracts.AdapterResult, error) {
	_ = token

	amountKopecks := int64(math.Round(req.PaymentInfo.Amount.Value * 100))
	payload := map[string]any{
		"TerminalKey": a.terminalKey,
		"Amount":      amountKopecks,
		"OrderId":     truncate(req.PaymentID, 36),
		"Description": truncate(req.PaymentInfo.Description, 140),
		"PayType":     "O",
		"Language":    "ru",
		"SuccessURL":  a.successURL,
		"FailURL":     a.failURL,
		"DATA": map[string]string{
			"merchant_id":     truncate(req.MerchantID, 100),
			"idempotency_key": truncate(req.IdempotencyKey, 100),
		},
	}
	if a.notificationURL != "" {
		payload["NotificationURL"] = a.notificationURL
	}
	if req.PaymentInfo.CustomerData.Email != "" || req.PaymentInfo.CustomerData.Phone != "" {
		payload["Receipt"] = map[string]any{
			"Email":    req.PaymentInfo.CustomerData.Email,
			"Phone":    req.PaymentInfo.CustomerData.Phone,
			"Taxation": "osn",
			"Items": []map[string]any{{
				"Name":          truncate(req.PaymentInfo.Description, 128),
				"Price":         amountKopecks,
				"Quantity":      1,
				"Amount":        amountKopecks,
				"PaymentMethod": "full_payment",
				"PaymentObject": "service",
				"Tax":           "none",
			}},
		}
	}

	if a.password != "" {
		payload["Token"] = signTBankPayload(payload, a.password)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return contracts.AdapterResult{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.initURL, bytes.NewReader(body))
	if err != nil {
		return contracts.AdapterResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.apiToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiToken)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return contracts.AdapterResult{}, err
	}
	defer resp.Body.Close()

	var tResp tbankInitResponse
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return contracts.AdapterResult{}, err
	}

	result := contracts.AdapterResult{
		ExternalTransactionID: tResp.PaymentID.String(),
		PaymentSystem:         "TBANK",
		ProviderStatus:        tResp.Status,
		PaymentURL:            tResp.PaymentURL,
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Status = string(dto.StatusFailed)
		result.ErrorMessage = fmt.Sprintf("tbank returned HTTP %d: %s", resp.StatusCode, firstNonEmpty(tResp.Message, tResp.Details))
		return result, nil
	}

	if !tResp.Success {
		result.Status = string(dto.StatusFailed)
		result.ErrorMessage = firstNonEmpty(tResp.Message, tResp.Details, "tbank init failed")
		return result, nil
	}

	result.Status = mapTBankStatus(tResp.Status)
	return result, nil
}

type tbankInitResponse struct {
	Success     bool           `json:"Success"`
	ErrorCode   string         `json:"ErrorCode"`
	TerminalKey string         `json:"TerminalKey"`
	Status      string         `json:"Status"`
	PaymentID   flexibleString `json:"PaymentId"`
	OrderID     string         `json:"OrderId"`
	Amount      int64          `json:"Amount"`
	PaymentURL  string         `json:"PaymentURL"`
	Message     string         `json:"Message"`
	Details     string         `json:"Details"`
}

type flexibleString string

func (s *flexibleString) UnmarshalJSON(b []byte) error {
	var asString string
	if err := json.Unmarshal(b, &asString); err == nil {
		*s = flexibleString(asString)
		return nil
	}
	var asNumber json.Number
	if err := json.Unmarshal(b, &asNumber); err == nil {
		*s = flexibleString(asNumber.String())
		return nil
	}
	return nil
}

func (s flexibleString) String() string {
	return string(s)
}

func signTBankPayload(payload map[string]any, password string) string {
	values := map[string]string{"Password": password}
	for k, v := range payload {
		if k == "Token" {
			continue
		}
		switch t := v.(type) {
		case string:
			values[k] = t
		case int:
			values[k] = strconv.Itoa(t)
		case int64:
			values[k] = strconv.FormatInt(t, 10)
		case float64:
			values[k] = strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			values[k] = strconv.FormatBool(t)
		}
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(values[k])
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

func mapTBankStatus(status string) string {
	switch strings.ToUpper(status) {
	case "CONFIRMED", "AUTHORIZED":
		return string(dto.StatusCaptured)
	case "REJECTED", "CANCELED", "CANCELLED", "DEADLINE_EXPIRED":
		return string(dto.StatusDeclined)
	case "NEW", "FORM_SHOWED", "AUTHORIZING", "3DS_CHECKING", "CHECKING", "UNKNOWN":
		return string(dto.StatusPending)
	default:
		return string(dto.StatusPending)
	}
}
