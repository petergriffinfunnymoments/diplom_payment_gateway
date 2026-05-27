package dto

type PaymentStatus string

const (
	StatusCreated          PaymentStatus = "CREATED"
	StatusPending          PaymentStatus = "PENDING"
	StatusCaptureRequested PaymentStatus = "CAPTURE_REQUESTED"
	StatusCaptured         PaymentStatus = "CAPTURED"
	StatusDeclined         PaymentStatus = "DECLINED"
	StatusCancelled        PaymentStatus = "CANCELLED"
	StatusFailed           PaymentStatus = "FAILED"
)

func IsValidPaymentStatus(status string) bool {
	switch PaymentStatus(status) {
	case StatusCreated,
		StatusPending,
		StatusCaptureRequested,
		StatusCaptured,
		StatusDeclined,
		StatusCancelled,
		StatusFailed:
		return true
	default:
		return false
	}
}
