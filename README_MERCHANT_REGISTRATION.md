# Служебная регистрация мерчантов

Этот комплект добавляет удобную служебную команду для подключения новых интернет-магазинов к платежному шлюзу.

## Файлы

```text
payment_gateway_tools/create-merchant.ps1
payment_gateway_tools/merchant-admin-tools.ps1
```

## Что делает create-merchant.ps1

Скрипт:

1. создаёт или дополняет таблицу `merchants`;
2. генерирует `api_key`;
3. генерирует `secret_key`;
4. считает SHA-256 hash от `api_key`;
5. при включённом `SECRET_PROTECTOR=vault_transit` шифрует `secret_key` через HashiCorp Vault;
6. записывает мерчанта в PostgreSQL;
7. выводит данные, которые нужно передать интернет-магазину.

## Запуск

В корне проекта:

```powershell
. .\payment_gateway_tools\pg-tools.ps1

.\payment_gateway_tools\create-merchant.ps1 `
  -MerchantID "merchant_books" `
  -Name "Book Shop" `
  -WebhookURL "https://books-shop.example.com/payment-webhook"
```

Если нужно перевыпустить ключи существующего мерчанта:

```powershell
.\payment_gateway_tools\create-merchant.ps1 `
  -MerchantID "merchant_books" `
  -Name "Book Shop" `
  -WebhookURL "https://books-shop.example.com/payment-webhook" `
  -RotateKeys
```

## Что передать интернет-магазину

```text
merchant_id
api_key
secret_key
webhook_url
```

`secret_key` нельзя передавать в запросах. Он используется магазином только для формирования HMAC-подписи.

Если включён Vault, в БД хранится не открытый `secret_key`, а значение вида:

```text
vault:v1:<ciphertext>
```

Подробная настройка описана в [README_VAULT_KEY_MANAGEMENT.md](README_VAULT_KEY_MANAGEMENT.md).

## Проверка

```powershell
. .\payment_gateway_tools\merchant-admin-tools.ps1
pgmerchants
pgmerchant merchant_books
```

## Отключить или включить мерчанта

```powershell
pgmerchantdisable merchant_books
pgmerchantenable merchant_books
```

## Как подключается второй магазин

Второй магазин получает свои уникальные учётные данные и использует то же API платежного шлюза:

```text
POST /payments
GET /payments/{payment_id}?merchant_id=...
```

Но каждый запрос должен быть подписан своими ключами.
