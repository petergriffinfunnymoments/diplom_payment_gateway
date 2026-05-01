package simple

import (
	"context"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type simpleCallbackHandler struct{}

func newSimpleCallbackHandler() contracts.CallbackHandler {
	return &simpleCallbackHandler{}
}

func (c *simpleCallbackHandler) HandleCallback(
	ctx context.Context,
	adapterResult contracts.AdapterResult,
	req dto.CreatePaymentRequest,
	token string,
) (dto.PaymentResponse, error) {
	_ = ctx

	status := statusCaptured
	fraudCheck := "PASSED"
	var errObj *dto.GatewayError

	if adapterResult.Status != statusCaptured {
		status = statusFailed
		fraudCheck = "FAILED"
		msg := adapterResult.ErrorMessage
		if msg == "" {
			msg = "payment adapter returned failed status"
		}
		errObj = &dto.GatewayError{Code: "ADAPTER_FAILED", Message: msg}
	}

	return dto.PaymentResponse{
		ID:             req.PaymentID,
		MerchantID:     req.MerchantID,
		IdempotencyKey: req.IdempotencyKey,
		CurrentStatus:  status,
		PaymentInfo: dto.PaymentInfoResponse{
			Amount:            req.PaymentInfo.Amount,
			PaymentMethodData: req.PaymentInfo.PaymentMethodData,
			CustomerData:      req.PaymentInfo.CustomerData,
			Description:       req.PaymentInfo.Description,
			CreatedAt:         req.PaymentInfo.CreatedAt,
			UpdatedAt:         nowUTC(),
		},
		TransactionDetails: dto.TransactionDetails{
			ExternalTransactionID: adapterResult.ExternalTransactionID,
			PaymentSystem:         adapterResult.PaymentSystem,
			Token:                 token,
			FraudCheckResult:      fraudCheck,
			RetryCount:            0,
		},
		Error: errObj,
	}, nil
}
