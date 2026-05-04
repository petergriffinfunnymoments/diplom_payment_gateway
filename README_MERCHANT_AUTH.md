# Авторизация мерчантов

Добавлена первичная авторизация интернет-магазина на уровне API-шлюза. Теперь методы `POST /payments` и `GET /payments/{payment_id}` требуют заголовки:

```http
X-Merchant-ID: merchant_12345
X-API-Key: demo_api_key
X-Timestamp: 1777753086
X-Signature: <hmac_sha256>
```

Webhook от ЮKassa (`POST /webhooks/yookassa`) не подписывается этими заголовками, потому что это входящий вызов от внешней платежной системы, а не запрос от интернет-магазина.

## Как считается подпись

Каноническая строка:

```text
X-Timestamp + "." + METHOD + "." + REQUEST_URI + "." + SHA256_HEX(BODY)
```

Пример для создания платежа:

```text
1777753086.POST./payments.<sha256 тела JSON>
```

Пример для проверки статуса:

```text
1777753086.GET./payments/pay_123?merchant_id=merchant_12345.<sha256 пустого тела>
```

Итоговая подпись:

```text
HMAC_SHA256_HEX(secret_key, canonical_string)
```

## Демо-ключи по умолчанию

Если переменные окружения не заданы, приложение создаёт демонстрационного мерчанта:

```powershell
$env:MERCHANT_ID="merchant_12345"
$env:MERCHANT_API_KEY="demo_api_key"
$env:MERCHANT_SECRET_KEY="demo_secret_key"
```

В рабочем режиме лучше явно задать свои значения перед запуском:

```powershell
$env:MERCHANT_ID="merchant_12345"
$env:MERCHANT_NAME="Демонстрационный интернет-магазин"
$env:MERCHANT_API_KEY="my_demo_api_key"
$env:MERCHANT_SECRET_KEY="my_demo_secret_key"
```

Важно: если меняешь `MERCHANT_API_KEY` и `MERCHANT_SECRET_KEY`, нужно также поменять демо-константы `MERCHANT_AUTH` в `web/static/app.js`, иначе фронтенд будет подписывать запросы старыми ключами.

## Что создаётся в PostgreSQL

При запуске с `DATABASE_URL` автоматически создаётся таблица:

```sql
merchants
```

Она хранит:

```text
merchant_id
name
api_key_hash
secret_key
active
created_at
updated_at
```

API-ключ хранится не в открытом виде, а как SHA-256-хеш. Secret key хранится открыто в учебном прототипе, потому что он нужен для проверки HMAC. В промышленной версии его следует хранить в KMS/Vault или хотя бы шифровать на уровне приложения.

## Проверка

Без заголовков:

```powershell
Invoke-RestMethod -Uri "http://localhost:8080/payments/pay_123?merchant_id=merchant_12345" -Method Get
```

Ожидаемо:

```json
{
  "code": "AUTHENTICATION_ERROR",
  "message": "missing merchant authentication headers"
}
```

Через сайт интернет-магазина всё должно работать автоматически: `web/static/app.js` формирует HMAC-подпись перед `fetch`.
