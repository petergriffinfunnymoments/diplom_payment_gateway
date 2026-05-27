package antifraud

import (
	"context"
	"fmt"
	"strings"

	"github.com/open-policy-agent/opa/rego"

	"payment-gateway/internal/dto"
)

const opaAntiFraudModule = `
package payment_gateway.antifraud

findings[f] {
	input.amount >= 500000
	f := {
		"result": "BLOCKED",
		"score": 100,
		"reason": "OPA policy blocked payment: amount exceeds 500000",
	}
}

findings[f] {
	lower(input.email) == "opa.blocked@example.com"
	f := {
		"result": "BLOCKED",
		"score": 100,
		"reason": "OPA policy blocked payment: email is in deny list",
	}
}

findings[f] {
	lower(input.digital_wallet_id) == "opa_blocked_wallet"
	f := {
		"result": "BLOCKED",
		"score": 100,
		"reason": "OPA policy blocked payment: digital wallet is in deny list",
	}
}
`

type opaAntiFraudPolicy struct {
	query rego.PreparedEvalQuery
}

func newOPAAntiFraudPolicy(ctx context.Context) (*opaAntiFraudPolicy, error) {
	query, err := rego.New(
		rego.Query("data.payment_gateway.antifraud.findings"),
		rego.Module("antifraud.rego", opaAntiFraudModule),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("prepare opa antifraud policy: %w", err)
	}

	return &opaAntiFraudPolicy{query: query}, nil
}

func (a *RuleBasedAntiFraud) checkOPA(ctx context.Context, req dto.CreatePaymentRequest) (fraudRuleResult, error) {
	if a == nil || a.opaPolicy == nil {
		return fraudRuleResult{}, nil
	}

	results, err := a.opaPolicy.query.Eval(ctx, rego.EvalInput(opaInput(req)))
	if err != nil {
		return fraudRuleResult{}, fmt.Errorf("evaluate opa antifraud policy: %w", err)
	}
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return fraudRuleResult{}, nil
	}

	findings, ok := results[0].Expressions[0].Value.([]interface{})
	if !ok || len(findings) == 0 {
		return fraudRuleResult{}, nil
	}

	totalScore := 0
	block := false
	reasons := make([]string, 0, len(findings))
	for _, raw := range findings {
		finding, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		totalScore += intValue(finding["score"])
		if strings.EqualFold(stringValue(finding["result"]), ResultBlocked) {
			block = true
		}
		if reason := strings.TrimSpace(stringValue(finding["reason"])); reason != "" {
			reasons = append(reasons, reason)
		}
	}

	if totalScore == 0 && len(reasons) == 0 {
		return fraudRuleResult{}, nil
	}

	return fraudRuleResult{
		Name:   "opa_policy",
		Score:  totalScore,
		Block:  block,
		Reason: strings.Join(reasons, "; "),
	}, nil
}

func opaInput(req dto.CreatePaymentRequest) map[string]interface{} {
	customer := req.PaymentInfo.CustomerData
	return map[string]interface{}{
		"merchant_id":              strings.TrimSpace(req.MerchantID),
		"payment_id":               strings.TrimSpace(req.PaymentID),
		"idempotency_key":          strings.TrimSpace(req.IdempotencyKey),
		"amount":                   req.PaymentInfo.Amount.Value,
		"currency":                 string(req.PaymentInfo.Amount.Currency),
		"payment_method":           string(req.PaymentInfo.PaymentMethodData.Type),
		"email":                    strings.TrimSpace(customer.Email),
		"phone":                    strings.TrimSpace(customer.Phone),
		"card_last4":               cardLast4(customer.CardNumber),
		"digital_wallet_id":        strings.TrimSpace(customer.DigitalWalletID),
		"digital_ruble_wallet_id":  strings.TrimSpace(customer.DigitalRubleWalletID),
		"digital_ruble_identifier": strings.TrimSpace(customer.DigitalRubleIdentifier),
		"description":              strings.TrimSpace(req.PaymentInfo.Description),
	}
}

func cardLast4(card string) string {
	digits := onlyDigits(card)
	if len(digits) < 4 {
		return ""
	}
	return digits[len(digits)-4:]
}

func intValue(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case jsonNumber:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

type jsonNumber interface {
	Int64() (int64, error)
}
