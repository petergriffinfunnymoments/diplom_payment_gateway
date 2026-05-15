package security

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/subsystems/merchantauth"
)

type NetworkConfig struct {
	TrustedProxyCIDRs    []*net.IPNet
	MerchantAllowedCIDRs []*net.IPNet
	AdminAllowedCIDRs    []*net.IPNet
	WebhookAllowedCIDRs  []*net.IPNet
}

func NetworkConfigFromEnv() (NetworkConfig, error) {
	cfg := NetworkConfig{}
	var err error

	if cfg.TrustedProxyCIDRs, err = ParseCIDRList(os.Getenv("TRUSTED_PROXY_CIDRS")); err != nil {
		return NetworkConfig{}, fmt.Errorf("TRUSTED_PROXY_CIDRS: %w", err)
	}
	if cfg.MerchantAllowedCIDRs, err = ParseCIDRList(os.Getenv("MERCHANT_ALLOWED_CIDRS")); err != nil {
		return NetworkConfig{}, fmt.Errorf("MERCHANT_ALLOWED_CIDRS: %w", err)
	}
	if cfg.AdminAllowedCIDRs, err = ParseCIDRList(os.Getenv("ADMIN_ALLOWED_CIDRS")); err != nil {
		return NetworkConfig{}, fmt.Errorf("ADMIN_ALLOWED_CIDRS: %w", err)
	}
	if cfg.WebhookAllowedCIDRs, err = ParseCIDRList(os.Getenv("WEBHOOK_ALLOWED_CIDRS")); err != nil {
		return NetworkConfig{}, fmt.Errorf("WEBHOOK_ALLOWED_CIDRS: %w", err)
	}

	return cfg, nil
}

func AuthenticatedNetworkMiddleware(cfg NetworkConfig, logger contracts.EventLogger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		merchant, ok := merchantauth.MerchantFromContext(r.Context())
		if !ok {
			logNetworkAccessDenied(r.Context(), logger, r, "", "", "AUTHENTICATED_ENDPOINT", "merchant context is missing", cfg)
			writeNetworkAccessDenied(w, "merchant context is missing")
			return
		}

		role := merchantauth.NormalizeRole(merchant.Role)
		allowedCIDRs := cfg.MerchantAllowedCIDRs
		policyName := "MERCHANT_ALLOWED_CIDRS"
		if role == merchantauth.RoleAdmin || role == merchantauth.RoleAuditor {
			allowedCIDRs = cfg.AdminAllowedCIDRs
			policyName = "ADMIN_ALLOWED_CIDRS"
		}

		clientIP, _ := cfg.ClientIP(r)
		if !IPAllowed(clientIP, allowedCIDRs) {
			logNetworkAccessDenied(r.Context(), logger, r, merchant.MerchantID, string(role), policyName, "request source IP is not allowed", cfg)
			writeNetworkAccessDenied(w, "request source IP is not allowed")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func WebhookNetworkMiddleware(cfg NetworkConfig, logger contracts.EventLogger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP, _ := cfg.ClientIP(r)
		if !IPAllowed(clientIP, cfg.WebhookAllowedCIDRs) {
			logNetworkAccessDenied(r.Context(), logger, r, "", "", "WEBHOOK_ALLOWED_CIDRS", "webhook source IP is not allowed", cfg)
			writeNetworkAccessDenied(w, "webhook source IP is not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func ParseCIDRList(raw string) ([]*net.IPNet, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})

	cidrs := make([]*net.IPNet, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		ipNet, err := parseCIDROrIP(part)
		if err != nil {
			return nil, err
		}
		cidrs = append(cidrs, ipNet)
	}
	return cidrs, nil
}

func IPAllowed(ip net.IP, allowed []*net.IPNet) bool {
	if len(allowed) == 0 {
		return true
	}
	if ip == nil {
		return false
	}
	for _, ipNet := range allowed {
		if ipNet != nil && ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

func (cfg NetworkConfig) ClientIP(r *http.Request) (net.IP, string) {
	if r == nil {
		return nil, "unknown"
	}

	remoteIP := RemoteIP(r.RemoteAddr)
	if IPAllowed(remoteIP, cfg.TrustedProxyCIDRs) && len(cfg.TrustedProxyCIDRs) > 0 {
		if ip := firstForwardedFor(r.Header.Get("X-Forwarded-For")); ip != nil {
			return ip, "x_forwarded_for"
		}
		if ip := parseIPLiteral(firstHeaderValue(r.Header.Get("X-Real-IP"))); ip != nil {
			return ip, "x_real_ip"
		}
		if ip := forwardedHeaderFor(r.Header.Get("Forwarded")); ip != nil {
			return ip, "forwarded"
		}
	}

	return remoteIP, "remote_addr"
}

func RemoteIP(remoteAddr string) net.IP {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return parseIPLiteral(host)
	}
	return parseIPLiteral(remoteAddr)
}

func firstForwardedFor(value string) net.IP {
	return parseIPLiteral(firstHeaderValue(value))
}

func forwardedHeaderFor(value string) net.IP {
	for _, entry := range strings.Split(value, ",") {
		for _, part := range strings.Split(entry, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(strings.ToLower(part), "for=") {
				raw := strings.TrimSpace(part[len("for="):])
				return parseIPLiteral(raw)
			}
		}
	}
	return nil
}

func parseCIDROrIP(value string) (*net.IPNet, error) {
	if strings.Contains(value, "/") {
		ip, ipNet, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q", value)
		}
		ipNet.IP = ip
		return ipNet, nil
	}

	ip := parseIPLiteral(value)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address %q", value)
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return &net.IPNet{IP: ipv4, Mask: net.CIDRMask(32, 32)}, nil
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, nil
}

func parseIPLiteral(value string) net.IP {
	value = strings.TrimSpace(strings.Trim(value, `"`))
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	return net.ParseIP(value)
}

func writeNetworkAccessDenied(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Code:    "NETWORK_ACCESS_DENIED",
		Message: message,
	})
}

func logNetworkAccessDenied(
	ctx context.Context,
	logger contracts.EventLogger,
	r *http.Request,
	merchantID string,
	role string,
	policyName string,
	reason string,
	cfg NetworkConfig,
) {
	if logger == nil {
		return
	}
	clientIP, source := cfg.ClientIP(r)
	clientIPValue := ""
	if clientIP != nil {
		clientIPValue = clientIP.String()
	}
	endpoint := ""
	remoteAddr := ""
	forwardedFor := ""
	realIP := ""
	if r != nil {
		endpoint = r.Method + " " + r.URL.Path
		remoteAddr = r.RemoteAddr
		forwardedFor = r.Header.Get("X-Forwarded-For")
		realIP = r.Header.Get("X-Real-IP")
	}

	correlationID := merchantID
	if correlationID == "" {
		correlationID = clientIPValue
	}

	_ = logger.Log(ctx, contracts.PaymentEvent{
		Type:          contracts.EventNetworkAccessDenied,
		Level:         contracts.LogLevelWarn,
		Service:       "api_gateway",
		MerchantID:    merchantID,
		CorrelationID: correlationID,
		Timestamp:     time.Now().UTC(),
		Message:       "Network access denied",
		Details:       reason,
		Context: map[string]string{
			"role":             role,
			"endpoint":         endpoint,
			"policy":           policyName,
			"client_ip":        clientIPValue,
			"client_ip_source": source,
			"remote_addr":      remoteAddr,
			"x_forwarded_for":  forwardedFor,
			"x_real_ip":        realIP,
			"reason":           reason,
		},
	})
}
