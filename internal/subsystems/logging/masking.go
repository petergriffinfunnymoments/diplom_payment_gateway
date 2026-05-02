package logging

import "strings"

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

func digitsOnly(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
