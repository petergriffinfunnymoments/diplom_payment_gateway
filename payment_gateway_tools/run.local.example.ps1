# Copy this file to payment_gateway_tools/run.local.ps1 and fill real values.
# run.local.ps1 is ignored by Git.

# Optional: protect merchant secret_key values in PostgreSQL through HashiCorp Vault Transit.
# $env:SECRET_PROTECTOR="vault_transit"
# $env:VAULT_ADDR="http://127.0.0.1:8200"
# $env:VAULT_TOKEN_FILE="$PSScriptRoot\vault_token.local.txt"
# $env:VAULT_TRANSIT_MOUNT="transit"
# $env:VAULT_TRANSIT_KEY="payment-gateway-merchant-secrets"
# $env:VAULT_NAMESPACE="<optional-enterprise-or-hcp-namespace>"

# Optional PCI DSS Requirement 4 transport controls.
# Local development can stay on http://localhost:8080.
# For direct TLS:
# $env:TLS_CERT_FILE="$PSScriptRoot\certs\gateway.local.crt"
# $env:TLS_KEY_FILE="$PSScriptRoot\certs\gateway.local.key"
#
# For production/reverse proxy/localtunnel-like HTTPS termination:
# $env:REQUIRE_HTTPS="true"
# $env:TRUST_PROXY_HEADERS="true"
# $env:TRUSTED_PROXY_CIDRS="127.0.0.1/32"
#
# When REQUIRE_HTTPS=true, PAYMENT_RETURN_URL and MERCHANT_WEBHOOK_URL must use https://.

# Optional PCI DSS Requirement 1 network security controls.
# Empty values mean "do not restrict this class of requests by IP" for local development.
# $env:MERCHANT_ALLOWED_CIDRS="203.0.113.10/32"
# $env:ADMIN_ALLOWED_CIDRS="198.51.100.0/24"
# $env:WEBHOOK_ALLOWED_CIDRS="203.0.113.0/24"

# Optional Robokassa test integration.
# Configure these after creating a Robokassa shop and test passwords.
# $env:ROBOKASSA_MERCHANT_LOGIN="<shop-merchant-login>"
# $env:ROBOKASSA_TEST_PASSWORD1="<test-password-1>"
# $env:ROBOKASSA_TEST_PASSWORD2="<test-password-2>"
# $env:ROBOKASSA_TEST_MODE="true"
# $env:ROBOKASSA_HASH_ALGORITHM="md5"
# Result URL in Robokassa technical settings:
# https://<public-url>/webhooks/robokassa

# Optional PayAnyWay test integration.
# $env:PAYANYWAY_MNT_ID="<business-account>"
# $env:PAYANYWAY_INTEGRITY_CODE="<integrity-code>"
# $env:PAYANYWAY_TEST_MODE="true"
# $env:PAYANYWAY_PAYMENT_URL="https://www.payanyway.ru/assistant.htm"
# Optional payment system override: card, sbpc2b, etc.
# $env:PAYANYWAY_PAYMENT_UNIT_ID="sbpc2b"
# Optional taxation system attribute for PayAnyWay receipt XML response.
# $env:PAYANYWAY_SNO="<tax-system-code>"
# Pay URL / payment notification URL in PayAnyWay settings:
# https://<public-url>/webhooks/payanyway
