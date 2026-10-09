# Security policy

Do not commit `.env`, the `secrets/` directory, database dumps, TLS/private keys,
OAuth credentials, session cookies, production logs containing personal data, or
`DATA_ENCRYPTION_KEY`.

Production deployment requirements and the limitations of the dated white-box review are in:

- `SECURITY_AUDIT_RU.md`
- `PRODUCTION_DEPLOYMENT_RU.md`

A suspected credential leak requires immediate provider-token revocation, password
rotation, session revocation, database-secret rotation where applicable, log review,
and assessment of whether personal data was exposed. Do not rotate
`DATA_ENCRYPTION_KEY` blindly: existing encrypted OAuth credentials must first be
re-encrypted or users will need to reconnect integrations.

Web Push subscriptions are bound to one current user per endpoint. The default
VAPID key is derived from `DATA_ENCRYPTION_KEY`; rotating that key invalidates
existing browser subscriptions. Treat an explicit `VAPID_PRIVATE_KEY` as a
production secret and never commit it.

Public registration requires Brevo email verification and explicit configuration.
Passwords use salted PBKDF2-HMAC-SHA256; verification tokens are stored as SHA-256
hashes. Email uses AES-256-GCM with per-account associated data and a keyed HMAC
blind index. `DATA_ENCRYPTION_KEY` also derives that index key: rotating it requires
re-encrypting emails AND rebuilding their indexes, as well as handling OAuth/Push.
Do not log registration request bodies or Brevo response bodies. Brevo receives
the recipient and verification link to deliver mail; configure its tracking/log
retention separately. See `docs/REGISTRATION_RU.md` for setup and limits.
