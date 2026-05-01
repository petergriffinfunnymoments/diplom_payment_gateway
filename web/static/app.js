/* eslint-disable no-console */

const els = {
  form: document.getElementById('paymentForm'),
  payBtn: document.getElementById('payBtn'),
  fillDemoBtn: document.getElementById('fillDemoBtn'),
  statusBox: document.getElementById('statusBox'),
  responseBox: document.getElementById('responseBox'),

  amount: document.getElementById('amount'),
  currency: document.getElementById('currency'),
  description: document.getElementById('description'),
  email: document.getElementById('email'),
  phone: document.getElementById('phone'),

  paymentMethodRadios: document.querySelectorAll('input[name="paymentMethod"]'),

  // SBP
  sbpPhone: document.getElementById('sbpPhone'),

  // CARD
  cardNumber: document.getElementById('cardNumber'),
  cardDate: document.getElementById('cardDate'),
  cvv: document.getElementById('cvv'),

  // WALLET
  walletId: document.getElementById('walletId'),
};

function showStatus(message, isError = false) {
  els.statusBox.classList.remove('hidden');
  els.statusBox.classList.toggle('error', !!isError);
  els.statusBox.textContent = message;
}

function hideStatus() {
  els.statusBox.classList.add('hidden');
}

function showResponse(obj) {
  els.responseBox.classList.remove('hidden');
  els.responseBox.textContent = JSON.stringify(obj, null, 2);
}

function getSelectedPaymentMethod() {
  for (const r of els.paymentMethodRadios) {
    if (r.checked) return r.value;
  }
  return 'СБП';
}

function nowIso() {
  // Go time.Time unmarshalling ожидает RFC3339.
  return new Date().toISOString();
}

function safeUUID() {
  if (crypto && crypto.randomUUID) return crypto.randomUUID();
  // fallback (student demo)
  return 'uuid-' + Math.random().toString(16).slice(2) + '-' + Date.now().toString(16);
}

function generatePaymentId() {
  return 'pay_' + Math.random().toString(16).slice(2);
}

function setMethodFieldsVisibility(method) {
  const sbpFields = document.getElementById('sbpFields');
  const cardFields = document.getElementById('cardFields');
  const walletFields = document.getElementById('walletFields');

  sbpFields.classList.toggle('hidden', method !== 'СБП');
  cardFields.classList.toggle('hidden', method !== 'Банковская карта');
  walletFields.classList.toggle('hidden', method !== 'Цифровой кошелек');
}

function buildRequestPayload() {
  const paymentMethod = getSelectedPaymentMethod();

  const merchant_id = 'merchant_12345';
  const idempotency_key = safeUUID();
  const payment_id = generatePaymentId();

  const base = {
    merchant_id,
    idempotency_key,
    payment_id,
    current_status: 'CREATED',
    payment_info: {
      amount: {
        value: Number(els.amount.value),
        currency: els.currency.value,
      },
      payment_method_data: {
        type: paymentMethod,
      },
      customer_data: {
        email: els.email.value || undefined,
        phone: els.phone.value || undefined,

        // SBP
        // (вместо отдельного поля мы кладём phone)
        // Go omitempty сам уберёт пустые строки
        card_number: undefined,
        card_date: undefined,
        CVV_code: undefined,
        digital_wallet_id: undefined,
      },
      created_at: nowIso(),
      description: els.description.value || '',
    },
  };

  // Заполняем метод-специфичные поля
  if (paymentMethod === 'СБП') {
    base.payment_info.customer_data.phone = els.sbpPhone.value || els.phone.value || undefined;
  } else if (paymentMethod === 'Банковская карта') {
    base.payment_info.customer_data.card_number = els.cardNumber.value || undefined;
    base.payment_info.customer_data.card_date = els.cardDate.value || undefined;
    base.payment_info.customer_data.CVV_code = els.cvv.value || undefined;
    // телефон оставляем как в общем поле, если заполнен
  } else if (paymentMethod === 'Цифровой кошелек') {
    base.payment_info.customer_data.digital_wallet_id = els.walletId.value || undefined;
  }

  return base;
}

async function submitPayment() {
  hideStatus();
  els.responseBox.classList.add('hidden');

  const payload = buildRequestPayload();

  els.payBtn.disabled = true;
  els.payBtn.textContent = 'Отправляем...';

  try {
    const res = await fetch('/payments', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json; charset=utf-8' },
      body: JSON.stringify(payload),
    });

    const text = await res.text();
    let data;
    try {
      data = JSON.parse(text);
    } catch {
      data = { raw: text };
    }

    if (!res.ok) {
      showStatus(`Ошибка HTTP ${res.status}`, true);
      showResponse(data);
      return;
    }

    showStatus('Ответ получен успешно');
    showResponse(data);
  } catch (e) {
    showStatus('Сетевая ошибка: ' + (e?.message || String(e)), true);
  } finally {
    els.payBtn.disabled = false;
    els.payBtn.textContent = 'Оплатить';
  }
}

function wireUI() {
  // initial visibility
  setMethodFieldsVisibility(getSelectedPaymentMethod());

  els.paymentMethodRadios.forEach((r) => {
    r.addEventListener('change', () => setMethodFieldsVisibility(getSelectedPaymentMethod()));
  });

  els.form.addEventListener('submit', async (e) => {
    e.preventDefault();
    await submitPayment();
  });

  els.fillDemoBtn.addEventListener('click', () => {
    els.amount.value = 1500;
    els.currency.value = 'RUB';
    els.description.value = 'Оплата заказа №67890';

    els.email.value = 'customer@example.com';
    els.phone.value = '+79991234567';

    // by default SBP
    els.sbpPhone.value = '+79991234567';
    els.cardNumber.value = '4111111111111111';
    els.cardDate.value = '12/29';
    els.cvv.value = '123';
    els.walletId.value = 'wallet_123';

    showStatus('Демо-данные заполнены');
    els.responseBox.classList.add('hidden');
  });
}

wireUI();
