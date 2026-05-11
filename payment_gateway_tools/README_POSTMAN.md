# Postman setup

Эти файлы позволяют проверять основные операции платежного шлюза без фронтенда:

- `postman_environment.json`
- `postman_collection.json`

## Импорт

1. Открой Postman.
2. Нажми `Import`.
3. Импортируй оба файла из папки `payment_gateway_tools`.
4. Справа сверху выбери environment `Payment Gateway Local`.
5. Проверь переменные environment:
   - `base_url` — обычно `http://localhost:8080`;
   - `merchant_id`;
   - `api_key`;
   - `secret_key`.

Значения `api_key` и `secret_key` должны совпадать с теми, которые использует шлюз.

## Запуск проекта

```powershell
. .\payment_gateway_tools\run.local.ps1
go run ./cmd/payment-gateway
```

Проверка доступности:

```text
GET {{base_url}}/health
```

## Что есть в коллекции

- `GET /health`
- `GET /payments/__auth_check__ - Check current credentials`
- `POST /payments - Card MIR`
- `POST /payments - Card Visa`
- `POST /payments - Digital Ruble`
- `GET /payments/{payment_id}`
- `POST /refunds/full/create`
- `POST /refunds/partial/create`
- `GET /refunds/status`
- `GET /refunds/search`
- `GET /reports/transactions - Merchant statistics`
- `POST /merchant/webhook - demo`

Коллекция автоматически авторизует мерчанта: функция `authorizeMerchantRequest()` в pre-request script считает HMAC-подпись и добавляет заголовки:

- `X-Merchant-ID`
- `X-API-Key`
- `X-Timestamp`
- `X-Signature`

Запрос `Merchant Auth / GET /payments/__auth_check__ - Check current credentials` проверяет текущие credentials.
Правильный результат для этого запроса — `404 PAYMENT_NOT_FOUND`: это значит, что авторизация прошла, а тестовый платеж ожидаемо не найден.
Если вернулся `401 AUTHENTICATION_ERROR`, проверь `merchant_id`, `api_key` и `secret_key` в environment.

После `POST /payments` коллекция сохраняет `payment_id`.
После `POST /refunds/...` коллекция сохраняет `refund_id`.

## Отчеты и статистика

Запрос `Reports / GET /reports/transactions - Merchant statistics` формирует отчет только по текущему `merchant_id`.

Доступные фильтры задаются переменными environment:

- `report_date_from` — начало периода, формат `YYYY-MM-DD` или RFC3339;
- `report_date_to` — конец периода, формат `YYYY-MM-DD` или RFC3339;
- `report_status` — например `CAPTURED`, `PENDING`, `DECLINED`, `FAILED`;
- `report_payment_system` — например `YOOKASSA`, `STRIPE`, `DIGITAL_RUBLE`, `DUMMY`;
- `report_payment_method` — например `Банковская карта` или `Цифровой рубль`;
- `report_limit` — сколько последних транзакций вернуть, максимум `500`.

Если фильтр оставить пустым, он не применяется. Во вкладке `Visualize` Postman показывает:

- карточки KPI: количество платежей, общая сумма, успешная сумма, средний чек;
- donut-диаграмму долей по статусам;
- гистограмму активности по дням;
- воронку статусов платежей;
- горизонтальные бары по сумме платежных систем;
- распределение по статусам, платежным системам и способам оплаты;
- таблицу последних транзакций.

## Визуализация ответов

В коллекции есть общий `Tests`-скрипт с Postman Visualizer. После выполнения `POST /payments` или `GET /payments/{payment_id}` открой вкладку `Visualize`.

Визуализация показывает:

- `Payment Summary`;
- `Customer Data`;
- `Transaction Details`;
- потоковую диаграмму архитектуры платежного шлюза из 9 внутренних подсистем, подписями обменов и стрелками движения данных.

Цвета на схеме:

- зеленый — модуль выполнил свою часть и транзакция пошла дальше;
- красный — модуль остановил транзакцию, например `VALIDATION_ERROR`, `ANTIFRAUD_DECLINED` или ошибка адаптера;
- серый — модуль еще не был достигнут или по ответу нельзя надежно определить выполнение.

Скрипт работает для всех способов оплаты, потому что читает общий `PaymentResponse`: банковская карта, СБП, цифровой кошелек и цифровой рубль.

## Важный порядок проверки

1. `GET /health`.
2. `Merchant Auth / GET /payments/__auth_check__ - Check current credentials`.
3. `POST /payments - Card MIR`, `POST /payments - Card Visa` или `POST /payments - Digital Ruble`.
4. Перейди по `payment_url`, если провайдер вернул redirect-ссылку.
5. Дождись webhook от провайдера или проверь статус через `GET /payments/{payment_id}`.
6. Когда платеж стал `CAPTURED`, выполни `POST /refunds/full/create` или `POST /refunds/partial/create`.
7. Проверь возврат через `GET /refunds/status` или `GET /refunds/search`.
8. Выполни `Reports / GET /reports/transactions - Merchant statistics`, чтобы увидеть статистику по текущему мерчанту.

Для локальной проверки без реального провайдера используй маршрут на provider `simulated` или совместимый `dummy`.

Для цифрового рубля коллекция использует эмуляционный provider `digital_ruble`.
Тестовые значения:

- `dr_wallet_123` → `CAPTURED`;
- `dr_wallet_declined` → `DECLINED`;
- `dr_wallet_error` → `FAILED`;
- `dr_wallet_pending` → `PENDING`.
