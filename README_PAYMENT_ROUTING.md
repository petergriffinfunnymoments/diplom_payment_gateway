# Маршрутизация платежей по адаптерам

Теперь платежный шлюз может сам выбирать адаптер платежной системы для каждой транзакции.
Выбор выполняет компонент `PaymentRouter` внутри оркестратора платежей.

## Таблица маршрутов

При запуске с `DATABASE_URL` автоматически создается таблица:

```sql
merchant_payment_routes
```

Основные поля:

- `merchant_id` — мерчант/интернет-магазин;
- `payment_method` — способ оплаты: `Банковская карта`, `СБП`, `Цифровой кошелек`;
- `provider` — ключ адаптера: `yookassa`, `stripe`, `simulated`, `dummy`, `tbank`;
- `payment_system` — имя внешней платежной системы в ответе шлюза;
- `priority` — приоритет маршрута, меньшее значение выбирается первым;
- `active` — включен ли маршрут.

## Как подключить команды

```powershell
. .\payment_gateway_tools\pg-tools.ps1
. .\payment_gateway_tools\payment-route-tools.ps1
```

## Примеры маршрутов

Отправлять карточные платежи `merchant_12345` в ЮKassa:

```powershell
pgrouteadd "merchant_12345" "Банковская карта" yookassa 1 YOOKASSA
```

Отправлять карточные платежи другого мерчанта в Stripe:

```powershell
pgrouteadd "merchant_books" "Банковская карта" stripe 1 STRIPE
```

Оставить цифровой кошелек на локальном эмуляционном адаптере:

```powershell
pgrouteadd "merchant_12345" "Цифровой кошелек" simulated 1 SIMULATED
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

## Как это работает

1. Интернет-магазин отправляет платеж в `POST /payments`.
2. Оркестратор вызывает маршрутизатор платежей.
3. Маршрутизатор ищет активное правило в `merchant_payment_routes` по `merchant_id` и `payment_method`.
4. Если правило найдено, он возвращает выбранный `provider`.
5. Фабрика адаптеров возвращает конкретный объект адаптера: `YooKassaAdapter`, `StripeAdapter`, `SimulatedAdapter` и т.д.
6. Если правила нет, используется fallback-логика: переменные окружения `CARD_PAYMENT_PROVIDER`, `SBP_PAYMENT_PROVIDER`, `WALLET_PAYMENT_PROVIDER` или совместимый provider `dummy`.

## Важное замечание

Переменные окружения теперь не являются основным способом выбора адаптера.
Они используются только как fallback. Основной выбор должен задаваться в таблице `merchant_payment_routes`.
