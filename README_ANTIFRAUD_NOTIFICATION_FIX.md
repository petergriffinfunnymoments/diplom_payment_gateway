# Исправление уведомлений при отказе антифрода

Проблема: при `ANTIFRAUD_DECLINED` оркестратор сохранял транзакцию и логи, но выходил через `failAndSave(...)`, где не вызывался сервис уведомлений.

Исправление: в `failAndSave(...)` добавлен вызов `o.notifications.Notify(ctx, resp)` для всех финальных ошибок внутри шлюза, включая отказ антифрода. Для отказа антифрода также заполняется `fraud_check_result = BLOCKED`.

## Проверка

1. Перезапустить шлюз с `MERCHANT_WEBHOOK_URL`.
2. На странице интернет-магазина ввести сумму `500000`.
3. Проверить таблицу уведомлений:

```sql
SELECT
    created_at,
    delivery_id,
    merchant_id,
    payment_id,
    event_type,
    status_code,
    success,
    error_message
FROM notification_deliveries
ORDER BY created_at DESC
LIMIT 20;
```

Ожидаемо: `event_type = payment.declined`, `success = true`, `status_code = 200`.
