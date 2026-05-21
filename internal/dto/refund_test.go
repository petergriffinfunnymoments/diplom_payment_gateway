package dto

import "testing"

func TestNormalizeRefundStatus(t *testing.T) {
	tests := map[string]string{
		"PROCESS_REFUND": "PROCESS_REFUND",
		"SUCCESS_REFUND": "SUCCESS_REFUND",
		"FAIL_REFUND":    "FAIL_REFUND",
		"PROCESS":        "PROCESS_REFUND",
		"SUCCESS":        "SUCCESS_REFUND",
		"FAIL":           "FAIL_REFUND",
		"pending":        "PROCESS_REFUND",
		"succeeded":      "SUCCESS_REFUND",
		"failed":         "FAIL_REFUND",
		"canceled":       "FAIL_REFUND",
		"":               "PROCESS_REFUND",
	}

	for input, want := range tests {
		if got := NormalizeRefundStatus(input); got != want {
			t.Fatalf("NormalizeRefundStatus(%q) = %q, want %q", input, got, want)
		}
	}
}
