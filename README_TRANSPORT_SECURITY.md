# Transport security и PCI DSS Requirement 4

Шлюз поддерживает два режима защиты передачи данных.

## Локальная разработка

По умолчанию можно запускать как раньше:

```powershell
go run ./cmd/payment-gateway
```

Это оставляет `http://localhost:8080`, что удобно для локальной разработки, но не является production-режимом для передачи PAN/CVV.

## Прямой TLS в приложении

Если задать сертификат и ключ, сервер запустится через `ListenAndServeTLS`:

```powershell
$env:TLS_CERT_FILE="C:\path\gateway.crt"
$env:TLS_KEY_FILE="C:\path\gateway.key"
$env:REQUIRE_HTTPS="true"

go run ./cmd/payment-gateway
```

## Reverse proxy или LocalTunnel

Если HTTPS завершается перед приложением, включи:

```powershell
$env:REQUIRE_HTTPS="true"
$env:TRUST_PROXY_HEADERS="true"
```

Тогда шлюз принимает запрос как защищённый только если proxy передал:

```http
X-Forwarded-Proto: https
```

В production доверять этому заголовку можно только от контролируемого reverse proxy.

## Проверка URL

Когда `REQUIRE_HTTPS=true`, значения должны быть `https://`:

```powershell
$env:PAYMENT_RETURN_URL="https://example.com"
$env:MERCHANT_WEBHOOK_URL="https://example.com/merchant/webhook"
```

Если указать `http://`, приложение завершит запуск с ошибкой.

## Security headers

Шлюз добавляет:

```text
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: no-referrer
Cache-Control: no-store
Strict-Transport-Security: max-age=31536000; includeSubDomains
```

`Strict-Transport-Security` добавляется только для HTTPS-запросов.
