# PCI DSS hardening notes

Проект не становится PCI DSS certified только за счёт этих изменений, но код стал ближе к PCI DSS v4.0.1 design baseline:

- `PaymentResponse` больше не возвращает и не сохраняет полный PAN, CVV и полный внутренний token.
- `customer_data.card_number` в response и `payment_transactions.payload_json` хранится только как маска вида `411111******1111`.
- `customer_data.CVV_code` удаляется из response и сохранённого payload.
- После успешной валидации `CVV_code` удаляется из `CreatePaymentRequest`, поэтому антифрод, маршрутизатор, токенизатор, адаптеры, callback, БД и уведомления получают уже CVV-free DTO.
- `transaction_details.token` заменён на `transaction_details.token_preview`.
- `TransactionStore.Save` дополнительно санитизирует `payload_json`, чтобы случайный полный response не попал в бизнес-БД.
- `TransactionStore.Save` также удаляет CVV-поля из произвольного JSON payload, если туда случайно передан не `PaymentResponse`.
- Webhook-обработчики YooKassa/Stripe санитизируют response перед сохранением и merchant notification.
- Логгеры применяют redaction для PAN/CVV/похожих секретов перед записью в console/PostgreSQL.
- `merchants.secret_key` может храниться в PostgreSQL в зашифрованном виде через HashiCorp Vault Transit (`SECRET_PROTECTOR=vault_transit`).
- Добавлена ролевая модель `merchant/admin/auditor`: обычный мерчант получает доступ только к своим платежам, возвратам и отчётам; `admin` может работать с данными любого мерчанта; `auditor` имеет read-only доступ к статусам, возвратам и отчётам.
- Неизвестные роли отклоняются при аутентификации, а отказы доступа логируются событием `authorization_failed`.
- Добавлены транспортные меры для PCI DSS Requirement 4: опциональный запуск через `ListenAndServeTLS`, режим `REQUIRE_HTTPS`, поддержка `X-Forwarded-Proto` при `TRUST_PROXY_HEADERS=true`, security headers и запрет `http://` в `PAYMENT_RETURN_URL`/`MERCHANT_WEBHOOK_URL` при включённом HTTPS enforcement.
- Добавлены регулярные security checks для PCI DSS Requirement 11: `go test`, `go vet`, `govulncheck`, проверка секретов в отслеживаемых файлах и GitHub Actions workflow.

Ограничения:

- Backend всё ещё принимает `card_number` и `CVV_code` во входном JSON для учебного сценария, поэтому card-data path остаётся в PCI scope. CVV используется только для валидации и затем очищается из внутренних DTO.
- Для реального сокращения scope нужно переходить на hosted checkout/hosted fields/iframe внешнего PCI DSS-compliant провайдера или выносить token vault в отдельный CDE-сегмент.
- Старые записи в `payment_transactions.payload_json`, созданные до этой правки, нужно очистить отдельной миграцией, если в них уже есть PAN/CVV.
- LocalTunnel подходит для демонстрации webhook-ов, но production-контур должен использовать контролируемый TLS/reverse proxy. Если TLS завершается на proxy, включай `REQUIRE_HTTPS=true` и `TRUST_PROXY_HEADERS=true`.
- Локальные security checks и CI не заменяют external ASV scans, internal vulnerability scans, penetration testing, IDS/FIM, SIEM и организационные PCI DSS-контроли.

Проверка:

```powershell
go test ./...
```

Расширенная security-проверка:

```powershell
.\payment_gateway_tools\security-check.ps1 -InstallMissingTools
```

Smoke-test по БД после нового платежа:

```sql
SELECT payload_json::text
FROM payment_transactions
ORDER BY updated_at DESC
LIMIT 5;
```

В результате не должно быть полного `card_number`, `CVV_code` и полного `tok_...`.
