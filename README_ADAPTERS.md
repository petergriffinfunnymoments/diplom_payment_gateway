# Адаптеры платежных систем

В проект добавлена расширяемая фабрика адаптеров `internal/subsystems/adapter/Factory`.
Оркестратор получает адаптер из фабрики по ключу маршрута и настройкам окружения.

## Поддерживаемые провайдеры

- `simulated` — локальный эмуляционный адаптер, возвращает `CAPTURED`.
- `dummy` — совместимый alias для старых маршрутов, использует тот же эмуляционный адаптер.
- `yookassa` — sandbox-ready адаптер ЮKassa. Создаёт платеж через `POST https://api.yookassa.ru/v3/payments` и возвращает ссылку `confirmation_url`.
- `tbank` — sandbox-ready адаптер T-Банк эквайринга. Инициирует платеж через `POST https://securepay.tinkoff.ru/v2/Init` и возвращает `PaymentURL`.

## Как выбрать провайдера

Общий провайдер для всех способов оплаты:

```powershell
$env:PAYMENT_PROVIDER="yookassa"
```

Или отдельно по способам оплаты:

```powershell
$env:CARD_PAYMENT_PROVIDER="yookassa"
$env:SBP_PAYMENT_PROVIDER="tbank"
$env:WALLET_PAYMENT_PROVIDER="simulated"
```

Если провайдер не настроен или его credentials отсутствуют, фабрика использует совместимый fallback `dummy`.

## ЮKassa

Для ЮKassa нужны оба значения:

```powershell
$env:YOOKASSA_SHOP_ID="идентификатор_магазина"
$env:YOOKASSA_SECRET_KEY="секретный_ключ"
$env:PAYMENT_RETURN_URL="http://localhost:8080"
$env:CARD_PAYMENT_PROVIDER="yookassa"
```

После создания платежа ответ шлюза будет иметь статус `PENDING`, потому что пользователь должен перейти по `payment_url` и подтвердить оплату на стороне ЮKassa.

## T-Банк

Минимальная настройка через пароль терминала:

```powershell
$env:TBANK_TERMINAL_KEY="terminal_key"
$env:TBANK_PASSWORD="password"
$env:PAYMENT_RETURN_URL="http://localhost:8080"
$env:CARD_PAYMENT_PROVIDER="tbank"
```

Если используешь Bearer API token:

```powershell
$env:TBANK_TERMINAL_KEY="terminal_key"
$env:TBANK_API_TOKEN="api_token"
$env:CARD_PAYMENT_PROVIDER="tbank"
```

Опционально:

```powershell
$env:TBANK_SUCCESS_URL="http://localhost:8080/success"
$env:TBANK_FAIL_URL="http://localhost:8080/fail"
$env:TBANK_NOTIFICATION_URL="https://example.com/tbank/webhook"
```

## Что изменилось в ответе шлюза

В `transaction_details` добавлены поля:

```json
{
  "provider_status": "pending",
  "payment_url": "https://..."
}
```

Для redirect-провайдеров нормальный ответ — `PENDING`, а не `CAPTURED`. Финальный статус нужно будет получать через webhook или отдельный метод проверки статуса платежа.
