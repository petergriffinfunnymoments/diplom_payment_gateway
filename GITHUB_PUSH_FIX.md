# Исправление ошибки GitHub Push Protection

GitHub блокировал push, потому что в `payment_gateway_tools/run.ps1` попал реальный Stripe Secret Key.

Что изменено:

- `payment_gateway_tools/run.ps1` больше не содержит реальных секретов;
- добавлен `payment_gateway_tools/run.local.example.ps1`;
- реальные ключи нужно хранить в `payment_gateway_tools/run.local.ps1`;
- `run.local.ps1`, `.env` и `*.local.ps1` добавлены в `.gitignore`;
- `payment_gateway_tools/pg-tools.ps1` больше не содержит реальный пароль PostgreSQL.

## Что нужно сделать локально

1. Создай локальный файл с секретами:

```powershell
Copy-Item .\payment_gateway_tools\run.local.example.ps1 .\payment_gateway_tools\run.local.ps1
notepad .\payment_gateway_tools\run.local.ps1
```

2. Вставь реальные значения только в `run.local.ps1`.

3. Проверь, что секретов больше нет в отслеживаемых файлах:

```powershell
Get-ChildItem -Recurse -File | Select-String "sk_test_"
Get-ChildItem -Recurse -File | Select-String "sk_live_"
Get-ChildItem -Recurse -File | Select-String "whsec_"
```

4. Если секрет был в последнем локальном коммите:

```powershell
git add .gitignore payment_gateway_tools/run.ps1 payment_gateway_tools/run.local.example.ps1 payment_gateway_tools/pg-tools.ps1 GITHUB_PUSH_FIX.md
git commit --amend --no-edit
git push origin main
```

5. Если GitHub всё ещё блокирует push, значит секрет остался в более раннем локальном коммите. Тогда посмотри последние коммиты:

```powershell
git log --oneline -5
```

Если секрет был в одном последнем коммите:

```powershell
git reset --soft HEAD~1
git add .
git commit -m "Sanitize local configuration files"
git push origin main
```

Если секрет был в двух последних коммитах, используй `HEAD~2`.

## Важно

Перевыпусти Stripe Secret Key в Stripe Dashboard. Даже если GitHub заблокировал push, ключ уже попал в локальную Git-историю.
