package digitalruble

import (
	"fmt"
	"strings"
	"time"

	"payment-gateway/internal/dto"
)

const (
	DefaultSmartContractID = "SC_MARKED_MONEY_V1"
	PlatformTransportSOAP  = "SOAP/XML sandbox"
	SignatureTypeHMAC      = "HMAC-SHA256-EMULATION"

	markGeneral    = "GENERAL"
	markEducation  = "EDUCATION"
	markHealthcare = "HEALTHCARE"
	markFood       = "SOCIAL_FOOD"
	markTransport  = "TRANSPORT"
	markBudget     = "BUDGET_TARGETED"
)

type PaymentCheck struct {
	MessageID       string
	MerchantID      string
	PaymentID       string
	WalletID        string
	Amount          float64
	Currency        string
	Category        string
	MoneyMark       string
	SmartContractID string
}

type CheckResult struct {
	Allowed         bool
	Result          string
	Reason          string
	MoneyMark       string
	SmartContractID string
	WalletID        string
	AvailableAmount float64
	RequiredAmount  float64
	MessageID       string
}

func NewMessageID() string {
	return fmt.Sprintf("drmsg_%d", time.Now().UnixNano())
}

func PaymentCheckFromCreateRequest(req dto.CreatePaymentRequest, walletID string, messageID string) PaymentCheck {
	if messageID == "" {
		messageID = NewMessageID()
	}
	category := PrimaryCategory(req.PaymentInfo.Items)
	return PaymentCheck{
		MessageID:       messageID,
		MerchantID:      req.MerchantID,
		PaymentID:       req.PaymentID,
		WalletID:        strings.TrimSpace(walletID),
		Amount:          req.PaymentInfo.Amount.Value,
		Currency:        string(req.PaymentInfo.Amount.Currency),
		Category:        category,
		MoneyMark:       RequiredMoneyMark(category),
		SmartContractID: SmartContractID(req.PaymentInfo.DigitalRubleData.SmartContractID),
	}
}

func PaymentCheckFromResponse(resp dto.PaymentResponse, walletID string, messageID string) PaymentCheck {
	if messageID == "" {
		messageID = NewMessageID()
	}
	category := PrimaryCategory(resp.PaymentInfo.Items)
	mark := strings.TrimSpace(resp.TransactionDetails.MoneyMark)
	if mark == "" {
		mark = RequiredMoneyMark(category)
	}
	contractID := strings.TrimSpace(resp.TransactionDetails.SmartContractID)
	if contractID == "" {
		contractID = SmartContractID(resp.PaymentInfo.DigitalRubleData.SmartContractID)
	}
	return PaymentCheck{
		MessageID:       messageID,
		MerchantID:      resp.MerchantID,
		PaymentID:       resp.ID,
		WalletID:        strings.TrimSpace(walletID),
		Amount:          resp.PaymentInfo.Amount.Value,
		Currency:        string(resp.PaymentInfo.Amount.Currency),
		Category:        category,
		MoneyMark:       mark,
		SmartContractID: contractID,
	}
}

func PrimaryCategory(items []dto.PaymentItem) string {
	for _, item := range items {
		category := NormalizeCategory(item.Category)
		if category != "" {
			return category
		}
	}
	return "general"
}

func NormalizeCategory(category string) string {
	category = strings.ToLower(strings.TrimSpace(category))
	category = strings.ReplaceAll(category, " ", "_")
	category = strings.ReplaceAll(category, "-", "_")
	return category
}

func RequiredMoneyMark(category string) string {
	switch NormalizeCategory(category) {
	case "education", "edu", "books", "school", "university":
		return markEducation
	case "medicine", "medical", "health", "healthcare", "pharmacy":
		return markHealthcare
	case "food", "groceries", "social_food":
		return markFood
	case "transport", "taxi", "bus", "metro":
		return markTransport
	case "subsidy", "budget", "government", "state":
		return markBudget
	default:
		return markGeneral
	}
}

func SmartContractID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultSmartContractID
	}
	return value
}

func CheckMarkedMoney(check PaymentCheck) CheckResult {
	check.Category = PrimaryCategoryFromValue(check.Category)
	if check.MoneyMark == "" {
		check.MoneyMark = RequiredMoneyMark(check.Category)
	}
	check.SmartContractID = SmartContractID(check.SmartContractID)
	check.WalletID = strings.TrimSpace(check.WalletID)
	if check.MessageID == "" {
		check.MessageID = NewMessageID()
	}

	available := walletMarkedBalance(check.WalletID, check.MoneyMark)
	result := CheckResult{
		Allowed:         available >= check.Amount,
		MoneyMark:       check.MoneyMark,
		SmartContractID: check.SmartContractID,
		WalletID:        check.WalletID,
		AvailableAmount: available,
		RequiredAmount:  check.Amount,
		MessageID:       check.MessageID,
	}
	if result.Allowed {
		result.Result = "PASSED"
		result.Reason = fmt.Sprintf("wallet %s has %.2f digital rubles marked as %s for category %s", displayWallet(check.WalletID), available, check.MoneyMark, check.Category)
		return result
	}

	result.Result = "DECLINED"
	result.Reason = fmt.Sprintf("wallet %s has %.2f digital rubles marked as %s, required %.2f for category %s", displayWallet(check.WalletID), available, check.MoneyMark, check.Amount, check.Category)
	return result
}

func PrimaryCategoryFromValue(category string) string {
	category = NormalizeCategory(category)
	if category == "" {
		return "general"
	}
	return category
}

func walletMarkedBalance(walletID string, mark string) float64 {
	walletID = strings.ToLower(strings.TrimSpace(walletID))
	mark = strings.ToUpper(strings.TrimSpace(mark))
	if walletID == "" {
		walletID = "dr_wallet_123"
	}

	balances := map[string]map[string]float64{
		"dr_wallet_123": {
			markGeneral:    100000,
			markEducation:  10000,
			markHealthcare: 5000,
			markFood:       3000,
			markTransport:  2000,
			markBudget:     1000,
		},
		"dr_wallet_no_mark": {
			markGeneral: 100000,
		},
		"dr_wallet_insufficient_marked": {
			markGeneral:   100000,
			markEducation: 100,
		},
		"dr_wallet_healthcare": {
			markGeneral:    10000,
			markHealthcare: 20000,
		},
	}

	wallet, ok := balances[walletID]
	if !ok {
		wallet = map[string]float64{markGeneral: 100000}
	}
	return wallet[mark]
}

func displayWallet(walletID string) string {
	walletID = strings.TrimSpace(walletID)
	if walletID == "" {
		return "default_wallet"
	}
	return walletID
}
