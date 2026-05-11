# Copy this file to payment_gateway_tools/run.local.ps1 and fill real values.
# run.local.ps1 is ignored by Git.

# Optional: protect merchant secret_key values in PostgreSQL through HashiCorp Vault Transit.
# $env:SECRET_PROTECTOR="vault_transit"
# $env:VAULT_ADDR="http://127.0.0.1:8200"
# $env:VAULT_TOKEN_FILE="$PSScriptRoot\vault_token.local.txt"
# $env:VAULT_TRANSIT_MOUNT="transit"
# $env:VAULT_TRANSIT_KEY="payment-gateway-merchant-secrets"
# $env:VAULT_NAMESPACE="<optional-enterprise-or-hcp-namespace>"
