const AUTH_ORIGINS = new Set([
  'https://authentication.fatsecret.com',
]);

function parseWebURL(value) {
  try {
    const url = new URL(value);
    if (url.username || url.password) return null;
    const local = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname);
    return url.protocol === 'https:' || (url.protocol === 'http:' && local) ? url : null;
  } catch {
    return null;
  }
}

function serverURL(value) {
  const url = typeof value === 'string' && parseWebURL(value.trim());
  if (!url || url.pathname !== '/' || url.search || url.hash) {
    throw new Error('Укажите адрес сайта без пути и параметров: https://ваш-домен. Для локальной разработки разрешён http://localhost:8080.');
  }
  return url.origin;
}

function isAppURL(value, origin) {
  return parseWebURL(value)?.origin === origin;
}

function navigationKind(value, origin) {
  const url = parseWebURL(value);
  if (!url) return 'blocked';
  if (url.origin === origin || AUTH_ORIGINS.has(url.origin)) return 'internal';
  return 'external';
}

function isControlsSender(event, contents, url) {
  return event.sender === contents && event.senderFrame === contents.mainFrame && event.senderFrame?.url === url;
}

module.exports = { serverURL, isAppURL, navigationKind, isControlsSender };
