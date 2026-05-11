# HashiCorp Vault для защиты секретов мерчантов

Интеграция добавляет защиту `merchants.secret_key`: API-ключ мерчанта хранится как SHA-256 hash, а HMAC `secret_key` может храниться в PostgreSQL как ciphertext Vault:

```text
vault:v1:<ciphertext>
```

Шлюз расшифровывает значение только в памяти при проверке HMAC-подписи мерчанта.

## Про Key Management и Transit

Документ HashiCorp Vault Key Management описывает secrets engine для управления жизненным циклом ключей и их распределения во внешние KMS-провайдеры. Для непосредственного шифрования данных приложения Vault рекомендует использовать Transit secrets engine: приложение отправляет plaintext в Vault `/transit/encrypt`, получает ciphertext и сохраняет его в БД.

В проекте используется именно такой data-path:

```text
payment-gateway -> Vault Transit encrypt/decrypt -> PostgreSQL хранит только vault:v1:...
```

При наличии Vault Enterprise Key Management можно использовать его как production-слой управления ключами, а Transit оставить интерфейсом шифрования для приложения.

## Быстрый локальный запуск Vault

Для дипломного стенда можно поднять dev Vault:

```powershell
docker run --rm -p 8200:8200 `
  --cap-add=IPC_LOCK `
  -e VAULT_DEV_ROOT_TOKEN_ID=root `
  hashicorp/vault:latest
```

В другом терминале:

```powershell
$env:VAULT_ADDR="http://127.0.0.1:8200"
$env:VAULT_TOKEN="root"

vault secrets enable transit
vault write -f transit/keys/payment-gateway-merchant-secrets
```

## Переменные окружения шлюза

В `payment_gateway_tools/run.local.ps1`:

```powershell
$env:SECRET_PROTECTOR="vault_transit"
$env:VAULT_ADDR="http://127.0.0.1:8200"
$env:VAULT_TOKEN="root"
$env:VAULT_TRANSIT_MOUNT="transit"
$env:VAULT_TRANSIT_KEY="payment-gateway-merchant-secrets"

# Для Vault Enterprise/HCP при необходимости:
# $env:VAULT_NAMESPACE="admin"
```

В production вместо `VAULT_TOKEN` лучше использовать `VAULT_TOKEN_FILE`, AppRole, Kubernetes auth или другой механизм выдачи короткоживущего токена.

## Минимальная Vault policy

Для шлюза достаточно разрешить encrypt/decrypt только по одному ключу:

```hcl
path "transit/encrypt/payment-gateway-merchant-secrets" {
  capabilities = ["update"]
}

path "transit/decrypt/payment-gateway-merchant-secrets" {
  capabilities = ["update"]
}
```

## Создание или ротация мерчанта

Если `SECRET_PROTECTOR=vault_transit`, скрипт создания мерчанта сам вызовет Vault `encrypt` и сохранит в БД ciphertext:

```powershell
.\payment_gateway_tools\create-merchant.ps1 `
  -MerchantID "merchant_12345" `
  -Name "Demo Shop" `
  -RotateKeys
```

Исходный `secret_key` будет показан один раз. Именно его нужно указать в Postman или передать мерчанту. В PostgreSQL будет сохранён `vault:v1:...`.

## Проверка

```sql
SELECT merchant_id, left(secret_key, 8) AS secret_prefix
FROM merchants;
```

Ожидаемый результат:

```text
vault:v1
```

После этого обычные подписанные запросы из Postman должны работать без изменений.

## Совместимость со старыми записями

Старые plaintext-значения остаются совместимыми: если `secret_key` не начинается с `vault:v`, шлюз использует его как обычный секрет. Для PCI DSS-подобного режима лучше перевыпустить ключи мерчантов через `-RotateKeys`.

Официальная документация:

- Vault Key Management secrets engine: https://developer.hashicorp.com/vault/docs/secrets/key-management
- Vault Transit secrets engine: https://developer.hashicorp.com/vault/docs/secrets/transit
- Vault Transit API: https://developer.hashicorp.com/vault/api-docs/secret/transit
