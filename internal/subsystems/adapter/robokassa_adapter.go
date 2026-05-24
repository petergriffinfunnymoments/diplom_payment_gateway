package adapter

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"sort"
	"strings"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type RobokassaAdapter struct {
	merchantLogin string
	password1     string
	paymentURL    string
	hashAlgorithm string
	testMode      bool
}

func NewRobokassaAdapterFromEnv() (contracts.PaymentAdapter, error) {
	merchantLogin := firstNonEmpty(
		os.Getenv("ROBOKASSA_MERCHANT_LOGIN"),
		os.Getenv("ROBOKASSA_LOGIN"),
	)
	if strings.TrimSpace(merchantLogin) == "" {
		return nil, errors.New("ROBOKASSA_MERCHANT_LOGIN is required")
	}

	testMode := robokassaBoolEnv("ROBOKASSA_TEST_MODE", true)
	password1 := strings.TrimSpace(os.Getenv("ROBOKASSA_PASSWORD1"))
	if testMode {
		password1 = firstNonEmpty(os.Getenv("ROBOKASSA_TEST_PASSWORD1"), password1)
	}
	if password1 == "" {
		return nil, errors.New("ROBOKASSA_PASSWORD1 or ROBOKASSA_TEST_PASSWORD1 is required")
	}

	paymentURL := strings.TrimSpace(os.Getenv("ROBOKASSA_PAYMENT_URL"))
	if paymentURL == "" {
		paymentURL = "https://auth.robokassa.ru/Merchant/Index.aspx"
	}

	hashAlgorithm := strings.TrimSpace(os.Getenv("ROBOKASSA_HASH_ALGORITHM"))
	if hashAlgorithm == "" {
		hashAlgorithm = "md5"
	}

	return &RobokassaAdapter{
		merchantLogin: strings.TrimSpace(merchantLogin),
		password1:     password1,
		paymentURL:    paymentURL,
		hashAlgorithm: hashAlgorithm,
		testMode:      testMode,
	}, nil
}

func (a *RobokassaAdapter) Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (contracts.AdapterResult, error) {
	_ = ctx
	_ = token

	outSum := fmt.Sprintf("%.2f", req.PaymentInfo.Amount.Value)
	invID := robokassaInvoiceID(req.MerchantID, req.PaymentID, req.IdempotencyKey)
	description := strings.TrimSpace(req.PaymentInfo.Description)
	if description == "" {
		description = "Payment " + req.PaymentID
	}

	shp := map[string]string{
		"Shp_idempotency_key": req.IdempotencyKey,
		"Shp_merchant_id":     req.MerchantID,
		"Shp_payment_id":      req.PaymentID,
	}

	signature, err := robokassaSignature(a.hashAlgorithm, a.merchantLogin, outSum, invID, a.password1, nil, shp)
	if err != nil {
		return contracts.AdapterResult{}, err
	}

	params := url.Values{}
	params.Set("MerchantLogin", a.merchantLogin)
	params.Set("OutSum", outSum)
	params.Set("InvId", invID)
	params.Set("Description", truncate(description, 100))
	params.Set("SignatureValue", signature)
	if a.testMode {
		params.Set("IsTest", "1")
	} else {
		params.Set("IsTest", "0")
	}
	for k, v := range shp {
		params.Set(k, v)
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
		ExternalTransactionID: invID,
		PaymentSystem:         "ROBOKASSA",
		Status:                string(dto.StatusPending),
		ProviderStatus:        providerStatus,
		PaymentURL:            paymentURL,
	}, nil
}

func robokassaInvoiceID(values ...string) string {
	h := fnv.New64a()
	for _, value := range values {
		_, _ = h.Write([]byte(strings.TrimSpace(value)))
		_, _ = h.Write([]byte{0})
	}
	const maxInt64 = uint64(^uint64(0) >> 1)
	n := h.Sum64() & maxInt64
	if n == 0 {
		n = 1
	}
	return fmt.Sprintf("%d", n)
}

func robokassaSignature(
	algorithm string,
	merchantLogin string,
	outSum string,
	invID string,
	password string,
	modifiers []string,
	shp map[string]string,
) (string, error) {
	parts := []string{
		strings.TrimSpace(merchantLogin),
		strings.TrimSpace(outSum),
		strings.TrimSpace(invID),
	}
	for _, modifier := range modifiers {
		parts = append(parts, modifier)
	}
	parts = append(parts, password)

	keys := make([]string, 0, len(shp))
	for key := range shp {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key+"="+shp[key])
	}

	return robokassaHash(algorithm, strings.Join(parts, ":"))
}

func robokassaHash(algorithm string, value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case "", "md5":
		sum := md5.Sum([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	case "sha1":
		sum := sha1.Sum([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	case "sha256":
		sum := sha256.Sum256([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	case "sha384":
		sum := sha512.Sum384([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	case "sha512":
		sum := sha512.Sum512([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	default:
		return "", fmt.Errorf("unsupported Robokassa hash algorithm %q", algorithm)
	}
}

func robokassaBoolEnv(name string, defaultValue bool) bool {
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
