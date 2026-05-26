package digitalruble

import (
	"strings"
	"testing"
)

func TestCheckMarkedMoneyAllowsMatchingMark(t *testing.T) {
	result := CheckMarkedMoney(PaymentCheck{
		MessageID:       "msg_test",
		MerchantID:      "merchant_12345",
		PaymentID:       "pay_test",
		WalletID:        "dr_wallet_123",
		Amount:          1500,
		Currency:        "RUB",
		Category:        "education",
		SmartContractID: DefaultSmartContractID,
	})

	if !result.Allowed {
		t.Fatalf("expected marked money check to pass: %+v", result)
	}
	if result.MoneyMark != "EDUCATION" {
		t.Fatalf("expected EDUCATION mark, got %s", result.MoneyMark)
	}
}

func TestCheckMarkedMoneyRejectsWalletWithoutRequiredMark(t *testing.T) {
	result := CheckMarkedMoney(PaymentCheck{
		MessageID:       "msg_test",
		MerchantID:      "merchant_12345",
		PaymentID:       "pay_test",
		WalletID:        "dr_wallet_no_mark",
		Amount:          1500,
		Currency:        "RUB",
		Category:        "education",
		SmartContractID: DefaultSmartContractID,
	})

	if result.Allowed {
		t.Fatalf("expected marked money check to fail: %+v", result)
	}
	if result.Result != "DECLINED" {
		t.Fatalf("expected DECLINED result, got %s", result.Result)
	}
}

func TestSOAPPaymentCheckRoundTrip(t *testing.T) {
	requestXML, err := BuildPaymentCheckSOAP(PaymentCheck{
		MessageID:       "msg_test",
		MerchantID:      "merchant_12345",
		PaymentID:       "pay_test",
		WalletID:        "dr_wallet_123",
		Amount:          1500,
		Currency:        "RUB",
		Category:        "education",
		SmartContractID: DefaultSmartContractID,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, responseXML, err := ProcessPaymentCheckSOAP(requestXML)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed {
		t.Fatalf("expected SOAP check to pass: %+v", result)
	}
	if !strings.Contains(string(responseXML), "<Result>PASSED</Result>") {
		t.Fatalf("expected PASSED SOAP response, got %s", responseXML)
	}
}
