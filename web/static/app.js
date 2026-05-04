/* eslint-disable no-console */

const els = {
  form: document.getElementById('paymentForm'),
  payBtn: document.getElementById('payBtn'),
  fillDemoBtn: document.getElementById('fillDemoBtn'),
  statusBox: document.getElementById('statusBox'),
  responseBox: document.getElementById('responseBox'),
  statusPaymentId: document.getElementById('statusPaymentId'),
  statusMerchantId: document.getElementById('statusMerchantId'),
  checkStatusBtn: document.getElementById('checkStatusBtn'),

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

const PAYMENT_METHODS = {
  SBP: 'СБП',
  CARD: 'Банковская карта',
  WALLET: 'Цифровой кошелек',
};

const validators = {
  phone: /^\+7\d{10}$/,
  cardNumber: /^\d{16,19}$/,
  cardDate: /^(0[1-9]|1[0-2])\/\d{2}$/,
  cvv: /^\d{3}$/,
  walletId: /^[A-Za-z0-9_-]{3,64}$/,
};

function showStatus(message, isError = false) {
  els.statusBox.classList.remove('hidden');
  els.statusBox.classList.toggle('error', !!isError);
  els.statusBox.textContent = message;
}

function hideStatus() {
  els.statusBox.classList.add('hidden');
  els.statusBox.textContent = '';
}

function showResponse(obj) {
  els.responseBox.classList.remove('hidden');
  els.responseBox.textContent = JSON.stringify(obj, null, 2);
  showProviderPaymentLink(obj);

  if (obj?.id && els.statusPaymentId) {
    els.statusPaymentId.value = obj.id;
  }
  if (obj?.merchant_id && els.statusMerchantId) {
    els.statusMerchantId.value = obj.merchant_id;
  }
}

function showProviderPaymentLink(obj) {
  const old = document.getElementById('providerPaymentLinkBox');
  if (old) old.remove();

  const paymentUrl = obj?.transaction_details?.payment_url;
  if (!paymentUrl) return;

  const box = document.createElement('div');
  box.id = 'providerPaymentLinkBox';
  box.className = 'provider-payment-link';

  const text = document.createElement('span');
  text.textContent = 'Платёж создан во внешней платёжной системе. ';

  const link = document.createElement('a');
  link.href = paymentUrl;
  link.target = '_blank';
  link.rel = 'noopener noreferrer';
  link.textContent = 'Перейти к оплате в ЮKassa';

  box.appendChild(text);
  box.appendChild(link);
  els.responseBox.insertAdjacentElement('beforebegin', box);
}

function getSelectedPaymentMethod() {
  for (const r of els.paymentMethodRadios) {
    if (r.checked) return r.value;
  }
  return PAYMENT_METHODS.SBP;
}

function nowIso() {
  // Go time.Time unmarshalling ожидает RFC3339.
  return new Date().toISOString();
}

function safeUUID() {
  if (crypto && crypto.randomUUID) return crypto.randomUUID();

  // fallback для учебного демо.
  return 'uuid-' + Math.random().toString(16).slice(2) + '-' + Date.now().toString(16);
}

function generatePaymentId() {
  return 'pay_' + Math.random().toString(16).slice(2);
}

function digitsOnly(value) {
  return String(value || '').replace(/\D/g, '');
}

function normalizePhoneInput(value) {
  const raw = String(value || '').trim();
  const digits = digitsOnly(raw);

  if (!digits) return '';

  // Пользователь может начать ввод с 8, 7 или сразу с 9XXXXXXXXX.
  if (digits.startsWith('8')) return '+7' + digits.slice(1, 11);
  if (digits.startsWith('7')) return '+7' + digits.slice(1, 11);
  if (digits.startsWith('9')) return '+7' + digits.slice(0, 10);

  return '+7' + digits.slice(0, 10);
}

function normalizeCardDateInput(value) {
  const digits = digitsOnly(value).slice(0, 4);

  if (digits.length <= 2) return digits;

  return digits.slice(0, 2) + '/' + digits.slice(2);
}

function setInvalid(input, message) {
  input.setCustomValidity(message);
  input.classList.add('invalid');
}

function setValid(input) {
  input.setCustomValidity('');
  input.classList.remove('invalid');
}

function validateOptionalPhone(input, label) {
  const value = input.value.trim();

  if (!value) {
    setValid(input);
    return true;
  }

  if (!validators.phone.test(value)) {
    setInvalid(input, `${label} должен быть в формате +79991234567`);
    return false;
  }

  setValid(input);
  return true;
}

function isFutureCardDate(value) {
  if (!validators.cardDate.test(value)) return false;

  const [monthRaw, yearRaw] = value.split('/');
  const month = Number(monthRaw);
  const year = 2000 + Number(yearRaw);

  // Карта действительна до конца указанного месяца.
  const now = new Date();
  const currentMonthStart = new Date(now.getFullYear(), now.getMonth(), 1);
  const cardNextMonthStart = new Date(year, month, 1);

  return cardNextMonthStart > currentMonthStart;
}

function luhnCheck(cardNumber) {
  let sum = 0;
  let shouldDouble = false;

  for (let i = cardNumber.length - 1; i >= 0; i -= 1) {
    let digit = Number(cardNumber[i]);

    if (shouldDouble) {
      digit *= 2;
      if (digit > 9) digit -= 9;
    }

    sum += digit;
    shouldDouble = !shouldDouble;
  }

  return sum % 10 === 0;
}

function validateForm() {
  const method = getSelectedPaymentMethod();
  const errors = [];

  // Сначала очищаем старые ошибки.
  [
    els.amount,
    els.description,
    els.email,
    els.phone,
    els.sbpPhone,
    els.cardNumber,
    els.cardDate,
    els.cvv,
    els.walletId,
  ].forEach(setValid);

  const amount = Number(els.amount.value);

  if (!Number.isFinite(amount) || amount <= 0 || amount > 1000000) {
    setInvalid(els.amount, 'Сумма должна быть больше 0 и не больше 1 000 000 RUB');
    errors.push('Сумма должна быть больше 0 и не больше 1 000 000 RUB.');
  }

  if (els.description.value.trim().length < 3) {
    setInvalid(els.description, 'Описание должно содержать минимум 3 символа');
    errors.push('Описание должно содержать минимум 3 символа.');
  }

  if (els.email.value.trim() && !els.email.checkValidity()) {
    setInvalid(els.email, 'Введите email в формате customer@example.com');
    errors.push('Email указан некорректно.');
  }

  if (!validateOptionalPhone(els.phone, 'Телефон')) {
    errors.push('Телефон должен быть в формате +79991234567.');
  }

  if (method === PAYMENT_METHODS.SBP) {
    if (!els.sbpPhone.value.trim()) {
      setInvalid(els.sbpPhone, 'Для СБП обязательно укажите телефон');
      errors.push('Для СБП обязательно укажите телефон.');
    } else if (!validateOptionalPhone(els.sbpPhone, 'Телефон для СБП')) {
      errors.push('Телефон для СБП должен быть в формате +79991234567.');
    }
  }

  if (method === PAYMENT_METHODS.CARD) {
    const cardNumber = digitsOnly(els.cardNumber.value);
    els.cardNumber.value = cardNumber;

    if (!validators.cardNumber.test(cardNumber)) {
      setInvalid(els.cardNumber, 'Номер карты должен содержать от 16 до 19 цифр');
      errors.push('Номер карты должен содержать от 16 до 19 цифр.');
    } else if (!luhnCheck(cardNumber)) {
      setInvalid(els.cardNumber, 'Номер карты не прошел проверку по алгоритму Луна');
      errors.push('Номер карты не прошел проверку по алгоритму Луна.');
    }

    if (!validators.cardDate.test(els.cardDate.value)) {
      setInvalid(els.cardDate, 'Срок карты должен быть в формате ММ/ГГ');
      errors.push('Срок карты должен быть в формате ММ/ГГ.');
    } else if (!isFutureCardDate(els.cardDate.value)) {
      setInvalid(els.cardDate, 'Срок действия карты истек');
      errors.push('Срок действия карты истек.');
    }

    if (!validators.cvv.test(els.cvv.value)) {
      setInvalid(els.cvv, 'CVV должен содержать ровно 3 цифры');
      errors.push('CVV должен содержать ровно 3 цифры.');
    }
  }

  if (method === PAYMENT_METHODS.WALLET) {
    if (!validators.walletId.test(els.walletId.value.trim())) {
      setInvalid(els.walletId, 'ID кошелька: латинские буквы, цифры, _ и -, от 3 до 64 символов');
      errors.push('ID кошелька может содержать латинские буквы, цифры, _ и -, от 3 до 64 символов.');
    }
  }

  if (errors.length > 0) {
    showStatus(errors[0], true);

    const firstInvalid = els.form.querySelector('.invalid');

    if (firstInvalid) {
      firstInvalid.focus();
      firstInvalid.reportValidity();
    }

    return false;
  }

  return true;
}

function setMethodFieldsVisibility(method) {
  const sbpFields = document.getElementById('sbpFields');
  const cardFields = document.getElementById('cardFields');
  const walletFields = document.getElementById('walletFields');

  sbpFields.classList.toggle('hidden', method !== PAYMENT_METHODS.SBP);
  cardFields.classList.toggle('hidden', method !== PAYMENT_METHODS.CARD);
  walletFields.classList.toggle('hidden', method !== PAYMENT_METHODS.WALLET);

  // Required включаем только для полей выбранного способа оплаты.
  els.sbpPhone.required = method === PAYMENT_METHODS.SBP;

  els.cardNumber.required = method === PAYMENT_METHODS.CARD;
  els.cardDate.required = method === PAYMENT_METHODS.CARD;
  els.cvv.required = method === PAYMENT_METHODS.CARD;

  els.walletId.required = method === PAYMENT_METHODS.WALLET;

  // Скрытые поля не должны блокировать отправку формы.
  [els.sbpPhone, els.cardNumber, els.cardDate, els.cvv, els.walletId].forEach(setValid);

  hideStatus();
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
        email: els.email.value.trim() || undefined,
        phone: els.phone.value.trim() || undefined,

        // Go omitempty сам уберёт пустые строки.
        card_number: undefined,
        card_date: undefined,
        CVV_code: undefined,
        digital_wallet_id: undefined,
      },
      created_at: nowIso(),
      description: els.description.value.trim(),
    },
  };

  // Заполняем метод-специфичные поля.
  if (paymentMethod === PAYMENT_METHODS.SBP) {
    base.payment_info.customer_data.phone = els.sbpPhone.value.trim() || els.phone.value.trim() || undefined;
  } else if (paymentMethod === PAYMENT_METHODS.CARD) {
    base.payment_info.customer_data.card_number = digitsOnly(els.cardNumber.value);
    base.payment_info.customer_data.card_date = els.cardDate.value.trim();
    base.payment_info.customer_data.CVV_code = els.cvv.value.trim();
  } else if (paymentMethod === PAYMENT_METHODS.WALLET) {
    base.payment_info.customer_data.digital_wallet_id = els.walletId.value.trim();
  }

  return base;
}

async function submitPayment() {
  hideStatus();
  els.responseBox.classList.add('hidden');
  const oldPaymentLink = document.getElementById('providerPaymentLinkBox');
  if (oldPaymentLink) oldPaymentLink.remove();

  if (!validateForm()) return;

  const payload = buildRequestPayload();

  els.payBtn.disabled = true;
  els.payBtn.textContent = 'Отправляем...';

  try {
    const res = await fetch('/payments', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json; charset=utf-8',
      },
      body: JSON.stringify(payload),
    });

    const text = await res.text();

    let data;

    try {
      data = JSON.parse(text);
    } catch {
      data = {
        raw: text,
      };
    }

    if (!res.ok || data?.error) {
      showStatus(data?.error?.message || `Ошибка HTTP ${res.status}`, true);
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


async function checkPaymentStatus() {
  hideStatus();
  els.responseBox.classList.add('hidden');
  const oldPaymentLink = document.getElementById('providerPaymentLinkBox');
  if (oldPaymentLink) oldPaymentLink.remove();

  const paymentId = els.statusPaymentId.value.trim();
  const merchantId = els.statusMerchantId.value.trim() || 'merchant_12345';

  if (!paymentId) {
    showStatus('Укажите payment_id для проверки статуса', true);
    els.statusPaymentId.focus();
    return;
  }

  els.checkStatusBtn.disabled = true;
  els.checkStatusBtn.textContent = 'Проверяем...';

  try {
    const res = await fetch(`/payments/${encodeURIComponent(paymentId)}?merchant_id=${encodeURIComponent(merchantId)}`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
      },
    });

    const text = await res.text();
    let data;
    try {
      data = JSON.parse(text);
    } catch {
      data = { raw: text };
    }

    if (!res.ok || data?.code) {
      showStatus(data?.message || `Ошибка HTTP ${res.status}`, true);
      showResponse(data);
      return;
    }

    showStatus('Актуальный статус получен');
    showResponse(data);
  } catch (e) {
    showStatus('Сетевая ошибка: ' + (e?.message || String(e)), true);
  } finally {
    els.checkStatusBtn.disabled = false;
    els.checkStatusBtn.textContent = 'Проверить статус';
  }
}

function wireInputFilters() {
  [els.phone, els.sbpPhone].forEach((input) => {
    input.addEventListener('input', () => {
      input.value = normalizePhoneInput(input.value);
      validateOptionalPhone(input, input === els.sbpPhone ? 'Телефон для СБП' : 'Телефон');
    });
  });

  els.cardNumber.addEventListener('input', () => {
    els.cardNumber.value = digitsOnly(els.cardNumber.value).slice(0, 19);
    setValid(els.cardNumber);
  });

  els.cardDate.addEventListener('input', () => {
    els.cardDate.value = normalizeCardDateInput(els.cardDate.value);
    setValid(els.cardDate);
  });

  els.cvv.addEventListener('input', () => {
    els.cvv.value = digitsOnly(els.cvv.value).slice(0, 3);
    setValid(els.cvv);
  });

  els.walletId.addEventListener('input', () => {
    els.walletId.value = els.walletId.value.replace(/[^A-Za-z0-9_-]/g, '').slice(0, 64);
    setValid(els.walletId);
  });
}

function wireUI() {
  // initial visibility
  setMethodFieldsVisibility(getSelectedPaymentMethod());
  wireInputFilters();

  els.paymentMethodRadios.forEach((r) => {
    r.addEventListener('change', () => setMethodFieldsVisibility(getSelectedPaymentMethod()));
  });

  els.form.addEventListener('submit', async (e) => {
    e.preventDefault();
    await submitPayment();
  });

  els.checkStatusBtn.addEventListener('click', async () => {
    await checkPaymentStatus();
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

    [
      els.amount,
      els.description,
      els.email,
      els.phone,
      els.sbpPhone,
      els.cardNumber,
      els.cardDate,
      els.cvv,
      els.walletId,
    ].forEach(setValid);

    showStatus('Демо-данные заполнены');
    els.responseBox.classList.add('hidden');
  });
}

wireUI();