package adapter

import (
	"fmt"
	"os"
	"strings"

	"payment-gateway/internal/contracts"
)

type Factory struct {
	adapters map[string]contracts.PaymentAdapter
}

func NewFactory() *Factory {
	f := &Factory{adapters: map[string]contracts.PaymentAdapter{}}
	f.Register("dummy", NewDummyAdapter("DUMMY"))
	return f
}

func NewFactoryFromEnv() *Factory {
	f := NewFactory()

	if a, err := NewYooKassaAdapterFromEnv(); err == nil {
		f.Register("yookassa", a)
	}

	if a, err := NewTBankAdapterFromEnv(); err == nil {
		f.Register("tbank", a)
	}

	if a, err := NewStripeAdapterFromEnv(); err == nil {
		f.Register("stripe", a)
	}

	return f
}

func (f *Factory) Register(key string, adapter contracts.PaymentAdapter) {
	if f.adapters == nil {
		f.adapters = map[string]contracts.PaymentAdapter{}
	}
	key = normalizeAdapterKey(key)
	if key == "" || adapter == nil {
		return
	}
	f.adapters[key] = adapter
}

func (f *Factory) Get(key string) (contracts.PaymentAdapter, bool) {
	if f == nil || f.adapters == nil {
		return nil, false
	}
	a, ok := f.adapters[normalizeAdapterKey(key)]
	return a, ok
}

func (f *Factory) Resolve(adapterKey string, paymentSystem string) (contracts.PaymentAdapter, string, error) {
	providerKey := providerFromEnv(adapterKey, paymentSystem)
	if a, ok := f.Get(providerKey); ok {
		return a, providerKey, nil
	}

	if a, ok := f.Get("dummy"); ok {
		return a, "dummy", nil
	}

	return nil, "", fmt.Errorf("adapter provider %q is not registered", providerKey)
}

func providerFromEnv(adapterKey string, paymentSystem string) string {
	adapterKey = normalizeAdapterKey(adapterKey)
	paymentSystem = strings.ToUpper(strings.TrimSpace(paymentSystem))

	var envName string
	switch adapterKey {
	case "card_adapter":
		envName = "CARD_PAYMENT_PROVIDER"
	case "sbp_adapter":
		envName = "SBP_PAYMENT_PROVIDER"
	case "wallet_adapter":
		envName = "WALLET_PAYMENT_PROVIDER"
	}

	if envName != "" {
		if v := normalizeAdapterKey(os.Getenv(envName)); v != "" {
			return v
		}
	}

	if v := normalizeAdapterKey(os.Getenv("PAYMENT_PROVIDER")); v != "" {
		return v
	}

	if paymentSystem != "" {
		return strings.ToLower(paymentSystem)
	}
	return "dummy"
}

func normalizeAdapterKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
