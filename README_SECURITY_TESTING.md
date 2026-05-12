# Security testing и PCI DSS Requirement 11

Проект содержит базовый набор регулярных проверок безопасности. Это не делает шлюз PCI DSS certified, но частично закрывает идею Requirement 11: регулярно тестировать безопасность систем и сетей.

## Локальный запуск

Из корня проекта:

```powershell
.\payment_gateway_tools\security-check.ps1
```

Для строгого прогона с `govulncheck` нужен актуальный Go toolchain. На момент настройки workflow используется Go `1.25.10`, потому что более старые patch-версии `1.25.x` содержат уязвимости стандартной библиотеки.

Если `govulncheck` ещё не установлен:

```powershell
.\payment_gateway_tools\security-check.ps1 -InstallMissingTools
```

Строгий режим, который используется в CI:

```powershell
.\payment_gateway_tools\security-check.ps1 -InstallMissingTools -RequireGovulncheck
```

## Что проверяется

Скрипт выполняет:

```text
go test ./...
go vet ./...
govulncheck ./...
tracked file secret scan
```

Secret scan проверяет отслеживаемые Git-файлы на типовые секреты:

```text
Stripe sk_test/sk_live
Stripe whsec
gateway pg_sk_test/pg_sk_live
PostgreSQL URL с паролем
private key block
```

Локальный `payment_gateway_tools/run.local.ps1` не сканируется, потому что он находится в `.gitignore` и предназначен для настоящих ключей.

## GitHub Actions

Workflow находится в:

```text
.github/workflows/security-checks.yml
```

Workflow запускает проверки на Go `1.25.10`.

Он запускается на:

```text
push
pull_request
workflow_dispatch
```

Если тесты, `go vet`, `govulncheck` или secret scan находят проблему, workflow завершается ошибкой.

## Что ещё требуется для полного PCI DSS 11

Для промышленного соответствия нужны дополнительные меры:

```text
external ASV vulnerability scans;
internal vulnerability scans;
penetration testing;
segmentation testing, если CDE сегментируется;
IDS/IPS или другой network intrusion detection;
file integrity monitoring;
процесс исправления найденных уязвимостей.
```

В дипломе эту доработку можно описывать как частичную реализацию Requirement 11 на уровне разработки и CI/CD.
