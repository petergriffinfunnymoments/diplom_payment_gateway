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
	f.Register("simulated", NewSimulatedAdapter("SIMULATED"))
	f.Register("digital_ruble", NewDigitalRubleAdapterFromEnv())
	return f
}

func NewFactoryFromEnv() *Factory {
	f := NewFactory()

	if a, err := NewYooKassaAdapterFromEnv(); err == nil {
		f.Register("yookassa", a)
	}

	if a, err := NewRobokassaAdapterFromEnv(); err == nil {
		f.Register("robokassa", a)
	}

	if a, err := NewPayAnyWayAdapterFromEnv(); err == nil {
		f.Register("payanyway", a)
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

// Resolve возвращает конкретный адаптер.
// Если маршрутизатор уже выбрал provider (например, yookassa или payanyway), фабрика просто возвращает
// соответствующую реализацию. Для старых adapterKey вида card_adapter сохраняется fallback через env.
func (f *Factory) Resolve(adapterKey string, paymentSystem string) (contracts.PaymentAdapter, string, error) {
	key := normalizeAdapterKey(adapterKey)

	// Новый режим: router возвращает provider напрямую.
	if key != "" && !isLegacyAdapterKey(key) {
		if a, ok := f.Get(key); ok {
			return a, key, nil
		}
		return nil, "", fmt.Errorf("adapter provider %q is not registered or not configured", key)
	}

	// Старый режим/fallback: выбираем provider из env.
	providerKey := providerFromEnv(key, paymentSystem)
	if providerKey == "" {
		return nil, "", fmt.Errorf("payment provider is not configured for adapter key %q", adapterKey)
	}

	if a, ok := f.Get(providerKey); ok {
		return a, providerKey, nil
	}

	return nil, "", fmt.Errorf("adapter provider %q is not registered or not configured", providerKey)
}

func providerFromEnv(adapterKey string, paymentSystem string) string {
	_ = paymentSystem
	adapterKey = normalizeAdapterKey(adapterKey)

	var envName string
	switch adapterKey {
	case "card_adapter":
		envName = "CARD_PAYMENT_PROVIDER"
	case "sbp_adapter":
		envName = "SBP_PAYMENT_PROVIDER"
	case "wallet_adapter":
		envName = "WALLET_PAYMENT_PROVIDER"
	case "digital_ruble_adapter":
		envName = "DIGITAL_RUBLE_PAYMENT_PROVIDER"
	}

	if envName != "" {
		if v := normalizeAdapterKey(os.Getenv(envName)); v != "" {
			return v
		}
	}

	if v := normalizeAdapterKey(os.Getenv("PAYMENT_PROVIDER")); v != "" {
		return v
	}

	return ""
}

func isLegacyAdapterKey(v string) bool {
	switch normalizeAdapterKey(v) {
	case "card_adapter", "sbp_adapter", "wallet_adapter", "digital_ruble_adapter":
		return true
	default:
		return false
	}
}

func normalizeAdapterKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
