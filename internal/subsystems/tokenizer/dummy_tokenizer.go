package tokenizer

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type DummyTokenizer struct{}

func NewDummyTokenizer() contracts.Tokenizer {
	return &DummyTokenizer{}
}

func (t *DummyTokenizer) Tokenize(ctx context.Context, req dto.CreatePaymentRequest) (string, error) {
	_ = ctx
	_ = req

	// Делаем “псевдо-токен” для каркаса.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "tok_" + hex.EncodeToString(b), nil
}
