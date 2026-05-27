package logging

import (
	"regexp"
	"strings"
)

var (
	panCandidateRegexp = regexp.MustCompile(`\b\d{13,19}\b`)
	cvvJSONRegexp      = regexp.MustCompile(`(?i)("?(?:CVV_code|cvv|cvc|cid)"?\s*[:=]\s*"?)[0-9]{3,4}("?)*`)
	secretRegexp       = regexp.MustCompile(`(?i)\b(YOOKASSA_SECRET_KEY\s*[:=]\s*\S+|PAYANYWAY_INTEGRITY_CODE\s*[:=]\s*\S+)\b`)
)

func MaskCardNumber(card string) string {
	card = digitsOnly(card)
	if len(card) < 10 {
		return ""
	}
	return card[:6] + strings.Repeat("*", len(card)-10) + card[len(card)-4:]
}

func MaskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) < 6 {
		return ""
	}
	return phone[:5] + strings.Repeat("*", max(0, len(phone)-7)) + phone[len(phone)-2:]
}

func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.Index(email, "@")
	if at <= 0 {
		return ""
	}
	name := email[:at]
	domain := email[at:]
	if len(name) == 1 {
		return name + "***" + domain
	}
	return name[:1] + "***" + domain
}

func TokenPreview(token string) string {
	token = strings.TrimSpace(token)
	if len(token) <= 8 {
		return ""
	}
	return token[:8] + "..."
}

func MaskSensitive(value string) string {
	if value == "" {
		return ""
	}
	value = cvvJSONRegexp.ReplaceAllString(value, `${1}[REDACTED]${2}`)
	value = secretRegexp.ReplaceAllString(value, "[REDACTED_SECRET]")
	value = panCandidateRegexp.ReplaceAllStringFunc(value, func(candidate string) string {
		if !luhnValid(candidate) {
			return candidate
		}
		return MaskCardNumber(candidate)
	})
	return value
}

func digitsOnly(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func luhnValid(number string) bool {
	sum := 0
	double := false
	for i := len(number) - 1; i >= 0; i-- {
		digit := int(number[i] - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return sum%10 == 0
}
