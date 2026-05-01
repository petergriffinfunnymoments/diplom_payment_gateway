package dto

// PaymentStatus описывает бизнес-статус платежа, который видит интернет-магазин.
// Внутренние этапы вроде validation/tokenization/fraud_check лучше хранить как события логирования,
// а не как current_status платежа.
type PaymentStatus string

const (
	StatusCreated          PaymentStatus = "CREATED"           // платеж создан в шлюзе
	StatusPending          PaymentStatus = "PENDING"           // ожидается подтверждение/действие пользователя или внешней ЭПС
	StatusAuthorized       PaymentStatus = "AUTHORIZED"        // платеж подтвержден, средства могут быть зарезервированы
	StatusCaptureRequested PaymentStatus = "CAPTURE_REQUESTED" // шлюз запросил списание средств
	StatusCaptured         PaymentStatus = "CAPTURED"          // средства успешно списаны
	StatusDeclined         PaymentStatus = "DECLINED"          // платеж отклонен пользователем, ЭПС, банком или антифродом
	StatusCancelled        PaymentStatus = "CANCELLED"         // платеж отменен пользователем/магазином до списания
	StatusFailed           PaymentStatus = "FAILED"            // техническая ошибка шлюза или внутреннего модуля

	// Статусы для будущего сценария аннулирования авторизованного платежа.
	StatusVoidRequested PaymentStatus = "VOID_REQUESTED"
	StatusVoidFailed    PaymentStatus = "VOID_FAILED"
	StatusVoided        PaymentStatus = "VOIDED"
)

func IsValidPaymentStatus(status string) bool {
	switch PaymentStatus(status) {
	case StatusCreated,
		StatusPending,
		StatusAuthorized,
		StatusCaptureRequested,
		StatusCaptured,
		StatusDeclined,
		StatusCancelled,
		StatusFailed,
		StatusVoidRequested,
		StatusVoidFailed,
		StatusVoided:
		return true
	default:
		return false
	}
}
