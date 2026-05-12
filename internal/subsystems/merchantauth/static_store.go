package merchantauth

import (
	"context"
	"os"
)

type StaticMerchantStore struct {
	merchant Merchant
}

func NewStaticMerchantStoreFromEnv() *StaticMerchantStore {
	merchantID := getenvStatic("MERCHANT_ID", "merchant_12345")
	apiKey := getenvStatic("MERCHANT_API_KEY", "demo_api_key")
	secretKey := getenvStatic("MERCHANT_SECRET_KEY", "demo_secret_key")

	return &StaticMerchantStore{
		merchant: Merchant{
			MerchantID: merchantID,
			Name:       getenvStatic("MERCHANT_NAME", "Демонстрационный интернет-магазин"),
			APIKeyHash: sha256Hex(apiKey),
			SecretKey:  secretKey,
			Role:       MerchantRole(getenvStatic("MERCHANT_ROLE", string(RoleMerchant))),
			Active:     true,
		},
	}
}

func (s *StaticMerchantStore) GetByID(ctx context.Context, merchantID string) (Merchant, bool, error) {
	_ = ctx
	if s == nil || merchantID != s.merchant.MerchantID {
		return Merchant{}, false, nil
	}
	return s.merchant, true, nil
}

func getenvStatic(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
