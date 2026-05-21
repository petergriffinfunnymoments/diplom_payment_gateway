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

	status := adapterResult.Status
	if status == "" {
		status = statusFailed
	}

	fraudCheck := "PASSED"
	var errObj *dto.GatewayError

	switch status {
	case statusCaptured, statusPending, statusCaptureRequested:
		// Не считаем PENDING ошибкой: внешний провайдер может вернуть ссылку на оплату.
	case statusDeclined, statusCancelled:
		fraudCheck = "PASSED"
		msg := adapterResult.ErrorMessage
		if msg == "" {
			msg = "payment was declined by external payment provider"
		}
		code := adapterResult.ErrorCode
		if code == "" {
			code = dto.ErrorPaymentDeclined
		}
		errObj = dto.NewGatewayError(code, msg)
	default:
		status = statusFailed
		fraudCheck = "FAILED"
		msg := adapterResult.ErrorMessage
		if msg == "" {
			msg = "payment adapter returned failed status"
		}
		code := adapterResult.ErrorCode
		if code == "" {
			code = dto.ErrorAdapterFailed
		}
		errObj = dto.NewGatewayError(code, msg)
	}

	return dto.PaymentResponse{
		ID:             req.PaymentID,
		MerchantID:     req.MerchantID,
		IdempotencyKey: req.IdempotencyKey,
		CurrentStatus:  status,
		PaymentInfo: dto.PaymentInfoResponse{
			Amount:            req.PaymentInfo.Amount,
			PaymentMethodData: req.PaymentInfo.PaymentMethodData,
			CustomerData:      req.PaymentInfo.CustomerData.Sanitized(),
			Description:       req.PaymentInfo.Description,
			CreatedAt:         req.PaymentInfo.CreatedAt,
			UpdatedAt:         nowUTC(),
		},
		TransactionDetails: dto.TransactionDetails{
			ExternalTransactionID: adapterResult.ExternalTransactionID,
			PaymentSystem:         adapterResult.PaymentSystem,
			ProviderStatus:        adapterResult.ProviderStatus,
			PaymentURL:            adapterResult.PaymentURL,
			QRID:                  adapterResult.QRID,
			QRPayload:             adapterResult.QRPayload,
			QRImageDataURI:        adapterResult.QRImageDataURI,
			QRExpiresAt:           formatOptionalTime(adapterResult.QRExpiresAt),
			ParticipantBank:       adapterResult.ParticipantBank,
			SchemaVersion:         adapterResult.SchemaVersion,
			SettlementHint:        adapterResult.SettlementHint,
			TokenPreview:          dto.TokenPreview(token),
			FraudCheckResult:      fraudCheck,
			RetryCount:            0,
		},
		Error: errObj,
	}, nil
}
