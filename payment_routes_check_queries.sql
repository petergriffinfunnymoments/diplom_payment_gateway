SELECT
  id,
  merchant_id,
  payment_method,
  provider,
  payment_system,
  priority,
  active,
  updated_at
FROM merchant_payment_routes
ORDER BY merchant_id, payment_method, priority, id;

SELECT
  merchant_id,
  payment_method,
  provider,
  payment_system,
  priority,
  active
FROM merchant_payment_routes
WHERE merchant_id = 'merchant_12345'
  AND payment_method = 'Банковская карта'
  AND active = TRUE
ORDER BY priority ASC, id ASC
LIMIT 1;
