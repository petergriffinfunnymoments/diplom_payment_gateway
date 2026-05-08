# Цифровой рубль

В проект добавлен первый интеграционный контур цифрового рубля как эмуляционный payment rail.

Важно: это не прямое подключение к платформе Банка России. Для дипломного прототипа шлюз моделирует реалистичную схему:

```text
payment gateway -> bank participant adapter -> digital ruble platform
```

То есть шлюз работает через адаптер банка-участника, а не как самостоятельный участник платформы цифрового рубля.

## Метод оплаты

```json
{
  "payment_method_data": {
    "type": "Цифровой рубль"
  }
}
```

Цифровой рубль внутри шлюза остаётся платежным методом в валюте `RUB`, а не отдельной валютой `DRUB`.

## Поля запроса

Минимальный пример:

```json
{
  "merchant_id": "merchant_12345",
  "idempotency_key": "uuid",
  "payment_id": "pay_...",
  "current_status": "CREATED",
  "payment_info": {
    "amount": {
      "value": 1500,
      "currency": "RUB"
    },
    "payment_method_data": {
      "type": "Цифровой рубль"
    },
    "customer_data": {
      "email": "customer@example.com",
      "phone": "+79991234567",
      "digital_ruble_wallet_id": "dr_wallet_123",
      "digital_ruble_account": "0000000000000000000000000000000000",
      "digital_ruble_identifier": "merchant:wallet:demo"
    },
    "description": "Оплата заказа цифровым рублем",
    "created_at": "2026-05-08T12:00:00Z"
  }
}
```

`digital_ruble_wallet_id` или `digital_ruble_identifier` обязателен. `digital_wallet_id` также принимается как совместимый fallback для ранних тестов.

## Эмуляционный адаптер

Provider key:

```text
digital_ruble
```

Тестовые сценарии:

- `dr_wallet_123` -> `CAPTURED`;
- `dr_wallet_declined` -> `DECLINED`;
- `dr_wallet_error` -> `FAILED`;
- `dr_wallet_pending` -> `PENDING`;
- любое другое значение -> `PENDING` и статус провайдера `qr_issued`.

## Ответ

В `transaction_details` добавлены поля для QR/банка-участника:

```json
{
  "payment_system": "DIGITAL_RUBLE",
  "provider_status": "settled",
  "qr_id": "drqr_...",
  "qr_payload": "drub://...",
  "qr_expires_at": "2026-05-08T12:15:00Z",
  "participant_bank": "BANK_PARTNER_1",
  "schema_version": "drub.v1",
  "settlement_hint": "RUB + DIGITAL_RUBLE; settlement through participant bank emulator"
}
```

## Переменные окружения

Опционально:

```powershell
$env:DIGITAL_RUBLE_PARTICIPANT_BANK="BANK_PARTNER_1"
$env:DIGITAL_RUBLE_SCHEMA_VERSION="drub.v1"
$env:DIGITAL_RUBLE_QR_TTL_SECONDS="900"
```

## Проверка через Postman

В коллекцию Postman добавлен запрос:

```text
POST /payments - Digital Ruble
```

Он проходит через обычную цепочку:

```text
API -> merchant auth -> validation -> antifraud -> tokenization -> router -> DigitalRubleAdapter -> DB -> logging -> notifications
```
