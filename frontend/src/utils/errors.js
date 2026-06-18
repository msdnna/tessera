// Turn backend / network error strings into friendly Russian for display.
// The backend surfaces two ugly shapes we must tame before showing a user:
//   • gin/validator binding errors, e.g.
//     "Key: 'Email' Error:Field validation for 'Email' failed on the 'email' tag"
//   • bare English sentinels, e.g. "invalid credentials".
// Anything we recognise is mapped; short unknown messages pass through; clearly
// internal noise falls back to a generic line. Applied centrally in the axios
// response interceptor so every `message.error(e.message)` caller benefits.

const SENTINELS = {
  'invalid credentials': 'Неверный email или пароль',
  'invalid email or password': 'Неверный email или пароль',
  'email already registered': 'Этот email уже зарегистрирован',
  'email already exists': 'Этот email уже зарегистрирован',
  'email already in use': 'Этот email уже используется',
  'user already exists': 'Пользователь с таким email уже существует',
  'user not found': 'Пользователь не найден',
  'invalid or expired token': 'Ссылка недействительна или устарела',
  'invalid token': 'Ссылка недействительна или устарела',
  'token expired': 'Срок действия ссылки истёк',
  'account is deactivated': 'Аккаунт деактивирован',
  'account deactivated': 'Аккаунт деактивирован',
  forbidden: 'Недостаточно прав',
  unauthorized: 'Нужно войти заново',
  'not found': 'Не найдено',
}

// Collapse a gin/validator error ("...failed on the 'X' tag") into one friendly
// sentence based on which validation tags failed.
function fromValidator(raw) {
  const tags = [...raw.matchAll(/failed on the '(\w+)' tag/g)].map((m) => m[1])
  if (!tags.length) return null
  if (tags.includes('email')) return 'Введите корректный email'
  if (tags.every((t) => t === 'required')) return 'Заполните все обязательные поля'
  if (tags.includes('min')) return 'Значение слишком короткое'
  if (tags.includes('max')) return 'Значение слишком длинное'
  return 'Проверьте правильность заполнения полей'
}

export function humanizeError(raw) {
  if (!raw) return 'Что-то пошло не так'
  const msg = String(raw).trim()
  const low = msg.toLowerCase()
  if (SENTINELS[low]) return SENTINELS[low]
  if (low.startsWith('key:') && low.includes('validation')) {
    return fromValidator(msg) || 'Проверьте правильность заполнения полей'
  }
  if (
    low.includes('network error') ||
    low.includes('timeout') ||
    low.includes('econnrefused') ||
    low.includes('failed to fetch')
  ) {
    return 'Нет связи с сервером. Проверьте подключение.'
  }
  return msg
}
