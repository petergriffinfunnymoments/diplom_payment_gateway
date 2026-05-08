package adapter

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type DigitalRubleAdapter struct {
	participantBank string
	schemaVersion   string
	qrTTL           time.Duration
}

func NewDigitalRubleAdapterFromEnv() contracts.PaymentAdapter {
	participantBank := strings.TrimSpace(os.Getenv("DIGITAL_RUBLE_PARTICIPANT_BANK"))
	if participantBank == "" {
		participantBank = "BANK_PARTNER_1"
	}

	schemaVersion := strings.TrimSpace(os.Getenv("DIGITAL_RUBLE_SCHEMA_VERSION"))
	if schemaVersion == "" {
		schemaVersion = "drub.v1"
	}

	ttl := 15 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("DIGITAL_RUBLE_QR_TTL_SECONDS")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			ttl = time.Duration(seconds) * time.Second
		}
	}

	return &DigitalRubleAdapter{
		participantBank: participantBank,
		schemaVersion:   schemaVersion,
		qrTTL:           ttl,
	}
}

func (a *DigitalRubleAdapter) Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (contracts.AdapterResult, error) {
	_ = ctx
	_ = token

	walletID := digitalRubleWalletID(req)
	qrID := fmt.Sprintf("drqr_%d", time.Now().UnixNano())
	externalID := fmt.Sprintf("drub_%d", time.Now().UnixNano())
	expiresAt := time.Now().UTC().Add(a.qrTTL)

	status, providerStatus, errMsg := emulateDigitalRubleStatus(walletID)
	return contracts.AdapterResult{
		ExternalTransactionID: externalID,
		PaymentSystem:         "DIGITAL_RUBLE",
		Status:                status,
		ProviderStatus:        providerStatus,
		PaymentURL:            "digital-ruble://pay?" + digitalRubleQuery(qrID, req),
		QRID:                  qrID,
		QRPayload:             a.qrPayload(qrID, req, walletID),
		QRExpiresAt:           expiresAt,
		ParticipantBank:       a.participantBank,
		SchemaVersion:         a.schemaVersion,
		SettlementHint:        "RUB + DIGITAL_RUBLE; settlement through participant bank emulator",
		ErrorMessage:          errMsg,
	}, nil
}

func (a *DigitalRubleAdapter) Refund(ctx context.Context, req contracts.RefundRequest) (contracts.RefundResult, error) {
	_ = ctx

	status := string(dto.RefundStatusSuccess)
	providerStatus := "reversed"
	errMsg := ""
	if strings.Contains(strings.ToLower(req.Reason), "simulate_failed") {
		status = string(dto.RefundStatusFail)
		providerStatus = "refund_failed"
		errMsg = "digital ruble refund emulation failed"
	}

	return contracts.RefundResult{
		ExternalRefundID: fmt.Sprintf("drrf_%d", time.Now().UnixNano()),
		PaymentSystem:    "DIGITAL_RUBLE",
		Status:           status,
		ProviderStatus:   providerStatus,
		ErrorMessage:     errMsg,
	}, nil
}

func (a *DigitalRubleAdapter) qrPayload(qrID string, req dto.CreatePaymentRequest, walletID string) string {
	values := url.Values{}
	values.Set("type", "UNIVERSAL_QR")
	values.Set("rail", "DIGITAL_RUBLE")
	values.Set("schema", a.schemaVersion)
	values.Set("participant_bank", a.participantBank)
	values.Set("qr_id", qrID)
	values.Set("merchant_id", req.MerchantID)
	values.Set("payment_id", req.PaymentID)
	values.Set("amount", fmt.Sprintf("%.2f", req.PaymentInfo.Amount.Value))
	values.Set("currency", string(req.PaymentInfo.Amount.Currency))
	values.Set("payer_wallet", walletID)
	values.Set("recipient_account", strings.TrimSpace(req.PaymentInfo.CustomerData.DigitalRubleAccount))
	values.Set("recipient_identifier", strings.TrimSpace(req.PaymentInfo.CustomerData.DigitalRubleIdentifier))
	values.Set("purpose", truncate(req.PaymentInfo.Description, 140))
	return "drub://" + values.Encode()
}

func digitalRubleQuery(qrID string, req dto.CreatePaymentRequest) string {
	values := url.Values{}
	values.Set("qr_id", qrID)
	values.Set("payment_id", req.PaymentID)
	values.Set("merchant_id", req.MerchantID)
	return values.Encode()
}

func digitalRubleWalletID(req dto.CreatePaymentRequest) string {
	customer := req.PaymentInfo.CustomerData
	switch {
	case strings.TrimSpace(customer.DigitalRubleWalletID) != "":
		return strings.TrimSpace(customer.DigitalRubleWalletID)
	case strings.TrimSpace(customer.DigitalRubleIdentifier) != "":
		return strings.TrimSpace(customer.DigitalRubleIdentifier)
	default:
		return strings.TrimSpace(customer.DigitalWalletID)
	}
}

func emulateDigitalRubleStatus(walletID string) (status string, providerStatus string, errMsg string) {
	switch strings.ToLower(strings.TrimSpace(walletID)) {
	case "dr_wallet_123":
		return string(dto.StatusCaptured), "settled", ""
	case "dr_wallet_declined":
		return string(dto.StatusDeclined), "participant_rejected", "digital ruble payment rejected by participant bank emulator"
	case "dr_wallet_error":
		return string(dto.StatusFailed), "technical_error", "digital ruble participant bank emulator returned technical error"
	case "dr_wallet_pending":
		return string(dto.StatusPending), "awaiting_customer_confirmation", ""
	default:
		return string(dto.StatusPending), "qr_issued", ""
	}
}
