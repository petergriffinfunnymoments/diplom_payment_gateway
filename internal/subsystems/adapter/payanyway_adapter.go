package adapter

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type PayAnyWayAdapter struct {
	mntID        string
	integrityKey string
	paymentURL   string
	testMode     bool
	unitID       string
}

func NewPayAnyWayAdapterFromEnv() (contracts.PaymentAdapter, error) {
	mntID := firstNonEmpty(
		os.Getenv("PAYANYWAY_MNT_ID"),
		os.Getenv("PAYANYWAY_BUSINESS_ACCOUNT"),
		os.Getenv("PAYANYWAY_ACCOUNT_ID"),
	)
	if strings.TrimSpace(mntID) == "" {
		return nil, errors.New("PAYANYWAY_MNT_ID is required")
	}

	integrityKey := strings.TrimSpace(os.Getenv("PAYANYWAY_INTEGRITY_CODE"))
	if integrityKey == "" {
		return nil, errors.New("PAYANYWAY_INTEGRITY_CODE is required")
	}

	paymentURL := strings.TrimSpace(os.Getenv("PAYANYWAY_PAYMENT_URL"))
	if paymentURL == "" {
		paymentURL = "https://www.payanyway.ru/assistant.htm"
	}

	return &PayAnyWayAdapter{
		mntID:        strings.TrimSpace(mntID),
		integrityKey: integrityKey,
		paymentURL:   paymentURL,
		testMode:     payAnyWayBoolEnv("PAYANYWAY_TEST_MODE", true),
		unitID:       strings.TrimSpace(os.Getenv("PAYANYWAY_PAYMENT_UNIT_ID")),
	}, nil
}

func (a *PayAnyWayAdapter) Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (contracts.AdapterResult, error) {
	_ = ctx
	_ = token

	transactionID := strings.TrimSpace(req.PaymentID)
	if transactionID == "" {
		transactionID = strings.TrimSpace(req.IdempotencyKey)
	}
	if transactionID == "" {
		return contracts.AdapterResult{}, errors.New("payment_id or idempotency_key is required for PayAnyWay payment")
	}

	amount := fmt.Sprintf("%.2f", req.PaymentInfo.Amount.Value)
	currency := strings.TrimSpace(string(req.PaymentInfo.Amount.Currency))
	if currency == "" {
		currency = "RUB"
	}
	testMode := "0"
	if a.testMode {
		testMode = "1"
	}
	subscriberID := strings.TrimSpace(req.MerchantID)

	signature := payAnyWayPaymentSignature(
		a.mntID,
		transactionID,
		amount,
		currency,
		subscriberID,
		testMode,
		a.integrityKey,
	)

	params := url.Values{}
	params.Set("MNT_ID", a.mntID)
	params.Set("MNT_TRANSACTION_ID", transactionID)
	params.Set("MNT_AMOUNT", amount)
	params.Set("MNT_CURRENCY_CODE", currency)
	params.Set("MNT_SUBSCRIBER_ID", subscriberID)
	params.Set("MNT_TEST_MODE", testMode)
	params.Set("MNT_DESCRIPTION", truncate(firstNonEmpty(req.PaymentInfo.Description, "Payment "+transactionID), 255))
	params.Set("MNT_SIGNATURE", signature)

	unitID := firstNonEmpty(a.unitID, payAnyWayDefaultUnitID(req.PaymentInfo.PaymentMethodData.Type))
	if unitID != "" {
		params.Set("paymentSystem.unitId", unitID)
	}

	paymentURL := a.paymentURL
	separator := "?"
	if strings.Contains(paymentURL, "?") {
		separator = "&"
	}
	paymentURL += separator + params.Encode()

	providerStatus := "payment_url_created"
	if a.testMode {
		providerStatus = "test_payment_url_created"
	}

	return contracts.AdapterResult{
		ExternalTransactionID: transactionID,
		PaymentSystem:         "PAYANYWAY",
		Status:                string(dto.StatusPending),
		ProviderStatus:        providerStatus,
		PaymentURL:            paymentURL,
	}, nil
}

func payAnyWayPaymentSignature(mntID, transactionID, amount, currency, subscriberID, testMode, integrityKey string) string {
	raw := strings.TrimSpace(mntID) +
		strings.TrimSpace(transactionID) +
		strings.TrimSpace(amount) +
		strings.TrimSpace(currency) +
		strings.TrimSpace(subscriberID) +
		strings.TrimSpace(testMode) +
		integrityKey
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func payAnyWayDefaultUnitID(method dto.PaymentMethodType) string {
	switch method {
	case dto.PaymentMethodSBP:
		return "sbpc2b"
	case dto.PaymentMethodCard:
		return "card"
	default:
		return ""
	}
}

func payAnyWayBoolEnv(name string, defaultValue bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return defaultValue
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return defaultValue
	}
}
