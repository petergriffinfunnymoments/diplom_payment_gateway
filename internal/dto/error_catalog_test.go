package dto

import "testing"

func TestGatewayErrorCatalogHasUniqueCodes(t *testing.T) {
	seen := map[string]bool{}
	for _, def := range GatewayErrorCatalog() {
		if def.Code == "" {
			t.Fatal("catalog contains empty code")
		}
		if seen[def.Code] {
			t.Fatalf("duplicate error code: %s", def.Code)
		}
		seen[def.Code] = true
		if def.Category == "" {
			t.Fatalf("error %s has empty category", def.Code)
		}
		if def.DefaultMessage == "" {
			t.Fatalf("error %s has empty default message", def.Code)
		}
	}
}

func TestNewGatewayErrorUsesDefaultMessage(t *testing.T) {
	err := NewGatewayError(ErrorAntifraudDeclined, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Code != ErrorAntifraudDeclined {
		t.Fatalf("unexpected code: %s", err.Code)
	}
	if err.Message == "" {
		t.Fatal("expected default message")
	}
}

func TestGatewayErrorDefinitionByCodeNormalizesInput(t *testing.T) {
	def, ok := GatewayErrorDefinitionByCode(" payment_declined ")
	if !ok {
		t.Fatal("expected payment_declined to be found")
	}
	if def.Code != ErrorPaymentDeclined {
		t.Fatalf("unexpected code: %s", def.Code)
	}
}
