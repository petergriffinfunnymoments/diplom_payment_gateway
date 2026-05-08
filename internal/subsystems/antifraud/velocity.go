package antifraud

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"payment-gateway/internal/dto"
)

const (
	velocityWindow5m  = 5 * time.Minute
	velocityWindow30m = 30 * time.Minute
	velocityWindow1h  = time.Hour
	velocityWindow24h = 24 * time.Hour
)

type velocityStore struct {
	mu     sync.Mutex
	events []velocityEvent
	seen   map[string]struct{}
}

type velocityEvent struct {
	MerchantID string
	PaymentID  string
	SeenKey    string
	At         time.Time
	CardHash   string
	EmailHash  string
	PhoneHash  string
	WalletHash string
}

type velocityCounts struct {
	CardAttempts1h           int
	EmailAttempts30m         int
	PhoneAttempts1h          int
	WalletAttempts1h         int
	MerchantAttempts5m       int
	DistinctCardsPerEmail24h int
	DistinctCardsPerPhone24h int
}

func newVelocityStore() *velocityStore {
	return &velocityStore{
		events: make([]velocityEvent, 0),
		seen:   make(map[string]struct{}),
	}
}

func (s *velocityStore) recordAndCount(req dto.CreatePaymentRequest, now time.Time) velocityCounts {
	if s == nil {
		return velocityCounts{}
	}

	event := velocityEvent{
		MerchantID: strings.TrimSpace(req.MerchantID),
		PaymentID:  strings.TrimSpace(req.PaymentID),
		SeenKey:    seenKey(req),
		At:         now,
		CardHash:   hashSensitive(onlyDigits(req.PaymentInfo.CustomerData.CardNumber)),
		EmailHash:  hashSensitive(normalizeEmail(req.PaymentInfo.CustomerData.Email)),
		PhoneHash:  hashSensitive(onlyDigits(req.PaymentInfo.CustomerData.Phone)),
		WalletHash: hashSensitive(strings.ToLower(strings.TrimSpace(req.PaymentInfo.CustomerData.DigitalWalletID))),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked(now.Add(-velocityWindow24h))
	if event.SeenKey != "" {
		if _, exists := s.seen[event.SeenKey]; !exists {
			s.events = append(s.events, event)
			s.seen[event.SeenKey] = struct{}{}
		}
	} else {
		s.events = append(s.events, event)
	}

	return s.countLocked(event, now)
}

func (s *velocityStore) pruneLocked(cutoff time.Time) {
	kept := s.events[:0]
	newSeen := make(map[string]struct{}, len(s.seen))
	for _, event := range s.events {
		if event.At.Before(cutoff) {
			continue
		}
		kept = append(kept, event)
		if event.SeenKey != "" {
			newSeen[event.SeenKey] = struct{}{}
		}
	}
	s.events = kept
	s.seen = newSeen
}

func (s *velocityStore) countLocked(current velocityEvent, now time.Time) velocityCounts {
	var counts velocityCounts
	distinctCardsByEmail := map[string]struct{}{}
	distinctCardsByPhone := map[string]struct{}{}

	for _, event := range s.events {
		if event.MerchantID != current.MerchantID {
			continue
		}

		age := now.Sub(event.At)
		if age <= velocityWindow5m {
			counts.MerchantAttempts5m++
		}

		if current.CardHash != "" && event.CardHash == current.CardHash && age <= velocityWindow1h {
			counts.CardAttempts1h++
		}
		if current.EmailHash != "" && event.EmailHash == current.EmailHash && age <= velocityWindow30m {
			counts.EmailAttempts30m++
		}
		if current.PhoneHash != "" && event.PhoneHash == current.PhoneHash && age <= velocityWindow1h {
			counts.PhoneAttempts1h++
		}
		if current.WalletHash != "" && event.WalletHash == current.WalletHash && age <= velocityWindow1h {
			counts.WalletAttempts1h++
		}
		if current.EmailHash != "" && event.EmailHash == current.EmailHash && event.CardHash != "" && age <= velocityWindow24h {
			distinctCardsByEmail[event.CardHash] = struct{}{}
		}
		if current.PhoneHash != "" && event.PhoneHash == current.PhoneHash && event.CardHash != "" && age <= velocityWindow24h {
			distinctCardsByPhone[event.CardHash] = struct{}{}
		}
	}

	counts.DistinctCardsPerEmail24h = len(distinctCardsByEmail)
	counts.DistinctCardsPerPhone24h = len(distinctCardsByPhone)
	return counts
}

func (c velocityCounts) empty() bool {
	return c.CardAttempts1h == 0 &&
		c.EmailAttempts30m == 0 &&
		c.PhoneAttempts1h == 0 &&
		c.WalletAttempts1h == 0 &&
		c.MerchantAttempts5m == 0 &&
		c.DistinctCardsPerEmail24h == 0 &&
		c.DistinctCardsPerPhone24h == 0
}

func seenKey(req dto.CreatePaymentRequest) string {
	parts := []string{
		strings.TrimSpace(req.MerchantID),
		strings.TrimSpace(req.PaymentID),
		strings.TrimSpace(req.IdempotencyKey),
	}
	return strings.Join(parts, ":")
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func hashSensitive(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
