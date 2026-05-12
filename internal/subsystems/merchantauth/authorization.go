package merchantauth

import (
	"context"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
)

func CanReadMerchantData(merchant Merchant, targetMerchantID string) bool {
	targetMerchantID = strings.TrimSpace(targetMerchantID)
	if targetMerchantID == "" {
		return false
	}

	switch NormalizeRole(merchant.Role) {
	case RoleAdmin, RoleAuditor:
		return true
	case RoleMerchant:
		return merchant.MerchantID == targetMerchantID
	default:
		return false
	}
}

func CanWriteMerchantData(merchant Merchant, targetMerchantID string) bool {
	targetMerchantID = strings.TrimSpace(targetMerchantID)
	if targetMerchantID == "" {
		return false
	}

	switch NormalizeRole(merchant.Role) {
	case RoleAdmin:
		return true
	case RoleMerchant:
		return merchant.MerchantID == targetMerchantID
	default:
		return false
	}
}

func LogAuthorizationFailed(
	ctx context.Context,
	logger contracts.EventLogger,
	merchant Merchant,
	targetMerchantID string,
	endpoint string,
	reason string,
) {
	if logger == nil {
		return
	}
	role := NormalizeRole(merchant.Role)
	if role == "" {
		role = "unknown"
	}
	_ = logger.Log(ctx, contracts.PaymentEvent{
		Type:          contracts.EventAuthorizationFailed,
		Level:         contracts.LogLevelWarn,
		Service:       "api_gateway",
		MerchantID:    merchant.MerchantID,
		CorrelationID: merchant.MerchantID,
		Timestamp:     time.Now().UTC(),
		Message:       "Authorization failed",
		Details:       reason,
		Context: map[string]string{
			"role":                  string(role),
			"target_merchant_id":    strings.TrimSpace(targetMerchantID),
			"endpoint":              endpoint,
			"authorization_failure": reason,
		},
	})
}
