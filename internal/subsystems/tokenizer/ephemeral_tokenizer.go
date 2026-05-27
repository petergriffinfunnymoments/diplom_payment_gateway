package tokenizer

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type EphemeralTokenizer struct{}

func NewEphemeralTokenizer() contracts.Tokenizer {
	return &EphemeralTokenizer{}
}

func (t *EphemeralTokenizer) Tokenize(ctx context.Context, req dto.CreatePaymentRequest) (string, error) {
	_ = ctx
	_ = req

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "tok_" + hex.EncodeToString(b), nil
}
