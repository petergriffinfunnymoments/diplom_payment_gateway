package simple

import (
	"strconv"
	"strings"
)

const (
	cardSchemeUnknown    = "UNKNOWN"
	cardSchemeMir        = "MIR"
	cardSchemeVisa       = "VISA"
	cardSchemeMastercard = "MASTERCARD"
)

// detectCardScheme определяет платёжную систему карты по первым цифрам номера.
// Это учебная BIN/IIN-маршрутизация: для промышленного шлюза лучше подключать полноценную BIN-базу.
func detectCardScheme(cardNumber string) string {
	digits := onlyDigits(cardNumber)
	if len(digits) < 4 {
		return cardSchemeUnknown
	}

	prefix4, err := strconv.Atoi(digits[:4])
	if err == nil && prefix4 >= 2200 && prefix4 <= 2204 {
		return cardSchemeMir
	}

	if strings.HasPrefix(digits, "4") {
		return cardSchemeVisa
	}

	if len(digits) >= 2 {
		prefix2, err := strconv.Atoi(digits[:2])
		if err == nil && prefix2 >= 51 && prefix2 <= 55 {
			return cardSchemeMastercard
		}
	}

	if err == nil && prefix4 >= 2221 && prefix4 <= 2720 {
		return cardSchemeMastercard
	}

	return cardSchemeUnknown
}

func providerForCardScheme(scheme string) (paymentSystem string, adapterKey string, ok bool) {
	switch strings.ToUpper(strings.TrimSpace(scheme)) {
	case cardSchemeMir:
		return "YOOKASSA", "yookassa", true
	case cardSchemeVisa, cardSchemeMastercard:
		return "STRIPE", "stripe", true
	default:
		return "", "", false
	}
}

func onlyDigits(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
