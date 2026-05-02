# Тестовые сценарии антифрода

## PASSED: обычный СБП-платёж
Сумма: 1500 RUB, телефон: +79991234567.
Ожидаемо: current_status = CAPTURED, fraud_check_result = PASSED.

## REVIEW: крупная сумма
Сумма: 150000 RUB.
Ожидаемо: платёж не блокируется, но fraud_check_result = REVIEW.

## BLOCKED: слишком крупная сумма
Сумма: 500000 RUB или выше.
Ожидаемо: current_status = DECLINED, error.code = ANTIFRAUD_DECLINED, fraud_check_result = BLOCKED.

## BLOCKED: заблокированный кошелёк
Способ оплаты: Цифровой кошелек, digital_wallet_id = blocked_wallet.
Ожидаемо: current_status = DECLINED, error.code = ANTIFRAUD_DECLINED.

## REVIEW: подозрительный email
email содержит fraud/scam/blocked или домен mailinator.com / tempmail.com / 10minutemail.com.
Ожидаемо: fraud_check_result = REVIEW.
