# Возвраты платежей

Шлюз поддерживает API возвратов в стиле Paypalich/Pally Refund Resource:

- `POST /refunds/full/create` — полный возврат платежа;
- `POST /refunds/partial/create` — частичный возврат платежа;
- `GET /refunds/status?id=ref_...&merchant_id=merchant_12345` — статус возврата;
- `GET /refunds/search?merchant_id=merchant_12345&payment_id=pay_...` — список возвратов.

Все endpoint-ы требуют merchant authentication: `X-Merchant-ID`, `X-API-Key`, `X-Timestamp`, `X-Signature`.

## Полный возврат

```json
{
  "merchant_id": "merchant_12345",
  "payment_id": "pay_...",
  "idempotency_key": "uuid",
  "reason": "requested_by_customer"
}
```

## Частичный возврат

```json
{
  "merchant_id": "merchant_12345",
  "payment_id": "pay_...",
  "idempotency_key": "uuid",
  "amount": {
    "value": 500,
    "currency": "RUB"
  },
  "reason": "requested_by_customer"
}
```

## Ответ

```json
{
  "data": {
    "id": "ref_...",
    "status": "SUCCESS",
    "amount": 500,
    "currency": "RUB",
    "entity_type": "payment",
    "entity_id": "pay_...",
    "payment_id": "pay_...",
    "provider": "yookassa",
    "payment_system": "YOOKASSA",
    "external_refund_id": "...",
    "provider_status": "succeeded",
    "refund_type": "partial",
    "created_at": "..."
  },
  "success": true
}
```

Статусы возврата:

- `NEW`;
- `PROCESS`;
- `SUCCESS`;
- `FAIL`.

Возврат разрешён только для платежей в статусе `CAPTURED`.

## Адаптеры

- `yookassa` отправляет `POST https://api.yookassa.ru/v3/refunds` с `payment_id`, `amount` и заголовком `Idempotence-Key`.
- `stripe` отправляет `POST https://api.stripe.com/v1/refunds`; если в транзакции сохранён Checkout Session ID, адаптер сначала получает Session и берёт `payment_intent`.
- `simulated` и совместимый `dummy` выполняют локальную эмуляцию успешного возврата.

Для PostgreSQL автоматически создаётся таблица `payment_refunds`.
