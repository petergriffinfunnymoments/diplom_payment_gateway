# YooKassa webhook и подтверждение тестового платежа

## Что добавлено

1. Endpoint для входящих уведомлений YooKassa:

```text
POST /webhooks/yookassa
```

2. Обработчик webhook:

```text
internal/httpapi/webhooks/yookassa_handler.go
```

Он принимает уведомления YooKassa, дополнительно проверяет актуальный статус платежа через `GET /v3/payments/{id}`, обновляет сохранённый `PaymentResponse` в `payment_transactions` и пишет событие в модуль логирования.

3. На странице интернет-магазина теперь появляется ссылка:

```text
Перейти к оплате в ЮKassa
```

Она появляется, если адаптер вернул `transaction_details.payment_url`.

## Важное уточнение

Webhook сам по себе не создаёт платеж в YooKassa. Чтобы платеж появился в тестовом кабинете YooKassa, шлюз должен создать платеж через YooKassaAdapter, а пользователь должен перейти по `payment_url` и подтвердить оплату тестовой картой.

## Переменные окружения для запуска

```powershell
$env:DATABASE_URL="postgres://postgres:ТВОЙ_ПАРОЛЬ@localhost:5432/payment_gateway?sslmode=disable"
$env:YOOKASSA_SHOP_ID="ТВОЙ_SHOP_ID"
$env:YOOKASSA_SECRET_KEY="ТВОЙ_СЕКРЕТНЫЙ_КЛЮЧ"
$env:CARD_PAYMENT_PROVIDER="yookassa"
$env:PAYMENT_RETURN_URL="http://localhost:8080"

go run ./cmd/payment-gateway
```

## Как открыть локальный сервер для webhook

YooKassa требует публичный HTTPS URL для уведомлений. Для локальной разработки удобно использовать ngrok:

```powershell
ngrok http 8080
```

Скопируй HTTPS URL вида:

```text
https://abc123.ngrok-free.app
```

Webhook URL для YooKassa будет:

```text
https://abc123.ngrok-free.app/webhooks/yookassa
```

## Что включить в кабинете YooKassa

В тестовом магазине открой:

```text
Интеграция → HTTP-уведомления
```

Укажи URL:

```text
https://abc123.ngrok-free.app/webhooks/yookassa
```

Подпишись минимум на события:

```text
payment.succeeded
payment.canceled
```

Можно также включить:

```text
payment.waiting_for_capture
```

## Проверка

1. Запусти сервер.
2. Открой `http://localhost:8080`.
3. Выбери оплату банковской картой.
4. Нажми оплатить.
5. В ответе должен быть статус `PENDING` и `payment_url`.
6. Перейди по ссылке «Перейти к оплате в ЮKassa».
7. Оплати тестовой картой из документации YooKassa.
8. После оплаты YooKassa отправит webhook в `/webhooks/yookassa`.
9. Проверь транзакции и логи.

Последние транзакции:

```sql
SELECT
    id,
    merchant_id,
    payment_id,
    status,
    payload_json,
    updated_at
FROM payment_transactions
ORDER BY updated_at DESC
LIMIT 10;
```

Последние события логирования:

```sql
SELECT
    le.timestamp,
    le.level,
    s.code AS service,
    e.code AS event,
    le.current_status,
    le.payment_id,
    le.message
FROM log_entries le
JOIN services s ON s.id = le.service_id
JOIN log_events e ON e.id = le.event_id
ORDER BY le.timestamp DESC
LIMIT 30;
```
