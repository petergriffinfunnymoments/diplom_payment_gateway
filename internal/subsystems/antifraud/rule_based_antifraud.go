package antifraud

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

const (
	ResultPassed  = "PASSED"
	ResultReview  = "REVIEW"
	ResultBlocked = "BLOCKED"
)

const (
	defaultReviewAmountLimit = 100_000.00
	defaultBlockAmountLimit  = 500_000.00
	maxRiskScoreBeforeReview = 30
	maxRiskScoreBeforeBlock  = 80
)

type RuleBasedAntiFraud struct {
	reviewAmountLimit float64
	blockAmountLimit  float64
	velocity          *velocityStore
	opaPolicy         *opaAntiFraudPolicy
}

type fraudRuleResult struct {
	Name   string
	Score  int
	Block  bool
	Reason string
}

func NewRuleBasedAntiFraud() contracts.AntiFraud {
	opaPolicy, _ := newOPAAntiFraudPolicy(context.Background())
	return &RuleBasedAntiFraud{
		reviewAmountLimit: defaultReviewAmountLimit,
		blockAmountLimit:  defaultBlockAmountLimit,
		velocity:          newVelocityStore(),
		opaPolicy:         opaPolicy,
	}
}

func (a *RuleBasedAntiFraud) Check(ctx context.Context, req dto.CreatePaymentRequest) (contracts.AntiFraudResult, error) {
	if err := ctx.Err(); err != nil {
		return contracts.AntiFraudResult{}, err
	}

	velocity := a.velocity.recordAndCount(req, time.Now().UTC())
	opaResult, err := a.checkOPA(ctx, req)
	if err != nil {
		return contracts.AntiFraudResult{}, err
	}

	ruleResults := []fraudRuleResult{
		opaResult,
		a.checkAmount(req),
		a.checkCard(req),
		a.checkDigitalWallet(req),
		a.checkPhone(req),
		a.checkEmail(req),
		a.checkDescription(req),
		a.checkVelocity(velocity),
	}

	totalScore := 0
	reasons := make([]string, 0)

	for _, result := range ruleResults {
		if result.Name == "" {
			continue
		}

		if result.Score > 0 {
			totalScore += result.Score
		}

		if result.Reason != "" {
			reasons = append(reasons, result.Reason)
		}

		if result.Block {
			return contracts.AntiFraudResult{
				Result: ResultBlocked,
				Reason: fmt.Sprintf("risk score %d blocked: %s", totalScore, joinReasons(reasons)),
			}, nil
		}
	}

	if totalScore >= maxRiskScoreBeforeBlock {
		return contracts.AntiFraudResult{
			Result: ResultBlocked,
			Reason: fmt.Sprintf("risk score %d exceeds block threshold %d: %s", totalScore, maxRiskScoreBeforeBlock, joinReasons(reasons)),
		}, nil
	}

	if totalScore >= maxRiskScoreBeforeReview {
		return contracts.AntiFraudResult{
			Result: ResultReview,
			Reason: fmt.Sprintf("risk score %d requires manual review: %s", totalScore, joinReasons(reasons)),
		}, nil
	}

	if totalScore > 0 {
		return contracts.AntiFraudResult{
			Result: ResultReview,
			Reason: joinReasons(reasons),
		}, nil
	}

	return contracts.AntiFraudResult{
		Result: ResultPassed,
		Reason: "no suspicious antifraud rules triggered",
	}, nil
}

func (a *RuleBasedAntiFraud) checkVelocity(v velocityCounts) fraudRuleResult {
	if v.empty() {
		return fraudRuleResult{}
	}

	reasons := make([]string, 0)
	score := 0
	block := false

	if v.CardAttempts1h >= 5 {
		score += 70
		block = true
		reasons = append(reasons, fmt.Sprintf("same card fingerprint has %d attempts in 1h", v.CardAttempts1h))
	} else if v.CardAttempts1h >= 4 {
		score += 40
		reasons = append(reasons, fmt.Sprintf("same card fingerprint has %d attempts in 1h", v.CardAttempts1h))
	}

	if v.EmailAttempts30m >= 5 {
		score += 35
		reasons = append(reasons, fmt.Sprintf("same email has %d attempts in 30m", v.EmailAttempts30m))
	}

	if v.PhoneAttempts1h >= 6 {
		score += 30
		reasons = append(reasons, fmt.Sprintf("same phone has %d attempts in 1h", v.PhoneAttempts1h))
	}

	if v.WalletAttempts1h >= 6 {
		score += 55
		block = true
		reasons = append(reasons, fmt.Sprintf("same wallet has %d attempts in 1h", v.WalletAttempts1h))
	} else if v.WalletAttempts1h >= 4 {
		score += 35
		reasons = append(reasons, fmt.Sprintf("same wallet has %d attempts in 1h", v.WalletAttempts1h))
	}

	if v.DistinctCardsPerEmail24h >= 5 {
		score += 60
		reasons = append(reasons, fmt.Sprintf("email used %d distinct cards in 24h", v.DistinctCardsPerEmail24h))
	} else if v.DistinctCardsPerEmail24h >= 3 {
		score += 30
		reasons = append(reasons, fmt.Sprintf("email used %d distinct cards in 24h", v.DistinctCardsPerEmail24h))
	}

	if v.DistinctCardsPerPhone24h >= 5 {
		score += 60
		reasons = append(reasons, fmt.Sprintf("phone used %d distinct cards in 24h", v.DistinctCardsPerPhone24h))
	} else if v.DistinctCardsPerPhone24h >= 3 {
		score += 30
		reasons = append(reasons, fmt.Sprintf("phone used %d distinct cards in 24h", v.DistinctCardsPerPhone24h))
	}

	if v.MerchantAttempts5m >= 30 {
		score += 20
		reasons = append(reasons, fmt.Sprintf("merchant has %d attempts in 5m", v.MerchantAttempts5m))
	}

	if score == 0 {
		return fraudRuleResult{}
	}

	return fraudRuleResult{
		Name:   "velocity_scorecard",
		Score:  score,
		Block:  block,
		Reason: strings.Join(reasons, "; "),
	}
}

func (a *RuleBasedAntiFraud) checkAmount(req dto.CreatePaymentRequest) fraudRuleResult {
	amount := req.PaymentInfo.Amount.Value

	if amount >= a.blockAmountLimit {
		return fraudRuleResult{
			Name:   "amount_block_limit",
			Score:  100,
			Block:  true,
			Reason: fmt.Sprintf("amount %.2f exceeds block limit %.2f", amount, a.blockAmountLimit),
		}
	}

	if amount >= a.reviewAmountLimit {
		return fraudRuleResult{
			Name:   "amount_review_limit",
			Score:  35,
			Reason: fmt.Sprintf("amount %.2f exceeds review limit %.2f", amount, a.reviewAmountLimit),
		}
	}

	return fraudRuleResult{}
}

func (a *RuleBasedAntiFraud) checkCard(req dto.CreatePaymentRequest) fraudRuleResult {
	if req.PaymentInfo.PaymentMethodData.Type != dto.PaymentMethodCard {
		return fraudRuleResult{}
	}

	card := onlyDigits(req.PaymentInfo.CustomerData.CardNumber)
	if card == "" {
		return fraudRuleResult{}
	}

	blockedCards := map[string]string{
		"4000000000000002": "blocked test card number",
		"4111111111110002": "blocked card number",
	}
	if reason, ok := blockedCards[card]; ok {
		return fraudRuleResult{Name: "blocked_card", Score: 100, Block: true, Reason: reason}
	}

	blockedLast4 := map[string]string{
		"0002": "card is in blocked last4 test list",
		"9995": "card is in suspicious last4 test list",
	}
	if len(card) >= 4 {
		last4 := card[len(card)-4:]
		if reason, ok := blockedLast4[last4]; ok {
			return fraudRuleResult{Name: "suspicious_card_last4", Score: 60, Block: last4 == "0002", Reason: reason}
		}
	}

	return fraudRuleResult{}
}

func (a *RuleBasedAntiFraud) checkDigitalWallet(req dto.CreatePaymentRequest) fraudRuleResult {
	if req.PaymentInfo.PaymentMethodData.Type != dto.PaymentMethodDigitalWallet &&
		req.PaymentInfo.PaymentMethodData.Type != dto.PaymentMethodDigitalRuble {
		return fraudRuleResult{}
	}

	walletID := strings.ToLower(strings.TrimSpace(digitalWalletRiskID(req)))
	blockedWallets := map[string]string{
		"blocked_wallet":    "digital wallet is blocked",
		"fraud_wallet":      "digital wallet is marked as fraudulent",
		"dr_wallet_blocked": "digital ruble wallet is blocked",
		"dr_wallet_fraud":   "digital ruble wallet is marked as fraudulent",
	}

	if reason, ok := blockedWallets[walletID]; ok {
		return fraudRuleResult{Name: "blocked_wallet", Score: 100, Block: true, Reason: reason}
	}

	return fraudRuleResult{}
}

func digitalWalletRiskID(req dto.CreatePaymentRequest) string {
	customer := req.PaymentInfo.CustomerData
	switch {
	case strings.TrimSpace(customer.DigitalRubleWalletID) != "":
		return customer.DigitalRubleWalletID
	case strings.TrimSpace(customer.DigitalRubleIdentifier) != "":
		return customer.DigitalRubleIdentifier
	default:
		return customer.DigitalWalletID
	}
}

func (a *RuleBasedAntiFraud) checkPhone(req dto.CreatePaymentRequest) fraudRuleResult {
	phone := strings.TrimSpace(req.PaymentInfo.CustomerData.Phone)
	if phone == "" {
		return fraudRuleResult{}
	}

	blockedPhones := map[string]string{
		"+79990000000": "phone is in blocked list",
		"+70000000000": "phone is in blocked list",
	}
	if reason, ok := blockedPhones[phone]; ok {
		return fraudRuleResult{Name: "blocked_phone", Score: 100, Block: true, Reason: reason}
	}

	if hasTooManyRepeatedDigits(phone) {
		return fraudRuleResult{Name: "suspicious_phone", Score: 20, Reason: "phone contains too many repeated digits"}
	}

	return fraudRuleResult{}
}

func (a *RuleBasedAntiFraud) checkEmail(req dto.CreatePaymentRequest) fraudRuleResult {
	email := strings.ToLower(strings.TrimSpace(req.PaymentInfo.CustomerData.Email))
	if email == "" {
		return fraudRuleResult{}
	}

	if strings.Contains(email, "fraud") || strings.Contains(email, "scam") || strings.Contains(email, "blocked") {
		return fraudRuleResult{Name: "suspicious_email_keyword", Score: 40, Reason: "email contains suspicious keyword"}
	}

	if isDisposableEmail(email) {
		return fraudRuleResult{Name: "temporary_email_domain", Score: 25, Reason: "email uses disposable domain from Castle list"}
	}

	return fraudRuleResult{}
}

func (a *RuleBasedAntiFraud) checkDescription(req dto.CreatePaymentRequest) fraudRuleResult {
	description := strings.ToLower(strings.TrimSpace(req.PaymentInfo.Description))
	if description == "" {
		return fraudRuleResult{}
	}

	suspiciousWords := []string{"fraud", "scam", "обнал", "мошен", "blocked"}
	for _, word := range suspiciousWords {
		if strings.Contains(description, word) {
			return fraudRuleResult{Name: "suspicious_description", Score: 30, Reason: "payment description contains suspicious keyword"}
		}
	}

	return fraudRuleResult{}
}

func joinReasons(reasons []string) string {
	filtered := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		if strings.TrimSpace(reason) != "" {
			filtered = append(filtered, reason)
		}
	}
	if len(filtered) == 0 {
		return "antifraud rule triggered"
	}
	return strings.Join(filtered, "; ")
}

func onlyDigits(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasTooManyRepeatedDigits(value string) bool {
	digits := onlyDigits(value)
	if len(digits) < 7 {
		return false
	}

	counts := make(map[rune]int)
	for _, r := range digits {
		counts[r]++
		if counts[r] >= 7 {
			return true
		}
	}
	return false
}
