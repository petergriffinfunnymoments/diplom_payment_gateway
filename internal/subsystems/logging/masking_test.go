package logging

import (
	"strings"
	"testing"
)

func TestMaskSensitiveRedactsPANAndCVV(t *testing.T) {
	value := `{"card_number":"4111111111111111","CVV_code":"123","note":"ok"}`
	masked := MaskSensitive(value)

	if strings.Contains(masked, "4111111111111111") {
		t.Fatalf("PAN was not redacted: %s", masked)
	}
	if strings.Contains(masked, `"CVV_code":"123"`) {
		t.Fatalf("CVV was not redacted: %s", masked)
	}
	if !strings.Contains(masked, "411111******1111") {
		t.Fatalf("PAN mask missing: %s", masked)
	}
}
