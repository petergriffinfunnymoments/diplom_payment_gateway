$env:DATABASE_URL="postgres://postgres:886540@localhost:5432/payment_gateway?sslmode=disable"

$env:YOOKASSA_SHOP_ID="1348152"
$env:YOOKASSA_SECRET_KEY="test_-piTcwmu-KCZlaEmMNFXcXD5KaYrtObHLQFGmL2GFtM"
$env:CARD_PAYMENT_PROVIDER="yookassa"
$env:PAYMENT_RETURN_URL="https://rude-pugs-push.loca.lt"

$env:MERCHANT_WEBHOOK_SECRET="demo_secret"

# Демо-ключи интернет-магазина для подписи запросов к твоему API-шлюзу.
# Если меняешь их здесь, поменяй MERCHANT_AUTH в web/static/app.js.
$env:MERCHANT_ID="merchant_12345"
$env:MERCHANT_NAME="Демонстрационный интернет-магазин"
$env:MERCHANT_API_KEY="demo_api_key"
$env:MERCHANT_SECRET_KEY="demo_secret_key"