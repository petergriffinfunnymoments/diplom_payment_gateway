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
