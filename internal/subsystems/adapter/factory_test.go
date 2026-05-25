package adapter

import "testing"

func TestFactoryResolveRequiresConfiguredProvider(t *testing.T) {
	factory := NewFactory()

	_, provider, err := factory.Resolve("card_adapter", "CARD")
	if err == nil {
		t.Fatal("expected error when provider is not configured")
	}
	if provider != "" {
		t.Fatalf("provider = %q, want empty", provider)
	}
}

func TestFactoryResolveLegacyAdapterFromEnv(t *testing.T) {
	t.Setenv("SBP_PAYMENT_PROVIDER", "simulated")

	factory := NewFactory()

	adapter, provider, err := factory.Resolve("sbp_adapter", "SBP")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if adapter == nil {
		t.Fatal("adapter is nil")
	}
	if provider != "simulated" {
		t.Fatalf("provider = %q, want simulated", provider)
	}
}
