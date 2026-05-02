# Модуль токенизации

## Файлы для замены

- `cmd/payment-gateway/main.go`
- `internal/orchestrator/simple/orchestrator.go`
- `internal/subsystems/tokenizer/dummy_tokenizer.go`
- `internal/subsystems/tokenizer/postgres_tokenizer.go`
- `payment_gateway_tools/pg-tools.ps1` — необязательно, только если хочешь команды `pgtokens` и `pgtokenpay`

## Что появится в PostgreSQL

При запуске сервера с `DATABASE_URL` автоматически создаётся таблица `payment_tokens`.

Она хранит:

- `merchant_id`
- `payment_id`
- `idempotency_key`
- `token_hash`
- `token_preview`
- `payment_method`
- `masked_value`
- `expires_at`
- `created_at`
- `revoked_at`

Полный номер карты, CVV и полный токен в таблицу не сохраняются.

## Проверка

После оплаты выполни:

```sql
SELECT id, merchant_id, payment_id, token_preview, payment_method, masked_value, expires_at, created_at
FROM payment_tokens
ORDER BY id DESC
LIMIT 20;
```

Или через PowerShell после подключения `pg-tools.ps1`:

```powershell
pgtokens 20
pgtokenpay pay_...
```
