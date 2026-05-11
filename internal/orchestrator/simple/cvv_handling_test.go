package simple

import (
	"context"
	"strings"
	"testing"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/adapter"
	"payment-gateway/internal/subsystems/storage"
)

func TestCreatePaymentDoesNotPassCVVBeyondValidation(t *testing.T) {
	store := storage.NewInMemoryTransactionStore()
	o := NewSimpleOrchestrator(store)

	workflow := &capturingWorkflow{}
	antiFraud := &capturingAntiFraud{}
	router := &capturingRouter{}
	tokenizer := &capturingTokenizer{}
	adapterSpy := &capturingPaymentAdapter{}
	callback := &capturingCallback{}

	factory := adapter.NewFactory()
	factory.Register("spy", adapterSpy)

	o.workflow = workflow
	o.antiFraud = antiFraud
	o.router = router
	o.tokenizer = tokenizer
	o.adapterFactory = factory
	o.callback = callback

	req := validOrchestratorCardRequest()
	resp, err := o.CreatePayment(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.PaymentInfo.CustomerData.CvvCode != "" {
		t.Fatalf("response contains CVV")
	}

	assertNoCVV(t, "workflow", workflow.req)
	assertNoCVV(t, "antifraud", antiFraud.req)
	assertNoCVV(t, "router", router.req)
	assertNoCVV(t, "tokenizer", tokenizer.req)
	assertNoCVV(t, "adapter", adapterSpy.req)
	assertNoCVV(t, "callback", callback.req)

	_, payloadJSON, found, err := store.GetByPaymentID(context.Background(), req.MerchantID, req.PaymentID)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected saved payment")
	}
	if containsCVV(payloadJSON) {
		t.Fatalf("saved payload contains CVV: %s", payloadJSON)
	}
}

func assertNoCVV(t *testing.T, name string, req dto.CreatePaymentRequest) {
	t.Helper()
	if req.PaymentInfo.CustomerData.CvvCode != "" {
		t.Fatalf("%s received CVV: %q", name, req.PaymentInfo.CustomerData.CvvCode)
	}
}

func containsCVV(value string) bool {
	for _, forbidden := range []string{"CVV_code", `"cvv"`, `"cvc"`, `"cid"`} {
		if strings.Contains(value, forbidden) {
			return true
		}
	}
	return false
}

type capturingWorkflow struct {
	req dto.CreatePaymentRequest
}

func (w *capturingWorkflow) StartSession(ctx context.Context, req dto.CreatePaymentRequest) (string, error) {
	_ = ctx
	w.req = req
	return "sess_test", nil
}

func (w *capturingWorkflow) CompleteSession(ctx context.Context, sessionID string, finalStatus string) error {
	_ = ctx
	_ = sessionID
	_ = finalStatus
	return nil
}

type capturingAntiFraud struct {
	req dto.CreatePaymentRequest
}

func (a *capturingAntiFraud) Check(ctx context.Context, req dto.CreatePaymentRequest) (contracts.AntiFraudResult, error) {
	_ = ctx
	a.req = req
	return contracts.AntiFraudResult{Result: "PASSED", Reason: "test"}, nil
}

type capturingRouter struct {
	req dto.CreatePaymentRequest
}

func (r *capturingRouter) Route(ctx context.Context, req dto.CreatePaymentRequest, fraud contracts.AntiFraudResult) (string, string, error) {
	_ = ctx
	_ = fraud
	r.req = req
	return "TEST", "spy", nil
}

type capturingTokenizer struct {
	req dto.CreatePaymentRequest
}

func (t *capturingTokenizer) Tokenize(ctx context.Context, req dto.CreatePaymentRequest) (string, error) {
	_ = ctx
	t.req = req
	return "tok_test_1234567890", nil
}

type capturingPaymentAdapter struct {
	req dto.CreatePaymentRequest
}

func (a *capturingPaymentAdapter) Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (contracts.AdapterResult, error) {
	_ = ctx
	_ = token
	a.req = req
	return contracts.AdapterResult{
		ExternalTransactionID: "ext_test",
		PaymentSystem:         "TEST",
		Status:                statusCaptured,
		ProviderStatus:        "succeeded",
	}, nil
}

type capturingCallback struct {
	req dto.CreatePaymentRequest
}

func (c *capturingCallback) HandleCallback(ctx context.Context, adapterResult contracts.AdapterResult, req dto.CreatePaymentRequest, token string) (dto.PaymentResponse, error) {
	_ = ctx
	c.req = req
	return dto.PaymentResponse{
		ID:             req.PaymentID,
		MerchantID:     req.MerchantID,
		IdempotencyKey: req.IdempotencyKey,
		CurrentStatus:  adapterResult.Status,
		PaymentInfo: dto.PaymentInfoResponse{
			Amount:            req.PaymentInfo.Amount,
			PaymentMethodData: req.PaymentInfo.PaymentMethodData,
			CustomerData:      req.PaymentInfo.CustomerData.Sanitized(),
			Description:       req.PaymentInfo.Description,
			CreatedAt:         req.PaymentInfo.CreatedAt,
			UpdatedAt:         time.Now().UTC(),
		},
		TransactionDetails: dto.TransactionDetails{
			ExternalTransactionID: adapterResult.ExternalTransactionID,
			PaymentSystem:         adapterResult.PaymentSystem,
			ProviderStatus:        adapterResult.ProviderStatus,
			TokenPreview:          dto.TokenPreview(token),
			FraudCheckResult:      "PASSED",
		},
	}, nil
}

func validOrchestratorCardRequest() dto.CreatePaymentRequest {
	return dto.CreatePaymentRequest{
		MerchantID:     "merchant_12345",
		IdempotencyKey: "idem_12345678",
		PaymentID:      "pay_12345",
		CurrentStatus:  string(dto.StatusCreated),
		PaymentInfo: dto.PaymentInfo{
			Amount: dto.AmountMoney{
				Value:    1500,
				Currency: "RUB",
			},
			PaymentMethodData: dto.PaymentMethodData{
				Type: dto.PaymentMethodCard,
			},
			CustomerData: dto.CustomerData{
				Email:      "customer@example.com",
				Phone:      "+79991234567",
				CardNumber: "4111111111111111",
				CardDate:   time.Now().UTC().AddDate(2, 0, 0).Format("01/06"),
				CvvCode:    "123",
			},
			CreatedAt:   time.Now().UTC(),
			Description: "Test card payment",
		},
	}
}
