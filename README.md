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
provider webhook -> /webhooks/yookassa, /webhooks/robokassa, /webhooks/payanyway или /webhooks/digital-ruble -> DB -> merchant notification
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

$env:ROBOKASSA_MERCHANT_LOGIN="..."
$env:ROBOKASSA_TEST_PASSWORD1="..."
$env:ROBOKASSA_TEST_PASSWORD2="..."
$env:ROBOKASSA_TEST_MODE="true"
$env:ROBOKASSA_HASH_ALGORITHM="md5"
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
POST /sandbox/digital-ruble/scan - Capture QR
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
CAPTURE_REQUESTED
CAPTURED
DECLINED
CANCELLED
FAILED
```

## Справочник внутренних ошибок

Основные статусы платежей не расширяются под каждую причину отказа. Конкретная причина хранится отдельно:

```json
{
  "current_status": "DECLINED",
  "error": {
    "code": "ANTIFRAUD_DECLINED",
    "message": "payment blocked by antifraud"
  },
  "transaction_details": {
    "provider_error_code": "fraud_suspected",
    "provider_error_message": "fraud_suspected"
  }
}
```

Внутренний справочник находится в:

```text
internal/dto/error_catalog.go
```

Правило разделения:

```text
current_status              -> итоговое состояние платежа: DECLINED, FAILED, CAPTURED и т.д.
error.code                  -> внутренний стабильный код ошибки шлюза
error.message               -> человекочитаемое описание причины
provider_error_code         -> сырой код внешнего провайдера, если он есть
provider_error_message      -> сообщение внешнего провайдера
```

Основные внутренние коды:

```text
BAD_REQUEST                 -> некорректный HTTP-запрос или JSON
AUTHENTICATION_ERROR        -> ошибка merchant authentication
AUTH_CONTEXT_MISSING        -> отсутствует контекст аутентифицированного мерчанта
FORBIDDEN                   -> роль или merchant scope не позволяют операцию
NETWORK_ACCESS_DENIED       -> IP/CIDR источника не разрешён
HTTPS_REQUIRED              -> запрос отклонён из-за требования HTTPS

VALIDATION_ERROR            -> платёжные данные не прошли валидацию
ANTIFRAUD_ERROR             -> техническая ошибка антифрода
ANTIFRAUD_DECLINED          -> антифрод заблокировал платёж
ROUTING_ERROR               -> маршрутизатор не выбрал провайдера
TOKENIZATION_ERROR          -> ошибка токенизации
ADAPTER_FACTORY_ERROR       -> адаптер не зарегистрирован или не настроен
PROVIDER_UNAVAILABLE        -> внешний провайдер недоступен или вернул сетевую ошибку
ADAPTER_FAILED              -> адаптер вернул технический сбой
PAYMENT_DECLINED            -> провайдер отклонил платёж
CALLBACK_ERROR              -> ошибка формирования PaymentResponse

PAYMENT_NOT_FOUND           -> платёж не найден
PAYMENT_NOT_CAPTURED        -> операция разрешена только для CAPTURED платежа
INVALID_STORED_RESPONSE     -> сохранённый PaymentResponse повреждён или не читается

YOOKASSA_PAYMENT_DECLINED   -> YooKassa отклонила или отменила платёж
YOOKASSA_FRAUD_SUSPECTED    -> YooKassa вернула fraud_suspected
DIGITAL_RUBLE_PAYMENT_DECLINED -> эмулятор цифрового рубля отклонил платёж
DIGITAL_RUBLE_TECHNICAL_ERROR  -> техническая ошибка эмулятора цифрового рубля
DIGITAL_RUBLE_QR_EXPIRED       -> QR-код цифрового рубля истёк до подтверждения
```

Для возвратов и отчётов используются дополнительные коды:

```text
REFUND_STORE_UNAVAILABLE
REFUND_STORAGE_ERROR
REFUND_NOT_SUPPORTED
REFUND_NOT_FOUND
REFUND_FAILED
REPORT_STORE_UNAVAILABLE
REPORT_STORAGE_ERROR
MERCHANT_SCOPE_MISMATCH
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
PROCESS_REFUND
SUCCESS_REFUND
FAIL_REFUND
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
POST /webhooks/robokassa
POST /webhooks/payanyway
POST /webhooks/digital-ruble
POST /sandbox/digital-ruble/scan
POST /merchant/webhook
```

Webhook-и внешних платежных систем не требуют merchant HMAC. `/sandbox/digital-ruble/scan` — тестовый endpoint для эмуляции сканирования QR и подтверждения цифрового рубля. Demo endpoint `/merchant/webhook` нужен для локальной проверки уведомлений интернет-магазина.

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
```

Для остальных карт router смотрит таблицу `merchant_payment_routes`. Если правила нет, provider должен быть задан через переменные окружения (`CARD_PAYMENT_PROVIDER`, `SBP_PAYMENT_PROVIDER`, `PAYMENT_PROVIDER`) или запрос завершится ошибкой конфигурации.

Добавить маршрут:

```powershell
. .\payment_gateway_tools\payment-route-tools.ps1
pgrouteadd "merchant_12345" "Цифровой рубль" digital_ruble 1 DIGITAL_RUBLE
pgrouteadd "merchant_12345" "СБП" robokassa 1 ROBOKASSA
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
robokassa
payanyway
digital_ruble
simulated
```

`simulated` используется только для локальных проверок и benchmark-тестов.

YooKassa создает redirect-платеж и возвращает `payment_url`; финальный статус обновляется через `/webhooks/yookassa`.

Robokassa создает ссылку на платежную форму и возвращает `payment_url`; при тестовом режиме в ссылку добавляется `IsTest=1`. Финальный статус успешного платежа обновляется через `Result URL` `/webhooks/robokassa`.

PayAnyWay создает ссылку на платежную форму MONETA.Assistant и возвращает `payment_url`; при тестовом режиме в ссылку добавляется `MNT_TEST_MODE=1`. Финальный статус успешного платежа обновляется через `Pay URL` `/webhooks/payanyway`.

Digital Ruble является эмуляционным адаптером банка-участника, потому что реальная платформа цифрового рубля не предоставляет публичный sandbox API для произвольного подключения.

### Robokassa test mode

В кабинете Robokassa в технических настройках магазина задай отдельные тестовые пароли и Result URL:

```text
Result URL: https://<public-url>/webhooks/robokassa
Метод Result URL: POST
Алгоритм подписи: MD5
```

Локальные переменные:

```powershell
$env:ROBOKASSA_MERCHANT_LOGIN="..."
$env:ROBOKASSA_TEST_PASSWORD1="..."
$env:ROBOKASSA_TEST_PASSWORD2="..."
$env:ROBOKASSA_TEST_MODE="true"
$env:ROBOKASSA_HASH_ALGORITHM="md5"
```

Для тестового платежа через маршрутизатор удобно направить СБП в Robokassa:

```powershell
pgrouteadd "merchant_12345" "СБП" robokassa 1 ROBOKASSA
```

Тестовые платежи Robokassa могут не отображаться в поиске операций личного кабинета; проверяй результат по webhook-у, логам и `GET /payments/{payment_id}`.

### PayAnyWay test mode

В настройках PayAnyWay/MONETA укажи бизнес-счет и код проверки целостности данных. Если доступно поле Pay URL / URL уведомления об оплате, задай:

```text
Pay URL: https://<public-url>/webhooks/payanyway
Метод: POST
```

Локальные переменные:

```powershell
$env:PAYANYWAY_MNT_ID="..."
$env:PAYANYWAY_INTEGRITY_CODE="..."
$env:PAYANYWAY_TEST_MODE="true"
$env:PAYANYWAY_PAYMENT_URL="https://www.payanyway.ru/assistant.htm"
```

Для автоматической регистрации дохода самозанятого PayAnyWay должен получить номенклатуру заказа. В `POST /payments` можно передать позиции в `payment_info.items`; если позиции не переданы, webhook сформирует одну позицию из описания и суммы платежа.

```json
"items": [
  {
    "name": "Тестовая услуга интернет-магазина",
    "price": 1500,
    "quantity": 1,
    "vat_tag": "1105",
    "payment_method": "full_payment",
    "payment_object": "service",
    "id_internal": "service_1"
  }
]
```

Если PayAnyWay требует систему налогообложения, ее можно передать в XML-ответе через переменную:

```powershell
$env:PAYANYWAY_SNO="..."
```

По умолчанию для СБП адаптер добавляет `paymentSystem.unitId=sbpc2b`, для карты - `paymentSystem.unitId=card`. Значение можно переопределить:

```powershell
$env:PAYANYWAY_PAYMENT_UNIT_ID="sbpc2b"
```

Для тестового платежа через маршрутизатор можно направить СБП в PayAnyWay:

```powershell
pgrouteadd "merchant_12345" "СБП" payanyway 0 PAYANYWAY
```

После успешной тестовой оплаты PayAnyWay отправляет уведомление на `/webhooks/payanyway`; шлюз проверяет `MNT_SIGNATURE`, обновляет платеж до `CAPTURED` и отвечает XML-документом `MNT_RESPONSE` с `MNT_RESULT_CODE=200`, `MNT_SIGNATURE` и атрибутами `INVENTORY`/`CLIENT`.

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

Для демонстрации маркировки цифровых рублей и простого смарт-контракта интернет-магазин может передать категорию товара и параметры цифрового рубля:

```json
{
  "items": [
    {
      "name": "Учебник по программированию",
      "price": 1500,
      "quantity": 1,
      "category": "education"
    }
  ],
  "digital_ruble_data": {
    "smart_contract_id": "SC_MARKED_MONEY_V1",
    "require_marked_money": true
  }
}
```

Эмулятор сопоставляет категорию с маркировкой денег:

```text
education -> EDUCATION
medicine  -> HEALTHCARE
food      -> SOCIAL_FOOD
transport -> TRANSPORT
general   -> GENERAL
```

Создание платежа цифровым рублем всегда возвращает QR и статус ожидания:

```text
current_status = PENDING
provider_status = qr_issued
```

Ответ содержит поля QR/банка-участника:

```json
{
  "payment_system": "DIGITAL_RUBLE",
  "qr_id": "drqr_...",
  "qr_payload": "drub://...",
  "qr_image_data_uri": "data:image/png;base64,...",
  "qr_expires_at": "...",
  "participant_bank": "BANK_PARTNER_1",
  "schema_version": "drub.v1",
  "settlement_hint": "RUB + DIGITAL_RUBLE; settlement through participant bank emulator",
  "money_mark": "EDUCATION",
  "smart_contract_id": "SC_MARKED_MONEY_V1",
  "smart_contract_result": "PENDING",
  "platform_transport": "SOAP/XML sandbox"
}
```

`qr_payload` — строковая полезная нагрузка QR. `qr_image_data_uri` — PNG-изображение QR-кода в формате data URI, чтобы его можно было сразу показать в Postman Visualizer или на стороне тестового клиента.

Подтверждение оплаты эмулируется отдельным sandbox endpoint-ом:

```text
POST /sandbox/digital-ruble/scan
```

Пример:

```json
{
  "merchant_id": "merchant_12345",
  "payment_id": "pay_...",
  "qr_id": "drqr_...",
  "payer_wallet_id": "dr_wallet_123",
  "result": "captured"
}
```

Поддерживаемые sandbox-результаты:

```text
captured -> CAPTURED
declined -> DECLINED
failed   -> FAILED
expired  -> CANCELLED
```

Если QR уже истёк, попытка `captured` завершится `CANCELLED` с ошибкой `DIGITAL_RUBLE_QR_EXPIRED`.

При `captured` scan endpoint дополнительно формирует SOAP/XML-сообщение `C2BPaymentCheck` и передает его в эмулятор платформы цифрового рубля. Эмулятор проверяет, хватает ли в кошельке покупателя цифровых рублей с нужной маркировкой. Демо-кошельки:

```text
dr_wallet_123                 -> хватает EDUCATION/HEALTHCARE/SOCIAL_FOOD/TRANSPORT/GENERAL
dr_wallet_no_mark             -> есть только GENERAL, платеж education будет DECLINED
dr_wallet_insufficient_marked -> EDUCATION есть, но меньше суммы платежа
dr_wallet_healthcare          -> хватает HEALTHCARE
```

Если маркировка не подходит или маркированного остатка недостаточно, платеж получает `DECLINED`, `provider_status=smart_contract_rejected`, а ошибка будет `DIGITAL_RUBLE_MARK_RESTRICTION_FAILED`.

SOAP-эмулятор платформы можно вызвать напрямую:

```text
POST /sandbox/digital-ruble/soap
Content-Type: text/xml
```

Пример XML:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<Envelope xmlns="http://schemas.xmlsoap.org/soap/envelope/">
  <Body>
    <BusinessEnvelope>
      <MessageID>msg_demo</MessageID>
      <MessageType>C2BPaymentCheck</MessageType>
      <Sender>PAYMENT_GATEWAY</Sender>
      <Receiver>DIGITAL_RUBLE_PLATFORM_EMULATOR</Receiver>
      <SigContainer>
        <SignatureType>HMAC-SHA256-EMULATION</SignatureType>
        <SignatureValue>demo</SignatureValue>
      </SigContainer>
      <Object>
        <PaymentID>pay_demo</PaymentID>
        <MerchantID>merchant_12345</MerchantID>
        <WalletID>dr_wallet_123</WalletID>
        <Amount>1500.00</Amount>
        <Currency>RUB</Currency>
        <Category>education</Category>
        <RequiredMoneyMark>EDUCATION</RequiredMoneyMark>
        <SmartContractID>SC_MARKED_MONEY_V1</SmartContractID>
      </Object>
    </BusinessEnvelope>
  </Body>
</Envelope>
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
network_access_denied
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
$env:TRUSTED_PROXY_CIDRS="127.0.0.1/32"
```

Если задан `TRUSTED_PROXY_CIDRS`, шлюз доверяет `X-Forwarded-Proto`, `X-Forwarded-For`, `X-Real-IP` и `Forwarded` только запросам от этих proxy. В `APP_ENV=production` при `TRUST_PROXY_HEADERS=true` значение `TRUSTED_PROXY_CIDRS` обязательно.

При `REQUIRE_HTTPS=true` значения `PAYMENT_RETURN_URL` и `MERCHANT_WEBHOOK_URL` должны начинаться с `https://`.

Шлюз добавляет security headers:

```text
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: no-referrer
Cache-Control: no-store
Strict-Transport-Security: max-age=31536000; includeSubDomains
```

## Network Security Controls

Для частичной реализации PCI DSS Requirement 1 шлюз поддерживает IP/CIDR allowlist на уровне приложения.

Переменные:

```powershell
$env:TRUSTED_PROXY_CIDRS="127.0.0.1/32,10.0.0.0/8"
$env:MERCHANT_ALLOWED_CIDRS="203.0.113.10/32"
$env:ADMIN_ALLOWED_CIDRS="198.51.100.0/24"
$env:WEBHOOK_ALLOWED_CIDRS="203.0.113.0/24"
```

Смысл:

```text
TRUSTED_PROXY_CIDRS   -> от каких proxy можно принимать X-Forwarded-For/X-Real-IP/Forwarded
MERCHANT_ALLOWED_CIDRS -> откуда обычные merchant-ключи могут создавать платежи и возвраты
ADMIN_ALLOWED_CIDRS   -> откуда admin/auditor могут читать или управлять данными
WEBHOOK_ALLOWED_CIDRS -> откуда принимаются webhook-и внешних платежных систем
```

Если allowlist-переменная пустая, соответствующий класс запросов не ограничивается по IP. Это оставлено для локальной разработки и Postman. В production лучше задавать allowlist-и явно.

При нарушении правила шлюз возвращает:

```json
{
  "code": "NETWORK_ACCESS_DENIED",
  "message": "request source IP is not allowed"
}
```

Отказ логируется событием `network_access_denied`.

Это не заменяет firewall/reverse proxy: PostgreSQL и Vault всё равно должны быть закрыты от интернета и доступны только внутреннему контуру шлюза.

## LocalTunnel

Если для демонстрации используется LocalTunnel, при смене URL обнови:

```text
PUBLIC_URL в run.local.ps1
PAYMENT_RETURN_URL
MERCHANT_WEBHOOK_URL
YooKassa webhook URL: /webhooks/yookassa
Robokassa Result URL: /webhooks/robokassa
PayAnyWay Pay URL: /webhooks/payanyway
Postman base_url
```

LocalTunnel подходит для демонстрации webhook-ов, но production-контур должен использовать контролируемый TLS/reverse proxy.

## PCI DSS

Проект не является PCI DSS certified. Реализованы учебные технические меры, которые приближают шлюз к отдельным требованиям PCI DSS v4.0.1:

```text
Requirement 1  -> IP/CIDR allowlist, trusted proxy CIDR, webhook source restrictions, network_access_denied logs
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
сетевые allowlist-и в приложении не заменяют firewall, reverse proxy и сегментацию сети;
нужны external ASV scans, penetration testing, IDS/FIM, SIEM и организационные процессы;
старые записи БД, созданные до маскирования, нужно очищать отдельной миграцией.
```

## Test commands

Для третьей главы диплома тесты разделены на четыре отдельные категории. Каждая категория запускается своей командой из корня проекта.

### Unit tests

Модульные тесты проверяют отдельные подсистемы без запуска всего платежного шлюза:

```powershell
.\payment_gateway_tools\test-unit.ps1
```

Покрываются DTO, валидатор, антифрод, адаптеры, логирование, merchant auth и Vault/secrets.

### Component integration tests

Компонентные интеграционные тесты проверяют взаимодействие нескольких модулей внутри приложения:

```powershell
.\payment_gateway_tools\test-integration.ps1
```

Покрываются orchestrator, webhook-и, отчеты и in-memory хранилище. Эти тесты используют тестовые заглушки и не требуют реальных YooKassa, Robokassa, PayAnyWay или PostgreSQL.

### Security tests

Тесты безопасности запускают security-focused Go-тесты, статический анализ, проверку зависимостей и поиск секретов в tracked-файлах:

```powershell
.\payment_gateway_tools\test-security.ps1 -InstallMissingTools -RequireGovulncheck
```

Проверяются:

```text
security-focused go tests;
go vet ./...;
govulncheck ./...;
tracked file secret scan.
```

### Performance tests

Производительность проверяется Go benchmark-тестами:

```powershell
.\payment_gateway_tools\test-performance.ps1
```

Для быстрого локального запуска можно уменьшить время измерения:

```powershell
.\payment_gateway_tools\test-performance.ps1 -Benchtime 1s -Count 1
```

Benchmark-и покрывают orchestrator, idempotency hit, валидатор, антифрод и построение отчетов.

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

## Performance benchmarks

Производительность проверяется Go benchmark-тестами. Они не запускаются при обычном `go test ./...`, а выполняются отдельной командой:

```powershell
go test ./... -run "^$" -bench . -benchmem
```

Удобный запуск через PowerShell:

```powershell
.\payment_gateway_tools\test-performance.ps1
```

Запустить только benchmark-и оркестратора:

```powershell
.\payment_gateway_tools\test-performance.ps1 -Package ./internal/orchestrator/simple
```

Запустить конкретный benchmark:

```powershell
.\payment_gateway_tools\test-performance.ps1 -Benchmark BenchmarkCreatePaymentFullFlowSimulated
```

Сейчас benchmark-и покрывают:

```text
orchestrator full payment flow через in-memory store и simulated adapter;
idempotency hit для повторного платежного запроса;
валидацию банковской карты и цифрового рубля;
rule-based antifraud;
построение отчета по 1000 in-memory транзакций.
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
pgrouteadd "merchant_12345" "СБП" robokassa 1 ROBOKASSA
pgrouteadd "merchant_12345" "СБП" payanyway 0 PAYANYWAY
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

Если webhook-и мерчанту не доставляются, проверь:

```powershell
echo $env:MERCHANT_WEBHOOK_URL
```

Если YooKassa webhook не доходит, проверь URL в кабинете YooKassa:

```text
https://<public-url>/webhooks/yookassa
```

Если GitHub блокирует push, проверь, что реальные секреты не попали в отслеживаемые файлы:

```text
YOOKASSA_SECRET_KEY
ROBOKASSA_TEST_PASSWORD1
ROBOKASSA_TEST_PASSWORD2
PAYANYWAY_INTEGRITY_CODE
DATABASE_URL с паролем
```

Реальные ключи должны храниться только в `payment_gateway_tools/run.local.ps1` или другом локальном файле из `.gitignore`.
