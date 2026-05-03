# Сервис уведомлений интернет-магазина

Модуль уведомлений отправляет POST-webhook интернет-магазину после изменения статуса платежа. В учебной демонстрации в качестве интернет-магазина можно использовать endpoint самого проекта:

```text
POST /merchant/webhook
```

## Переменные окружения

```powershell
$env:MERCHANT_WEBHOOK_URL="https://YOUR_LOCALTUNNEL_URL.loca.lt/merchant/webhook"
$env:MERCHANT_WEBHOOK_SECRET="demo_secret" # необязательно
```

Если `MERCHANT_WEBHOOK_URL` пустой, уведомления отключены, но платежный шлюз продолжает работать.

## Формат уведомления

```json
{
  "event": "payment.captured",
  "payment_id": "pay_...",
  "merchant_id": "merchant_12345",
  "idempotency_key": "...",
  "status": "CAPTURED",
  "amount": {"value": 1500, "currency": "RUB"},
  "payment_method": "Банковская карта",
  "payment_system": "YOOKASSA",
  "provider_status": "succeeded",
  "external_transaction_id": "...",
  "occurred_at": "2026-05-02T20:18:06Z"
}
```

## Подпись

Если задан `MERCHANT_WEBHOOK_SECRET`, шлюз добавляет заголовок:

```text
X-Payment-Gateway-Signature: sha256=<hmac_sha256_body>
```

## Проверка

```sql
SELECT * FROM notification_deliveries ORDER BY created_at DESC LIMIT 20;
```

или через PowerShell-команду:

```powershell
pgnotifications 20
```
