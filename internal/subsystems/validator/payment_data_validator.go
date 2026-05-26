package validator

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	playground "github.com/go-playground/validator/v10"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

const (
	maxAmountValue       = 1_000_000.00
	maxDescriptionLength = 255
)

var (
	phoneRegexp        = regexp.MustCompile(`^\+7\d{10}$`)
	cardDateRegexp     = regexp.MustCompile(`^(0[1-9]|1[0-2])/\d{2}$`)
	cardDigitsRegexp   = regexp.MustCompile(`^\d{13,19}$`)
	cvvRegexp          = regexp.MustCompile(`^\d{3,4}$`)
	walletIDRegexp     = regexp.MustCompile(`^[A-Za-z0-9_-]{3,64}$`)
	drIdentifierRegexp = regexp.MustCompile(`^[A-Za-z0-9_.:-]{3,70}$`)
	drAccountRegexp    = regexp.MustCompile(`^[A-Za-z0-9]{1,34}$`)
)

// PaymentDataValidator — реальный модуль валидации платежных данных.
// Он проверяет общую структуру запроса и метод-специфичные поля:
// СБП, банковская карта, цифровой кошелёк.
type PaymentDataValidator struct {
	validate *playground.Validate
}

// NewPaymentDataValidator создаёт валидатор на базе github.com/go-playground/validator/v10.
func NewPaymentDataValidator() contracts.PaymentValidator {
	v := playground.New()

	_ = v.RegisterValidation("payment_method", validatePaymentMethod)
	_ = v.RegisterValidation("currency", validateCurrency)
	_ = v.RegisterValidation("phone_ru", validatePhoneRU)
	_ = v.RegisterValidation("card_number", validateCardNumber)
	_ = v.RegisterValidation("card_exp", validateCardExpiration)
	_ = v.RegisterValidation("cvv", validateCVV)
	_ = v.RegisterValidation("wallet_id", validateWalletID)
	_ = v.RegisterValidation("digital_ruble_identifier", validateDigitalRubleIdentifier)
	_ = v.RegisterValidation("digital_ruble_account", validateDigitalRubleAccount)
	_ = v.RegisterValidation("payment_status", validatePaymentStatus)

	return &PaymentDataValidator{validate: v}
}

func (v *PaymentDataValidator) Validate(ctx context.Context, req dto.CreatePaymentRequest) (dto.CreatePaymentRequest, error) {
	if err := ctx.Err(); err != nil {
		return req.WithoutSensitiveAuthenticationData(), err
	}

	normalized := normalizeRequest(req)
	if err := v.validate.Struct(toValidationModel(normalized)); err != nil {
		return normalized.WithoutSensitiveAuthenticationData(), formatValidationError(err)
	}

	if err := validatePaymentMethodSpecificFields(normalized); err != nil {
		return normalized.WithoutSensitiveAuthenticationData(), err
	}

	return normalized.WithoutSensitiveAuthenticationData(), nil
}

type createPaymentValidationModel struct {
	MerchantID     string                     `validate:"required,min=3,max=64"`
	IdempotencyKey string                     `validate:"required,min=8,max=128"`
	PaymentID      string                     `validate:"required,min=3,max=64"`
	CurrentStatus  string                     `validate:"required,payment_status"`
	PaymentInfo    paymentInfoValidationModel `validate:"required"`
}

type paymentInfoValidationModel struct {
	Amount            amountValidationModel            `validate:"required"`
	PaymentMethodData paymentMethodDataValidationModel `validate:"required"`
	CustomerData      customerDataValidationModel      `validate:"required"`
	Items             []paymentItemValidationModel     `validate:"omitempty,dive"`
	DigitalRubleData  digitalRubleDataValidationModel  `validate:"omitempty"`
	CreatedAt         time.Time                        `validate:"required"`
	Description       string                           `validate:"required,min=3,max=255"`
}

type amountValidationModel struct {
	Value    float64 `validate:"required,gt=0,lte=1000000"`
	Currency string  `validate:"required,currency"`
}

type paymentMethodDataValidationModel struct {
	Type string `validate:"required,payment_method"`
}

type customerDataValidationModel struct {
	Email                  string `validate:"omitempty,email,max=254"`
	Phone                  string `validate:"omitempty,phone_ru"`
	CardNumber             string `validate:"omitempty,card_number"`
	CardDate               string `validate:"omitempty,card_exp"`
	CvvCode                string `validate:"omitempty,cvv"`
	DigitalWalletID        string `validate:"omitempty,wallet_id"`
	DigitalRubleWalletID   string `validate:"omitempty,digital_ruble_identifier"`
	DigitalRubleAccount    string `validate:"omitempty,digital_ruble_account"`
	DigitalRubleIdentifier string `validate:"omitempty,digital_ruble_identifier"`
}

type paymentItemValidationModel struct {
	Name          string  `validate:"required,min=1,max=128"`
	Price         float64 `validate:"required,gt=0,lte=1000000"`
	Quantity      float64 `validate:"required,gt=0,lte=100000"`
	Category      string  `validate:"omitempty,max=64"`
	VATTag        string  `validate:"omitempty,max=32"`
	PaymentMethod string  `validate:"omitempty,max=32"`
	PaymentObject string  `validate:"omitempty,max=32"`
	IDInternal    string  `validate:"omitempty,max=64"`
}

type digitalRubleDataValidationModel struct {
	SmartContractID string `validate:"omitempty,max=64"`
}

func toValidationModel(req dto.CreatePaymentRequest) createPaymentValidationModel {
	return createPaymentValidationModel{
		MerchantID:     req.MerchantID,
		IdempotencyKey: req.IdempotencyKey,
		PaymentID:      req.PaymentID,
		CurrentStatus:  req.CurrentStatus,
		PaymentInfo: paymentInfoValidationModel{
			Amount: amountValidationModel{
				Value:    req.PaymentInfo.Amount.Value,
				Currency: string(req.PaymentInfo.Amount.Currency),
			},
			PaymentMethodData: paymentMethodDataValidationModel{
				Type: string(req.PaymentInfo.PaymentMethodData.Type),
			},
			CustomerData: customerDataValidationModel{
				Email:                  req.PaymentInfo.CustomerData.Email,
				Phone:                  req.PaymentInfo.CustomerData.Phone,
				CardNumber:             req.PaymentInfo.CustomerData.CardNumber,
				CardDate:               req.PaymentInfo.CustomerData.CardDate,
				CvvCode:                req.PaymentInfo.CustomerData.CvvCode,
				DigitalWalletID:        req.PaymentInfo.CustomerData.DigitalWalletID,
				DigitalRubleWalletID:   req.PaymentInfo.CustomerData.DigitalRubleWalletID,
				DigitalRubleAccount:    req.PaymentInfo.CustomerData.DigitalRubleAccount,
				DigitalRubleIdentifier: req.PaymentInfo.CustomerData.DigitalRubleIdentifier,
			},
			Items: toPaymentItemValidationModels(req.PaymentInfo.Items),
			DigitalRubleData: digitalRubleDataValidationModel{
				SmartContractID: req.PaymentInfo.DigitalRubleData.SmartContractID,
			},
			CreatedAt:   req.PaymentInfo.CreatedAt,
			Description: req.PaymentInfo.Description,
		},
	}
}

func toPaymentItemValidationModels(items []dto.PaymentItem) []paymentItemValidationModel {
	if len(items) == 0 {
		return nil
	}
	models := make([]paymentItemValidationModel, 0, len(items))
	for _, item := range items {
		models = append(models, paymentItemValidationModel{
			Name:          item.Name,
			Price:         item.Price,
			Quantity:      item.Quantity,
			Category:      item.Category,
			VATTag:        item.VATTag,
			PaymentMethod: item.PaymentMethod,
			PaymentObject: item.PaymentObject,
			IDInternal:    item.IDInternal,
		})
	}
	return models
}

func normalizeRequest(req dto.CreatePaymentRequest) dto.CreatePaymentRequest {
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.PaymentID = strings.TrimSpace(req.PaymentID)
	req.CurrentStatus = strings.TrimSpace(req.CurrentStatus)

	req.PaymentInfo.Amount.Currency = dto.PaymentCurrency(strings.ToUpper(strings.TrimSpace(string(req.PaymentInfo.Amount.Currency))))
	req.PaymentInfo.PaymentMethodData.Type = dto.PaymentMethodType(strings.TrimSpace(string(req.PaymentInfo.PaymentMethodData.Type)))
	req.PaymentInfo.Description = strings.TrimSpace(req.PaymentInfo.Description)

	customer := req.PaymentInfo.CustomerData
	customer.Email = strings.ToLower(strings.TrimSpace(customer.Email))
	customer.Phone = normalizePhone(customer.Phone)
	customer.CardNumber = onlyDigits(customer.CardNumber)
	customer.CardDate = strings.TrimSpace(customer.CardDate)
	customer.CvvCode = strings.TrimSpace(customer.CvvCode)
	customer.DigitalWalletID = strings.TrimSpace(customer.DigitalWalletID)
	customer.DigitalRubleWalletID = strings.TrimSpace(customer.DigitalRubleWalletID)
	customer.DigitalRubleAccount = strings.TrimSpace(customer.DigitalRubleAccount)
	customer.DigitalRubleIdentifier = strings.TrimSpace(customer.DigitalRubleIdentifier)
	req.PaymentInfo.CustomerData = customer
	for i := range req.PaymentInfo.Items {
		req.PaymentInfo.Items[i].Name = strings.TrimSpace(req.PaymentInfo.Items[i].Name)
		req.PaymentInfo.Items[i].Category = strings.ToLower(strings.TrimSpace(req.PaymentInfo.Items[i].Category))
		req.PaymentInfo.Items[i].VATTag = strings.TrimSpace(req.PaymentInfo.Items[i].VATTag)
		req.PaymentInfo.Items[i].PaymentMethod = strings.TrimSpace(req.PaymentInfo.Items[i].PaymentMethod)
		req.PaymentInfo.Items[i].PaymentObject = strings.TrimSpace(req.PaymentInfo.Items[i].PaymentObject)
		req.PaymentInfo.Items[i].IDInternal = strings.TrimSpace(req.PaymentInfo.Items[i].IDInternal)
	}
	req.PaymentInfo.DigitalRubleData.SmartContractID = strings.TrimSpace(req.PaymentInfo.DigitalRubleData.SmartContractID)

	return req
}

func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	return phone
}

func onlyDigits(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func validatePaymentStatus(fl playground.FieldLevel) bool {
	return dto.IsValidPaymentStatus(fl.Field().String())
}

func validatePaymentMethod(fl playground.FieldLevel) bool {
	switch dto.PaymentMethodType(fl.Field().String()) {
	case dto.PaymentMethodSBP, dto.PaymentMethodCard, dto.PaymentMethodDigitalWallet, dto.PaymentMethodDigitalRuble:
		return true
	default:
		return false
	}
}

func validateCurrency(fl playground.FieldLevel) bool {
	return fl.Field().String() == "RUB"
}

func validatePhoneRU(fl playground.FieldLevel) bool {
	return phoneRegexp.MatchString(fl.Field().String())
}

func validateCardNumber(fl playground.FieldLevel) bool {
	cardNumber := fl.Field().String()
	return cardDigitsRegexp.MatchString(cardNumber) && luhnValid(cardNumber)
}

func validateCardExpiration(fl playground.FieldLevel) bool {
	value := fl.Field().String()
	if !cardDateRegexp.MatchString(value) {
		return false
	}

	parts := strings.Split(value, "/")
	month := atoi(parts[0])
	year := 2000 + atoi(parts[1])

	now := time.Now().UTC()
	// Карта действительна до конца указанного месяца.
	expiration := time.Date(year, time.Month(month)+1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)
	return expiration.After(now)
}

func validateCVV(fl playground.FieldLevel) bool {
	return cvvRegexp.MatchString(fl.Field().String())
}

func validateWalletID(fl playground.FieldLevel) bool {
	return walletIDRegexp.MatchString(fl.Field().String())
}

func validateDigitalRubleIdentifier(fl playground.FieldLevel) bool {
	return drIdentifierRegexp.MatchString(fl.Field().String())
}

func validateDigitalRubleAccount(fl playground.FieldLevel) bool {
	return drAccountRegexp.MatchString(fl.Field().String())
}

func validatePaymentMethodSpecificFields(req dto.CreatePaymentRequest) error {
	customer := req.PaymentInfo.CustomerData

	switch req.PaymentInfo.PaymentMethodData.Type {
	case dto.PaymentMethodSBP:
		if customer.Phone == "" {
			return errors.New("customer_data.phone обязателен для оплаты через СБП")
		}
	case dto.PaymentMethodCard:
		missing := make([]string, 0, 3)
		if customer.CardNumber == "" {
			missing = append(missing, "customer_data.card_number")
		}
		if customer.CardDate == "" {
			missing = append(missing, "customer_data.card_date")
		}
		if customer.CvvCode == "" {
			missing = append(missing, "customer_data.CVV_code")
		}
		if len(missing) > 0 {
			return fmt.Errorf("для оплаты банковской картой обязательны поля: %s", strings.Join(missing, ", "))
		}
	case dto.PaymentMethodDigitalWallet:
		if customer.DigitalWalletID == "" {
			return errors.New("customer_data.digital_wallet_id обязателен для оплаты цифровым кошельком")
		}
	case dto.PaymentMethodDigitalRuble:
		if customer.DigitalRubleWalletID == "" && customer.DigitalRubleIdentifier == "" && customer.DigitalWalletID == "" {
			return errors.New("для оплаты цифровым рублем укажите customer_data.digital_ruble_wallet_id или customer_data.digital_ruble_identifier")
		}
	}

	return nil
}

func luhnValid(number string) bool {
	sum := 0
	double := false
	for i := len(number) - 1; i >= 0; i-- {
		digit := int(number[i] - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return sum%10 == 0
}

func atoi(value string) int {
	result := 0
	for _, r := range value {
		result = result*10 + int(r-'0')
	}
	return result
}

func formatValidationError(err error) error {
	var validationErrors playground.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return err
	}

	messages := make([]string, 0, len(validationErrors))
	for _, fieldErr := range validationErrors {
		messages = append(messages, validationErrorMessage(fieldErr))
	}
	return errors.New(strings.Join(messages, "; "))
}

func validationErrorMessage(err playground.FieldError) string {
	field := jsonFieldName(err.Namespace())

	switch err.Tag() {
	case "required":
		return field + " обязателен"
	case "min":
		return fmt.Sprintf("%s должен быть не короче %s символов", field, err.Param())
	case "max":
		return fmt.Sprintf("%s должен быть не длиннее %s символов", field, err.Param())
	case "gt":
		return field + " должен быть больше 0"
	case "lte":
		return fmt.Sprintf("%s не должен превышать %.2f", field, maxAmountValue)
	case "payment_status":
		return field + " содержит недопустимый статус платежа"
	case "oneof":
		return field + " содержит недопустимое значение"
	case "email":
		return field + " должен быть корректным email"
	case "payment_method":
		return field + " должен быть одним из: СБП, Банковская карта, Цифровой кошелек, Цифровой рубль"
	case "currency":
		return field + " должен быть RUB"
	case "phone_ru":
		return field + " должен быть в формате +79991234567"
	case "card_number":
		return field + " должен быть корректным номером карты"
	case "card_exp":
		return field + " должен быть в формате ММ/ГГ и не быть просроченным"
	case "cvv":
		return field + " должен состоять из 3 или 4 цифр"
	case "wallet_id":
		return field + " должен содержать 3-64 символа: латинские буквы, цифры, _ или -"
	case "digital_ruble_identifier":
		return field + " должен содержать 3-70 символов: латинские буквы, цифры, _, ., :, -"
	case "digital_ruble_account":
		return field + " должен содержать до 34 латинских букв или цифр"
	default:
		return field + " не прошёл проверку " + err.Tag()
	}
}

func jsonFieldName(namespace string) string {
	replacements := map[string]string{
		"createPaymentValidationModel": "request",
		"PaymentInfo":                  "payment_info",
		"Amount":                       "amount",
		"PaymentMethodData":            "payment_method_data",
		"CustomerData":                 "customer_data",
		"MerchantID":                   "merchant_id",
		"IdempotencyKey":               "idempotency_key",
		"PaymentID":                    "payment_id",
		"CurrentStatus":                "current_status",
		"Value":                        "value",
		"Currency":                     "currency",
		"Type":                         "type",
		"Email":                        "email",
		"Phone":                        "phone",
		"CardNumber":                   "card_number",
		"CardDate":                     "card_date",
		"CvvCode":                      "CVV_code",
		"DigitalWalletID":              "digital_wallet_id",
		"DigitalRubleWalletID":         "digital_ruble_wallet_id",
		"DigitalRubleAccount":          "digital_ruble_account",
		"DigitalRubleIdentifier":       "digital_ruble_identifier",
		"Items":                        "items",
		"Category":                     "category",
		"DigitalRubleData":             "digital_ruble_data",
		"SmartContractID":              "smart_contract_id",
		"CreatedAt":                    "created_at",
		"Description":                  "description",
	}

	parts := strings.Split(namespace, ".")
	for i, part := range parts {
		if replacement, ok := replacements[part]; ok {
			parts[i] = replacement
		}
	}

	if len(parts) > 0 && parts[0] == "request" {
		parts = parts[1:]
	}
	return strings.Join(parts, ".")
}
