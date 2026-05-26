package digitalruble

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

const soapNamespace = "http://schemas.xmlsoap.org/soap/envelope/"

type SOAPEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	XMLNS   string   `xml:"xmlns,attr,omitempty"`
	Body    SOAPBody `xml:"Body"`
}

type SOAPBody struct {
	BusinessEnvelope SOAPBusinessEnvelope `xml:"BusinessEnvelope"`
}

type SOAPBusinessEnvelope struct {
	MessageID    string                 `xml:"MessageID"`
	MessageType  string                 `xml:"MessageType"`
	Sender       string                 `xml:"Sender"`
	Receiver     string                 `xml:"Receiver"`
	SigContainer SOAPSignatureContainer `xml:"SigContainer"`
	Object       SOAPPaymentObject      `xml:"Object"`
}

type SOAPSignatureContainer struct {
	SignatureType  string `xml:"SignatureType"`
	SignatureValue string `xml:"SignatureValue"`
}

type SOAPPaymentObject struct {
	PaymentID       string `xml:"PaymentID,omitempty"`
	MerchantID      string `xml:"MerchantID,omitempty"`
	WalletID        string `xml:"WalletID,omitempty"`
	Amount          string `xml:"Amount,omitempty"`
	Currency        string `xml:"Currency,omitempty"`
	Category        string `xml:"Category,omitempty"`
	RequiredMark    string `xml:"RequiredMoneyMark,omitempty"`
	SmartContractID string `xml:"SmartContractID,omitempty"`
	ResultCode      string `xml:"ResultCode,omitempty"`
	Result          string `xml:"Result,omitempty"`
	Reason          string `xml:"Reason,omitempty"`
}

func BuildPaymentCheckSOAP(check PaymentCheck) ([]byte, error) {
	check = normalizePaymentCheck(check)
	envelope := SOAPEnvelope{
		XMLNS: soapNamespace,
		Body: SOAPBody{
			BusinessEnvelope: SOAPBusinessEnvelope{
				MessageID:   check.MessageID,
				MessageType: "C2BPaymentCheck",
				Sender:      "PAYMENT_GATEWAY",
				Receiver:    "DIGITAL_RUBLE_PLATFORM_EMULATOR",
				SigContainer: SOAPSignatureContainer{
					SignatureType:  SignatureTypeHMAC,
					SignatureValue: SignPaymentCheck(check),
				},
				Object: SOAPPaymentObject{
					PaymentID:       check.PaymentID,
					MerchantID:      check.MerchantID,
					WalletID:        check.WalletID,
					Amount:          fmt.Sprintf("%.2f", check.Amount),
					Currency:        check.Currency,
					Category:        check.Category,
					RequiredMark:    check.MoneyMark,
					SmartContractID: check.SmartContractID,
				},
			},
		},
	}
	body, err := xml.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), body...), nil
}

func ProcessPaymentCheckSOAP(body []byte) (CheckResult, []byte, error) {
	var envelope SOAPEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return CheckResult{}, nil, err
	}
	check, err := paymentCheckFromSOAPEnvelope(envelope)
	if err != nil {
		return CheckResult{}, nil, err
	}

	result := CheckMarkedMoney(check)
	response, err := BuildPaymentCheckSOAPResponse(check, result)
	if err != nil {
		return CheckResult{}, nil, err
	}
	return result, response, nil
}

func BuildPaymentCheckSOAPResponse(check PaymentCheck, result CheckResult) ([]byte, error) {
	resultCode := "200"
	if !result.Allowed {
		resultCode = "409"
	}

	envelope := SOAPEnvelope{
		XMLNS: soapNamespace,
		Body: SOAPBody{
			BusinessEnvelope: SOAPBusinessEnvelope{
				MessageID:   check.MessageID + "_response",
				MessageType: "C2BPaymentCheckResponse",
				Sender:      "DIGITAL_RUBLE_PLATFORM_EMULATOR",
				Receiver:    "PAYMENT_GATEWAY",
				SigContainer: SOAPSignatureContainer{
					SignatureType:  SignatureTypeHMAC,
					SignatureValue: SignSOAPResponse(check.MessageID, resultCode, result.Result),
				},
				Object: SOAPPaymentObject{
					PaymentID:       check.PaymentID,
					MerchantID:      check.MerchantID,
					WalletID:        check.WalletID,
					Amount:          fmt.Sprintf("%.2f", check.Amount),
					Currency:        check.Currency,
					Category:        check.Category,
					RequiredMark:    result.MoneyMark,
					SmartContractID: result.SmartContractID,
					ResultCode:      resultCode,
					Result:          result.Result,
					Reason:          result.Reason,
				},
			},
		},
	}
	body, err := xml.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), body...), nil
}

func SignPaymentCheck(check PaymentCheck) string {
	check = normalizePaymentCheck(check)
	value := strings.Join([]string{
		check.MessageID,
		check.PaymentID,
		check.MerchantID,
		check.WalletID,
		fmt.Sprintf("%.2f", check.Amount),
		check.Currency,
		check.Category,
		check.MoneyMark,
		check.SmartContractID,
	}, "|")
	return hmacHex(value)
}

func SignSOAPResponse(messageID string, resultCode string, result string) string {
	return hmacHex(strings.Join([]string{messageID, resultCode, result}, "|"))
}

func paymentCheckFromSOAPEnvelope(envelope SOAPEnvelope) (PaymentCheck, error) {
	business := envelope.Body.BusinessEnvelope
	object := business.Object
	amount, err := strconv.ParseFloat(strings.TrimSpace(object.Amount), 64)
	if err != nil {
		return PaymentCheck{}, fmt.Errorf("invalid SOAP payment amount: %w", err)
	}
	check := PaymentCheck{
		MessageID:       strings.TrimSpace(business.MessageID),
		MerchantID:      strings.TrimSpace(object.MerchantID),
		PaymentID:       strings.TrimSpace(object.PaymentID),
		WalletID:        strings.TrimSpace(object.WalletID),
		Amount:          amount,
		Currency:        strings.ToUpper(strings.TrimSpace(object.Currency)),
		Category:        PrimaryCategoryFromValue(object.Category),
		MoneyMark:       strings.ToUpper(strings.TrimSpace(object.RequiredMark)),
		SmartContractID: SmartContractID(object.SmartContractID),
	}
	if check.MessageID == "" || check.PaymentID == "" || check.MerchantID == "" {
		return PaymentCheck{}, fmt.Errorf("SOAP message_id, merchant_id and payment_id are required")
	}
	return normalizePaymentCheck(check), nil
}

func normalizePaymentCheck(check PaymentCheck) PaymentCheck {
	if check.MessageID == "" {
		check.MessageID = NewMessageID()
	}
	check.MerchantID = strings.TrimSpace(check.MerchantID)
	check.PaymentID = strings.TrimSpace(check.PaymentID)
	check.WalletID = strings.TrimSpace(check.WalletID)
	check.Currency = strings.ToUpper(strings.TrimSpace(check.Currency))
	if check.Currency == "" {
		check.Currency = "RUB"
	}
	check.Category = PrimaryCategoryFromValue(check.Category)
	check.MoneyMark = strings.ToUpper(strings.TrimSpace(check.MoneyMark))
	if check.MoneyMark == "" {
		check.MoneyMark = RequiredMoneyMark(check.Category)
	}
	check.SmartContractID = SmartContractID(check.SmartContractID)
	return check
}

func hmacHex(value string) string {
	mac := hmac.New(sha256.New, []byte("digital-ruble-soap-sandbox-secret"))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}
