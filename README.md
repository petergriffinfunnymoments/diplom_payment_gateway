# Payment Gateway

Учебный платежный шлюз на Go для дипломного проекта. Шлюз принимает платежные запросы от интернет-магазинов, аутентифицирует мерчантов, валидирует платежные данные, выполняет антифрод-проверку, токенизирует чувствительные данные, выбирает платежного провайдера, сохраняет состояние в PostgreSQL, принимает webhook-и провайдеров и отправляет уведомления мерчанту.

Проект работает как API-only backend. Старый тестовый фронтенд удален: все основные операции удобнее проверять через Postman или прямые HTTP-запросы.

## Архитектура

В шлюзе выделены 9 подсистем:

1. API-шлюз.
2. Модуль валидации платежных данных.
3. Оркестратор платежей.
4. Антифрод.
5. Модуль токенизации.
6. Адаптер платежной системы.
7. Сервис уведомлений.
8. Модуль логирования.
9. База данных.

Основной поток платежа:

```text
POST /payments
  -> merchant authentication
  -> validation
  -> orchestrator
  -> antifraud
  -> tokenization
  -> payment router
  -> provider adapter
  -> PostgreSQL
  -> event logging
  -> merchant notification
```

Финальный статус может обновляться webhook-ом от внешнего провайдера:

```text
provider webhook -> /webhooks/yookassa или /webhooks/stripe -> DB -> merchant notification
```

## Быстрый запуск

1. Запусти PostgreSQL.
2. Скопируй пример локальных переменных:

```powershell
Copy-Item .\payment_gateway_tools\run.local.example.ps1 .\payment_gateway_tools\run.local.ps1
```

3. Заполни реальные значения в `payment_gateway_tools/run.local.ps1`. Этот файл находится в `.gitignore`.
4. Примени переменные и запусти шлюз:

```powershell
. .\payment_gateway_tools\run.local.ps1
go run ./cmd/payment-gateway
```

Проверка:

```text
GET http://localhost:8080/health
```

Минимальные переменные окружения:

```powershell
$env:DATABASE_URL="postgres://postgres:<PASSWORD>@localhost:5432/payment_gateway?sslmode=disable"

$env:MERCHANT_ID="merchant_12345"
$env:MERCHANT_API_KEY="demo_api_key"
$env:MERCHANT_SECRET_KEY="demo_secret_key"

$env:PAYMENT_RETURN_URL="http://localhost:8080"
$env:MERCHANT_WEBHOOK_URL="http://localhost:8080/merchant/webhook"
$env:MERCHANT_WEBHOOK_SECRET="demo_secret"
```

Для реальных redirect-провайдеров также нужны:

```powershell
$env:YOOKASSA_SHOP_ID="..."
$env:YOOKASSA_SECRET_KEY="..."

$env:STRIPE_SECRET_KEY="..."
$env:STRIPE_WEBHOOK_SECRET="..."
```

## Postman

Файлы для Postman находятся в `payment_gateway_tools`:

```text
postman_collection.json
postman_environment.json
```

Импортируй оба файла в Postman и выбери environment `Payment Gateway Local`.

Проверь переменные:

```text
base_url
merchant_id
api_key
secret_key
```

Коллекция автоматически считает HMAC-подпись через pre-request script и добавляет заголовки:

```text
X-Merchant-ID
X-API-Key
X-Timestamp
X-Signature
```

Основные запросы в коллекции:

```text
GET  /health
GET  /payments/__auth_check__ - Check current credentials
POST /payments - Card MIR
POST /payments - Card Visa
POST /payments - Digital Ruble
GET  /payments/{payment_id}
POST /refunds/full/create
POST /refunds/partial/create
GET  /refunds/status
GET  /refunds/search
GET  /reports/transactions - Merchant statistics
POST /merchant/webhook - demo
```

Запрос `GET /payments/__auth_check__` считается успешной проверкой credentials, если вернулся `404 PAYMENT_NOT_FOUND`: это значит, что авторизация прошла, а тестовый платеж ожидаемо не найден.

В Postman Visualize добавлены представления платежей и отчетов: карточки KPI, диаграммы статусов, активности по дням, платежных систем, способов оплаты и потоковая схема подсистем шлюза.

## API

### Платежи

Создать платеж:

```text
POST /payments
```

Пример тела:

```json
{
  "merchant_id": "merchant_12345",
  "idempotency_key": "uuid",
  "payment_id": "pay_demo_001",
  "current_status": "CREATED",
  "payment_info": {
    "amount": {
      "value": 1500,
      "currency": "RUB"
    },
    "payment_method_data": {
      "type": "Банковская карта"
    },
    "customer_data": {
      "email": "customer@example.com",
      "phone": "+79991234567",
      "card_number": "2200000000000004",
      "card_date": "12/29",
      "CVV_code": "123"
    },
    "description": "Оплата заказа"
  }
}
```

Получить статус:

```text
GET /payments/{payment_id}?merchant_id=merchant_12345
```

Статусы платежей:

```text
CREATED
PENDING
AUTHORIZED
CAPTURE_REQUESTED
CAPTURED
DECLINED
CANCELLED
FAILED
VOID_REQUESTED
VOID_FAILED
VOIDED
```

### Возвраты

API возвратов сделан в стиле Pally/Paypalich Refund Resource:

```text
POST /refunds/full/create
POST /refunds/partial/create
GET  /refunds/status?id=ref_...&merchant_id=merchant_12345
GET  /refunds/search?merchant_id=merchant_12345&payment_id=pay_...
```

Полный возврат:

```json
{
  "merchant_id": "merchant_12345",
  "payment_id": "pay_...",
  "idempotency_key": "uuid",
  "reason": "requested_by_customer"
}
```

Частичный возврат:

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

Возврат разрешен только для платежей в статусе `CAPTURED`.

Статусы возврата:

```text
NEW
PROCESS
SUCCESS
FAIL
```

### Отчеты

```text
GET /reports/transactions?merchant_id=merchant_12345
```

Фильтры:

```text
date_from
date_to
status
payment_system
payment_method
limit
```

Обычный мерчант может получать статистику только по своему `merchant_id`. Роли `admin` и `auditor` могут читать отчеты по любому мерчанту.

### Webhook-и

```text
POST /webhooks/yookassa
POST /webhooks/stripe
POST /merchant/webhook
```

Webhook-и внешних платежных систем не требуют merchant HMAC. Demo endpoint `/merchant/webhook` нужен для локальной проверки уведомлений интернет-магазина.

## Merchant Authentication

Платежи, статусы, возвраты и отчеты требуют подпись мерчанта.

Заголовки:

```text
X-Merchant-ID
X-API-Key
X-Timestamp
X-Signature
```

Canonical string:

```text
timestamp.METHOD.REQUEST_URI.SHA256(body)
```

Signature:

```text
HMAC-SHA256(secret_key, canonical_string)
```

`api_key` хранится как SHA-256 hash. `secret_key` нужен для проверки HMAC; в учебном режиме он может храниться открыто, а в усиленном режиме шифруется через HashiCorp Vault Transit.

## Роли

Поддерживаются роли:

```text
merchant
admin
auditor
```

`merchant` может создавать платежи и возвраты, а также читать только свои платежи, возвраты и отчеты.

`admin` предназначен для внутреннего оператора шлюза и может работать с данными любого мерчанта.

`auditor` имеет read-only доступ к статусам, возвратам и отчетам по мерчантам, но не может создавать платежи и возвраты.

Неизвестные роли отклоняются при аутентификации. Отказы доступа логируются событием `authorization_failed`.

## Управление мерчантами

Создать мерчанта:

```powershell
.\payment_gateway_tools\create-merchant.ps1 `
  -MerchantID "merchant_books" `
  -Name "Book Shop" `
  -WebhookURL "https://books-shop.example.com/payment-webhook"
```

Создать администратора:

```powershell
.\payment_gateway_tools\create-merchant.ps1 `
  -MerchantID "admin_1" `
  -Name "Gateway Admin" `
  -Role admin
```

Создать аудитора:

```powershell
.\payment_gateway_tools\create-merchant.ps1 `
  -MerchantID "auditor_1" `
  -Name "Gateway Auditor" `
  -Role auditor
```

Перевыпустить ключи:

```powershell
.\payment_gateway_tools\create-merchant.ps1 `
  -MerchantID "merchant_books" `
  -Name "Book Shop" `
  -RotateKeys
```

Посмотреть мерчантов:

```powershell
. .\payment_gateway_tools\merchant-admin-tools.ps1
pgmerchants
pgmerchant merchant_books
```

Отключить, включить или изменить роль:

```powershell
pgmerchantdisable merchant_books
pgmerchantenable merchant_books
pgmerchantrole admin_1 admin
```

## Маршрутизация платежей

Бизнес-выбор провайдера выполняет router внутри оркестратора: `internal/orchestrator/simple/router.go`.

Инфраструктурное хранилище правил маршрутизации находится отдельно: `internal/subsystems/routing/postgres_routes.go`.

Для банковских карт сначала применяется маршрутизация по платежной системе карты:

```text
МИР 2200-2204       -> yookassa
Visa 4             -> stripe
Mastercard 51-55   -> stripe
Mastercard 2221-2720 -> stripe
```

Если схему карты определить нельзя, router смотрит таблицу `merchant_payment_routes`. Если правила нет, используется fallback через переменные окружения или `dummy`.

Добавить маршрут:

```powershell
. .\payment_gateway_tools\payment-route-tools.ps1
pgrouteadd "merchant_12345" "Цифровой рубль" digital_ruble 1 DIGITAL_RUBLE
```

Посмотреть маршруты:

```powershell
pgroutes
pgmerchant_routes merchant_12345
```

Отключить маршрут:

```powershell
pgroutedisable "merchant_12345" "Банковская карта" yookassa
```

## Адаптеры

Фабрика адаптеров находится в `internal/subsystems/adapter/factory.go`.

Поддерживаемые provider key:

```text
yookassa
stripe
tbank
digital_ruble
simulated
dummy
```

`dummy` является совместимым alias для локального эмуляционного адаптера.

YooKassa создает redirect-платеж и возвращает `payment_url`; финальный статус обновляется через `/webhooks/yookassa`.

Stripe создает Checkout Session и возвращает `payment_url`; финальный статус обновляется через `/webhooks/stripe`.

Digital Ruble является эмуляционным адаптером банка-участника, потому что реальная платформа цифрового рубля не предоставляет публичный sandbox API для произвольного подключения.

## Цифровой рубль

Способ оплаты:

```json
{
  "payment_method_data": {
    "type": "Цифровой рубль"
  }
}
```

Минимальные customer fields:

```json
{
  "digital_ruble_wallet_id": "dr_wallet_123"
}
```

Также поддерживаются `digital_ruble_identifier` и совместимый fallback `digital_wallet_id`.

Тестовые сценарии:

```text
dr_wallet_123      -> CAPTURED
dr_wallet_declined -> DECLINED
dr_wallet_error    -> FAILED
dr_wallet_pending  -> PENDING
другое значение    -> PENDING, provider_status=qr_issued
```

Ответ содержит поля QR/банка-участника:

```json
{
  "payment_system": "DIGITAL_RUBLE",
  "qr_id": "drqr_...",
  "qr_payload": "drub://...",
  "qr_expires_at": "...",
  "participant_bank": "BANK_PARTNER_1",
  "schema_version": "drub.v1",
  "settlement_hint": "RUB + DIGITAL_RUBLE; settlement through participant bank emulator"
}
```

Опциональные переменные:

```powershell
$env:DIGITAL_RUBLE_PARTICIPANT_BANK="BANK_PARTNER_1"
$env:DIGITAL_RUBLE_SCHEMA_VERSION="drub.v1"
$env:DIGITAL_RUBLE_QR_TTL_SECONDS="900"
```

## Антифрод

Антифрод расположен в `internal/subsystems/antifraud`.

Результаты:

```text
PASSED
REVIEW
BLOCKED
```

Примеры правил:

```text
сумма >= 500000 -> BLOCKED
сумма >= 100000 -> REVIEW
suspicious email/phone -> REVIEW или BLOCKED
blocked_wallet -> BLOCKED
```

Если антифрод возвращает `BLOCKED`, оркестратор выставляет `DECLINED`, сохраняет транзакцию, пишет лог и отправляет уведомление мерчанту.

## Токенизация и защита cardholder data

Токенизатор расположен в `internal/subsystems/tokenizer`.

PostgreSQL-таблица:

```text
payment_tokens
```

Хранятся:

```text
merchant_id
payment_id
idempotency_key
token_hash
token_preview
payment_method
masked_value
expires_at
created_at
revoked_at
```

Полный номер карты, CVV и полный внутренний токен не должны сохраняться.

После успешной валидации CVV удаляется из DTO, поэтому антифрод, токенизатор, router, adapter, БД, логи и уведомления получают уже CVV-free данные.

Проверка:

```sql
SELECT id, merchant_id, payment_id, token_preview, payment_method, masked_value, expires_at, created_at
FROM payment_tokens
ORDER BY id DESC
LIMIT 20;
```

## Логирование

Логирование расположено в `internal/subsystems/logging`.

Основные таблицы:

```text
services
log_events
log_entries
log_context
```

Примеры событий:

```text
payment_received
payment_validated
fraud_checked
tokenized
adapter_called
adapter_result_received
payment_response_sent
payment_failed
notification_sent
notification_failed
merchant_webhook_received
authorization_failed
refund_requested
refund_adapter_called
refund_adapter_result_received
refund_response_sent
refund_failed
```

Команды:

```powershell
. .\payment_gateway_tools\pg-tools.ps1
pglogs 30
pgpay pay_...
pgctx pay_...
```

## Уведомления мерчанту

Сервис уведомлений отправляет POST-webhook после изменения статуса платежа.

Переменные:

```powershell
$env:MERCHANT_WEBHOOK_URL="https://example.com/merchant/webhook"
$env:MERCHANT_WEBHOOK_SECRET="demo_secret"
```

Если `MERCHANT_WEBHOOK_URL` пустой, уведомления отключены, но шлюз продолжает работать.

Если задан `MERCHANT_WEBHOOK_SECRET`, шлюз добавляет подпись:

```text
X-Payment-Gateway-Signature: sha256=<hmac_sha256_body>
```

Проверка:

```powershell
pgnotifications 20
```

или SQL:

```sql
SELECT *
FROM notification_deliveries
ORDER BY created_at DESC
LIMIT 20;
```

## HashiCorp Vault

Vault Transit можно использовать для защиты `merchants.secret_key`.

Data path:

```text
payment-gateway -> Vault Transit encrypt/decrypt -> PostgreSQL хранит vault:v1:<ciphertext>
```

Локальный dev Vault:

```powershell
docker run --rm -p 8200:8200 `
  --cap-add=IPC_LOCK `
  -e VAULT_DEV_ROOT_TOKEN_ID=root `
  hashicorp/vault:latest
```

Инициализация Transit:

```powershell
$env:VAULT_ADDR="http://127.0.0.1:8200"
$env:VAULT_TOKEN="root"

vault secrets enable transit
vault write -f transit/keys/payment-gateway-merchant-secrets
```

Переменные шлюза:

```powershell
$env:SECRET_PROTECTOR="vault_transit"
$env:VAULT_ADDR="http://127.0.0.1:8200"
$env:VAULT_TOKEN="root"
$env:VAULT_TRANSIT_MOUNT="transit"
$env:VAULT_TRANSIT_KEY="payment-gateway-merchant-secrets"
```

В production вместо `VAULT_TOKEN` лучше использовать `VAULT_TOKEN_FILE`, AppRole, Kubernetes auth или другой механизм короткоживущих токенов.

Проверка:

```sql
SELECT merchant_id, left(secret_key, 8) AS secret_prefix
FROM merchants;
```

Ожидаемый префикс:

```text
vault:v1
```

## Transport Security

Для локальной разработки можно использовать:

```text
http://localhost:8080
```

Для режима PCI DSS Requirement 4 включи TLS в приложении:

```powershell
$env:TLS_CERT_FILE="C:\path\gateway.crt"
$env:TLS_KEY_FILE="C:\path\gateway.key"
$env:REQUIRE_HTTPS="true"
```

Если HTTPS завершается на reverse proxy или LocalTunnel:

```powershell
$env:REQUIRE_HTTPS="true"
$env:TRUST_PROXY_HEADERS="true"
```

При `REQUIRE_HTTPS=true` значения `PAYMENT_RETURN_URL` и `MERCHANT_WEBHOOK_URL` должны начинаться с `https://`.

Шлюз добавляет security headers:

```text
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: no-referrer
Cache-Control: no-store
Strict-Transport-Security: max-age=31536000; includeSubDomains
```

## LocalTunnel

Если для демонстрации используется LocalTunnel, при смене URL обнови:

```text
PUBLIC_URL в run.local.ps1
PAYMENT_RETURN_URL
MERCHANT_WEBHOOK_URL
YooKassa webhook URL: /webhooks/yookassa
Stripe webhook URL: /webhooks/stripe
Postman base_url
```

LocalTunnel подходит для демонстрации webhook-ов, но production-контур должен использовать контролируемый TLS/reverse proxy.

## PCI DSS

Проект не является PCI DSS certified. Реализованы учебные технические меры, которые приближают шлюз к отдельным требованиям PCI DSS v4.0.1:

```text
Requirement 3  -> маскирование PAN, удаление CVV, token_hash/token_preview, Vault для secret_key
Requirement 4  -> HTTPS enforcement, TLS mode, proxy headers, security headers
Requirement 6  -> тесты, безопасная обработка ошибок, security-focused checks
Requirement 7  -> роли merchant/admin/auditor, ограничения доступа к данным мерчантов
Requirement 8  -> merchant authentication через api_key + HMAC signature
Requirement 10 -> журналирование событий обработки платежей и отказов доступа
Requirement 11 -> go test, go vet, govulncheck, secret scan, GitHub Actions
```

Ограничения:

```text
backend всё ещё принимает PAN/CVV во входном JSON для учебного сценария;
для промышленного сокращения PCI scope нужен hosted checkout/hosted fields/iframe;
нужны external ASV scans, penetration testing, IDS/FIM, SIEM и организационные процессы;
старые записи БД, созданные до маскирования, нужно очищать отдельной миграцией.
```

## Security checks

Локальный запуск:

```powershell
.\payment_gateway_tools\security-check.ps1
```

С установкой отсутствующих инструментов:

```powershell
.\payment_gateway_tools\security-check.ps1 -InstallMissingTools
```

Строгий режим CI:

```powershell
.\payment_gateway_tools\security-check.ps1 -InstallMissingTools -RequireGovulncheck
```

Проверяются:

```text
go test ./...
go vet ./...
govulncheck ./...
tracked file secret scan
```

GitHub Actions workflow:

```text
.github/workflows/security-checks.yml
```

## Полезные PowerShell-команды

Подключить инструменты:

```powershell
. .\payment_gateway_tools\pg-tools.ps1
. .\payment_gateway_tools\payment-route-tools.ps1
. .\payment_gateway_tools\merchant-admin-tools.ps1
```

Транзакции и логи:

```powershell
pgtx 10
pglogs 30
pgtokens 20
pgerrors 20
pgstatus
pgpay pay_...
pgctx pay_...
```

Маршруты:

```powershell
pgroutes
pgmerchant_routes merchant_12345
pgrouteadd "merchant_12345" "Цифровой рубль" digital_ruble 1 DIGITAL_RUBLE
pgroutedisable "merchant_12345" "Банковская карта" yookassa
```

Мерчанты:

```powershell
pgmerchants
pgmerchant merchant_12345
pgmerchantdisable merchant_12345
pgmerchantenable merchant_12345
```

## Типовые проблемы

Если Stripe выбран маршрутизатором, но `STRIPE_SECRET_KEY` не задан:

```text
adapter provider "stripe" is not registered or not configured
```

Если webhook-и мерчанту не доставляются, проверь:

```powershell
echo $env:MERCHANT_WEBHOOK_URL
```

Если YooKassa webhook не доходит, проверь URL в кабинете YooKassa:

```text
https://<public-url>/webhooks/yookassa
```

Если Stripe webhook не проходит проверку подписи, проверь:

```powershell
echo $env:STRIPE_WEBHOOK_SECRET
```

Если GitHub блокирует push, проверь, что реальные секреты не попали в отслеживаемые файлы:

```text
sk_test_
sk_live_
whsec_
YOOKASSA_SECRET_KEY
DATABASE_URL с паролем
```

Реальные ключи должны храниться только в `payment_gateway_tools/run.local.ps1` или другом локальном файле из `.gitignore`.
