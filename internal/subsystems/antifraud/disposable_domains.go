package antifraud

import (
	_ "embed"
	"strings"
)

//go:embed disposable_email_domains.txt
var disposableEmailDomainsRaw string

var disposableEmailDomains = loadDisposableEmailDomains(disposableEmailDomainsRaw)

func loadDisposableEmailDomains(raw string) map[string]struct{} {
	domains := make(map[string]struct{})
	for _, line := range strings.Split(raw, "\n") {
		domain := normalizeEmailDomain(line)
		if domain == "" || strings.HasPrefix(domain, "#") {
			continue
		}
		domains[domain] = struct{}{}
	}
	return domains
}

func isDisposableEmail(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false
	}
	return isDisposableEmailDomain(email[at+1:])
}

func isDisposableEmailDomain(domain string) bool {
	domain = normalizeEmailDomain(domain)
	for domain != "" {
		if _, ok := disposableEmailDomains[domain]; ok {
			return true
		}
		dot := strings.IndexByte(domain, '.')
		if dot < 0 {
			return false
		}
		domain = domain[dot+1:]
	}
	return false
}

func normalizeEmailDomain(value string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".")
}
