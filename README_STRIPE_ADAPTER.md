# Stripe Adapter

Добавлен адаптер Stripe Checkout для платежного шлюза.

## Что делает адаптер

`StripeAdapter` создаёт Stripe Checkout Session через API Stripe и возвращает в ответе шлюза:

- `current_status = PENDING`
- `transaction_details.payment_system = STRIPE`
- `transaction_details.provider_status = unpaid/open`
- `transaction_details.payment_url = ссылка на Stripe Checkout`

Пользователь переходит по `payment_url`, вводит тестовую карту на странице Stripe, после чего Stripe отправляет webhook в шлюз на `/webhooks/stripe`.

## Переменные окружения

Минимально:

```powershell
$env:STRIPE_SECRET_KEY="sk_test_..."
$env:CARD_PAYMENT_PROVIDER="stripe"
$env:PAYMENT_RETURN_URL="https://YOUR_TUNNEL_URL"
```

Для webhook:

```powershell
$env:STRIPE_WEBHOOK_SECRET="whsec_..."
```

Если Stripe не принимает валюту `RUB` в твоём тестовом аккаунте, можно включить тестовый override валюты:

```powershell
$env:STRIPE_CURRENCY_OVERRIDE="usd"
```

В этом случае шлюз всё равно принимает платежи в своей модели, но внешняя тестовая сессия Stripe создаётся в USD. Это удобно только для демонстрации адаптера.

## Webhook URL

Если используешь LocalTunnel или Cloudflare Tunnel, в Stripe Dashboard нужно указать:

```text
https://YOUR_TUNNEL_URL/webhooks/stripe
```

Рекомендуемые события:

```text
checkout.session.completed
checkout.session.expired
checkout.session.async_payment_failed
```

## Проверка

1. Запусти шлюз с `CARD_PAYMENT_PROVIDER=stripe`.
2. Создай платеж банковской картой.
3. В ответе должна появиться ссылка Stripe Checkout.
4. Перейди по ссылке и введи тестовую карту Stripe.
5. После webhook статус в `payment_transactions` должен измениться на `CAPTURED` или `DECLINED`.

Полезные команды:

```powershell
pgtx 10
pglogs 30
pglastpaylogs
```

## Тестовые карты Stripe

Для успешного сценария обычно используется тестовая карта:

```text
4242 4242 4242 4242
```

Срок действия — любая будущая дата, CVC — любые 3 цифры.
