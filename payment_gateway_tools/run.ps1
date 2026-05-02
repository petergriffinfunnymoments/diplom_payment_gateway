$env:DATABASE_URL="postgres://postgres:886540@localhost:5432/payment_gateway?sslmode=disable"

$env:YOOKASSA_SHOP_ID="1348152"
$env:YOOKASSA_SECRET_KEY="test_-piTcwmu-KCZlaEmMNFXcXD5KaYrtObHLQFGmL2GFtM"

$env:CARD_PAYMENT_PROVIDER="yookassa"
$env:PAYMENT_RETURN_URL="https://ТВОЙ_LOCALTUNNEL_URL.loca.lt"

go run ./cmd/payment-gateway