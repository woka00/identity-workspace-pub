const test = require('node:test');
const assert = require('node:assert/strict');
const { serverURL, isAppURL, navigationKind, isControlsSender } = require('../policy.cjs');
const origin = 'https://workspace.example.com';

test('window actions accept only the local controls main frame', () => {
  const url = 'file:///app/controls/index.html';
  const mainFrame = { url };
  const contents = { mainFrame };
  assert.equal(isControlsSender({ sender: contents, senderFrame: mainFrame }, contents, url), true);
  assert.equal(isControlsSender({ sender: {}, senderFrame: mainFrame }, contents, url), false);
  assert.equal(isControlsSender({ sender: contents, senderFrame: { url } }, contents, url), false);
  assert.equal(isControlsSender({ sender: contents, senderFrame: null }, contents, url), false);
  mainFrame.url = origin;
  assert.equal(isControlsSender({ sender: contents, senderFrame: mainFrame }, contents, url), false);
});

test('server address accepts HTTPS and explicit loopback development origins', () => {
  assert.equal(serverURL(` ${origin}/ `), origin);
  for (const url of ['http://localhost:8080', 'http://127.0.0.1:8080', 'http://[::1]:8080']) {
    assert.equal(serverURL(url), url);
  }
});

test('server address rejects credentials, insecure remote hosts and ambiguous base paths', () => {
  for (const url of [null, {}, '', 'example.com', 'file:///tmp/a', 'javascript:alert(1)',
    'http://example.com', 'http://localhost.evil.test', 'https://user:password@example.com',
    `${origin}/tasks`, `${origin}?token=private`, `${origin}#private`]) {
    assert.throws(() => serverURL(url));
  }
});

test('navigation preserves the workspace and the existing OAuth provider', () => {
  for (const url of [`${origin}/?view=profile`,
    'https://authentication.fatsecret.com/oauth/authorize']) {
    assert.equal(navigationKind(url, origin), 'internal');
  }
});

test('lookalike origins and different ports never receive workspace privileges', () => {
  for (const url of ['https://workspace.example.com.evil.test', 'https://workspace.example.com:444',
    'https://example.com']) {
    assert.equal(isAppURL(url, origin), false);
  }
  assert.equal(navigationKind('https://authentication.fatsecret.com.evil.test', origin), 'external');
});

test('external opening excludes OS handlers, script URLs, credentials and insecure hosts', () => {
  for (const url of ['file:///etc/passwd', 'javascript:alert(1)', 'data:text/html,hello',
    'ms-settings:privacy', 'mailto:user@example.com', 'https://user:secret@example.com', 'http://example.com']) {
    assert.equal(navigationKind(url, origin), 'blocked');
  }
  assert.equal(navigationKind('https://example.com/help', origin), 'external');
});
