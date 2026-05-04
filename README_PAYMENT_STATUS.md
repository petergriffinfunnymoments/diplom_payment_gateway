# GET /payments/{payment_id}

Добавлен API-метод получения актуального статуса платежа.

## Endpoint

```http
GET /payments/{payment_id}?merchant_id=merchant_12345
```

Пример:

```powershell
Invoke-RestMethod `
  -Uri "http://localhost:8080/payments/pay_123?merchant_id=merchant_12345" `
  -Method Get
```

Ответом является последний сохранённый `PaymentResponse` из таблицы `payment_transactions`.

## Зачем это нужно

Метод нужен интернет-магазину для ручной проверки статуса платежа, например если webhook от внешней платёжной системы не дошёл или магазин хочет повторно проверить состояние заказа.

Типовой сценарий:

1. `POST /payments` создаёт платёж.
2. Шлюз возвращает `PENDING` и `payment_url` внешней платёжной системы.
3. Пользователь оплачивает платёж.
4. Webhook обновляет статус в `payment_transactions`.
5. Интернет-магазин вызывает `GET /payments/{payment_id}` и получает актуальный статус: `CAPTURED`, `DECLINED`, `FAILED` или `PENDING`.

## Проверка через PowerShell

```powershell
Invoke-RestMethod `
  -Uri "http://localhost:8080/payments/ТВОЙ_PAYMENT_ID?merchant_id=merchant_12345" `
  -Method Get
```

## Проверка через фронтенд

На странице интернет-магазина добавлен блок «Проверка статуса платежа». После создания платежа поле `payment_id` заполняется автоматически. После оплаты в ЮKassa можно нажать «Проверить статус» и увидеть обновлённый `PaymentResponse`.

## Проверка через pg-tools.ps1

```powershell
pgpayment pay_ТВОЙ_ID
```
