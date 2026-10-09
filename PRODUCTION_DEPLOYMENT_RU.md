# AVATAR.ID — безопасный выход в production на VPS

Документ рассчитан на один VPS и один экземпляр backend за Nginx. Команды приведены для Linux с Docker Compose v2 и Nginx. Подставьте свой домен вместо `avatar.example.com`.


## Роль Makefile

Makefile предназначен только для управления AVATAR.ID после того, как Docker, reverse proxy, DNS и HTTPS уже настроены владельцем сервера. Он не устанавливает пакеты, не меняет firewall/SSH/Nginx и не перезапускает чужие службы.

На VPS с другими проектами обязательно задайте в `.env` уникальные значения:

```dotenv
COMPOSE_PROJECT_NAME=avatar-id-prod
APP_PORT=18080
```

Проверьте, что порт свободен, а ваш Nginx/Caddy/Traefik проксирует домен на `127.0.0.1:18080`. После ручной подготовки инфраструктуры используйте:

```bash
make doctor
make check
make deploy
```

Для обновлений: `make update`. Подробности приведены в `MAKEFILE_RU.md`.

## 0. Правила, которые нельзя нарушать

- Никогда не выполняйте `docker compose down -v`: это удалит volume PostgreSQL.
- Не публикуйте наружу порты `5432` и `8080`.
- Не помещайте реальные секреты в `.env`, Git, ZIP, сообщения или screenshot.
- Не меняйте `DATA_ENCRYPTION_KEY` после первого production-запуска без процедуры re-encryption.
- До любой миграции сделайте backup и проверьте, что файл не пустой.
- Не включайте аккаунты с preview-паролями: приложение само заблокирует production startup.

## 1. Подготовить VPS

1. Обновите ОС и установите Docker Engine/Compose, Nginx, Certbot и firewall.
2. Используйте отдельного непривилегированного пользователя для проекта; добавьте его в группу `docker` только если понимаете, что доступ к Docker эквивалентен root-доступу.
3. В SSH используйте ключи. Отключение password login выполняйте только после проверки второго открытого SSH-сеанса.
4. Разрешите входящие соединения только на SSH, 80 и 443. Пример UFW:

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow OpenSSH        # измените правило, если SSH работает не на 22 порту
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
sudo ufw status verbose
```

5. Проверьте DNS A/AAAA: домен должен указывать на VPS. Если IPv6 не настроен полностью, не создавайте ошибочную AAAA-запись.

## 2. Проверить архив и распаковать проект

Сначала положите ZIP и файл `.sha256` в один каталог и проверьте архив **до распаковки**:

```bash
cd /путь/к/скачанным/файлам
sha256sum -c avatar-id-closed-preview-21-project-makefile.zip.sha256
```

Только после успешной проверки распакуйте:

```bash
sudo mkdir -p /opt/avatar-id
sudo chown "$USER":"$USER" /opt/avatar-id
cd /opt/avatar-id
unzip /путь/avatar-id-closed-preview-21-project-makefile.zip
```

Файлы `Dockerfile`, `docker-compose.yml`, `backend`, `frontend` должны оказаться непосредственно в `/opt/avatar-id`, без дополнительной папки `avatar-id`.

## 3. Сначала сохранить существующую базу

Если текущий проект уже работает, сделайте dump **до замены файлов**:

```bash
cd /путь/к/текущему/проекту
umask 077
mkdir -p backups
chmod 700 backups
docker compose exec -T db sh -ec \
  'exec pg_dump --format=custom --compress=6 --no-owner --no-privileges --username="$POSTGRES_USER" --dbname="$POSTGRES_DB"' \
  > "backups/pre-security-$(date -u +%Y%m%dT%H%M%SZ).dump"
```

Проверьте размер и структуру:

```bash
ls -lh backups/*.dump
docker compose exec -T db pg_restore --list < backups/имя.dump >/dev/null
```

Скопируйте backup на другое физическое хранилище. После перехода используйте `./deploy/backup.sh`.

## 4. Создать production-конфигурацию и secrets

```bash
cd /opt/avatar-id
cp .env.production.example .env
sed -i 's#https://avatar.example.com#https://ВАШ-ДОМЕН#g' .env
mkdir -p secrets backups
chmod 700 secrets backups
umask 077
```

Создайте DB password и ключ шифрования:

```bash
openssl rand -hex 32 > secrets/postgres_password
openssl rand -base64 32 > secrets/data_encryption_key
chmod 600 secrets/postgres_password secrets/data_encryption_key
```

Создайте `database_url`. Пароль генерируется hex, поэтому безопасен в URL без дополнительного escaping:

```bash
DB_PASSWORD="$(cat secrets/postgres_password)"
printf 'postgres://avatar:%s@db:5432/avatarid?sslmode=disable&connect_timeout=10\n' "$DB_PASSWORD" \
  > secrets/database_url
unset DB_PASSWORD
chmod 600 secrets/database_url
```

`sslmode=disable` допустим только потому, что DB доступна лишь внутри локальной изолированной Docker network. Для удалённой PostgreSQL используйте TLS с проверкой сертификата (`verify-full`).

Создайте файлы provider secrets. Даже если интеграция временно не используется, файл должен существовать, но может быть пустым:

```bash
install -m 600 /dev/null secrets/fatsecret_consumer_key
install -m 600 /dev/null secrets/fatsecret_consumer_secret
```

Заполните их без пробелов и лишней строки только на VPS. Удобно использовать редактор с правами текущего пользователя:

```bash
nano secrets/fatsecret_consumer_key
nano secrets/fatsecret_consumer_secret
chmod 600 secrets/*
```

Проверка без вывода содержимого:

```bash
for f in secrets/*; do printf '%s: %s bytes\n' "$f" "$(wc -c < "$f")"; done
```

## 5. Сменить пароль роли PostgreSQL без потери volume

`POSTGRES_PASSWORD_FILE` применяется автоматически только при создании новой базы. Для существующего `pgdata` он **не меняет** пароль роли.

Запустите только DB с существующим volume:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d db
docker compose -f docker-compose.yml -f docker-compose.prod.yml ps
```

Затем измените пароль роли на значение из secret. Команда безопасна при условии, что пароль создан именно `openssl rand -hex 32`:

```bash
DB_PASSWORD="$(cat secrets/postgres_password)"
printf "ALTER ROLE avatar WITH PASSWORD '%s';\n" "$DB_PASSWORD" | \
  docker compose -f docker-compose.yml -f docker-compose.prod.yml exec -T db \
    psql -U avatar -d postgres -v ON_ERROR_STOP=1
unset DB_PASSWORD
```

Если локальный `psql` требует старый пароль, временно передайте **старый** пароль через `PGPASSWORD` только для этой команды и очистите shell history/переменную. Не удаляйте volume.

Проверьте новое соединение через контейнер приложения позже; не печатайте `database_url` в лог.

## 6. Обновить build toolchain, собрать image и проверить конфигурацию

Vite используется только во время сборки и не входит в runtime-контейнер. В текущем lockfile закреплена исправленная ветка `6.4.3`; dev/preview дополнительно ограничены `127.0.0.1` и не должны публиковаться наружу. Перед production build установите зависимости строго из lockfile и проверьте audit:

```bash
cd /opt/avatar-id/frontend
npm ci --ignore-scripts
npm audit --audit-level=high
npm run build
cd /opt/avatar-id
```

Не используйте `npm audit fix --force` без проверки breaking changes. Обновления зависимостей и lockfile проводите как отдельное ревьюируемое изменение.

Затем выполните полный набор проверок:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml config --quiet
./deploy/security-checks.sh
```

Скрипт должен выполнить Go/npm/audit/image checks. Остановитесь при любом HIGH/CRITICAL или ошибке build. Затем:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml build --pull --no-cache app
```

`--pull` обновляет base images. Перед автоматическим обновлением в будущем проверяйте staging.

## 7. Применить миграции отдельно

Production overlay по умолчанию использует `RUN_MIGRATIONS=false`, чтобы запуск нескольких контейнеров не делал скрытых schema changes. Выполните:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml run --rm app migrate
```

Миграция `016_security_hardening.sql` добавляет только совместимые поля/индексы и не требует удаления данных. Migration runner также записывает SHA-256 для каждого применённого SQL-файла и остановится, если уже применённая миграция была изменена.

## 8. Обязательно заменить preview-пароли или отключить аккаунты

Для каждого реально используемого аккаунта:

```bash
cd /opt/avatar-id
umask 077
read -rsp 'Новый пароль avatar01 (не менее 15 символов): ' PW; echo
printf '%s' "$PW" > secrets/admin_password
unset PW
chmod 600 secrets/admin_password

docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  -f docker-compose.admin.yml \
  run --rm app set-password avatar01

shred -u secrets/admin_password 2>/dev/null || rm -f secrets/admin_password
```

Повторите для всех нужных `avatar01`–`avatar15`. Используйте уникальные случайные пароли, лучше парольный менеджер. Не отправляйте их в мессенджер одной общей таблицей.

Неиспользуемые аккаунты отключите:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml run --rm app disable-user avatar15
```

Самый ранний аккаунт получает доступ к `/admin/foods` при миграции. Для выдачи или отзыва права модерации используйте:

```bash
make grant-admin LOGIN=avatar01
make revoke-admin LOGIN=avatar02
```

Проверьте gate:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml run --rm app security-status
```

Команда должна сообщить, что все включённые аккаунты имеют production passwords. Смена пароля отзывает старые сессии.

## 9. Получить TLS-сертификат и настроить Nginx

Замените домен в шаблонах:

```bash
DOMAIN='ваш-домен.ru'
sed "s/avatar.example.com/$DOMAIN/g" deploy/nginx/avatar-id-bootstrap.conf.example \
  | sudo tee /etc/nginx/sites-available/avatar-id-bootstrap >/dev/null
sudo ln -s /etc/nginx/sites-available/avatar-id-bootstrap /etc/nginx/sites-enabled/avatar-id-bootstrap
sudo mkdir -p /var/www/certbot/.well-known/acme-challenge
sudo nginx -t && sudo systemctl reload nginx
```

Получите сертификат через webroot:

```bash
sudo certbot certonly --webroot -w /var/www/certbot -d "$DOMAIN"
```

Установите rate-limit и основной конфиг:

```bash
sudo cp deploy/nginx/00-avatar-id-rate-limit.conf.example \
  /etc/nginx/conf.d/00-avatar-id-rate-limit.conf
sed "s/avatar.example.com/$DOMAIN/g" deploy/nginx/avatar-id.conf.example \
  | sudo tee /etc/nginx/sites-available/avatar-id >/dev/null
sudo ln -sf /etc/nginx/sites-available/avatar-id /etc/nginx/sites-enabled/avatar-id
sudo rm -f /etc/nginx/sites-enabled/avatar-id-bootstrap
sudo nginx -t && sudo systemctl reload nginx
sudo certbot renew --dry-run
```

Проверьте, что порт 8080 слушает только loopback:

```bash
ss -lntp | grep -E ':(80|443|8080|5432)\b'
```

Ожидается: 80/443 публично через Nginx, 8080 только `127.0.0.1`, 5432 не опубликован.

## 10. Настроить OAuth callbacks

В кабинете FatSecret укажите точный адрес:

```text
https://ВАШ-ДОМЕН/api/integrations/fatsecret/callback
```

Адрес должен совпадать символ в символ, без лишнего `/`, другого поддомена или HTTP.

## 11. Запустить приложение

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d app
docker compose -f docker-compose.yml -f docker-compose.prod.yml ps
docker compose -f docker-compose.yml -f docker-compose.prod.yml logs --tail=200 app
```

В логах не должно быть токенов, паролей, `database_url` или content secrets. Production startup остановится, если passwords не ротированы, ключ неверен или callback/CORS небезопасны.

## 12. Выполнить smoke/security-проверку

```bash
./deploy/verify-production.sh https://ВАШ-ДОМЕН
```

Затем вручную:

1. войдите одним аккаунтом;
2. откройте DevTools → Application → Cookies;
3. cookie должна называться `__Host-avatar_id_session` и иметь Secure/HttpOnly/SameSite=Lax;
4. проверьте задачи, профиль, загрузку допустимого фото;
5. переподключите/проверьте FatSecret;
6. войдите вторым аккаунтом и убедитесь, что не видны данные первого;
7. попробуйте ID первого аккаунта в URL API второго — ожидается 404, не чужие данные;

На staging дополнительно запустите подготовленный пассивный OWASP ZAP Baseline:

```bash
./deploy/staging-dast.sh https://staging.ВАШ-ДОМЕН
```

Скрипт сохраняет отчёты в `SECURITY_TEST_RESULTS/zap`. Активный ZAP scan запускается только после явного флага и только на disposable staging с backup; не направляйте его на production с реальными данными. После этого выполните внешний TLS scanner и ручные authenticated/IDOR-тесты двумя аккаунтами.

## 13. Backup и проверка восстановления

Создать backup:

```bash
./deploy/backup.sh /opt/avatar-id/backups
```

Пример cron ежедневно в 03:20:

```cron
20 3 * * * cd /opt/avatar-id && ./deploy/backup.sh /opt/avatar-id/backups >> /var/log/avatar-id-backup.log 2>&1
```

Обязательно:

- шифровать backup перед отправкой offsite;
- хранить минимум одну копию вне VPS;
- отдельно и безопасно резервировать `DATA_ENCRYPTION_KEY`;
- настроить retention;
- ежемесячно делать фактический restore в временную БД и входить в приложение;
- не считать `pg_restore --list` полноценной проверкой восстановления.

Потеря DB backup означает потерю данных. Потеря `DATA_ENCRYPTION_KEY` означает невозможность прочитать сохранённые OAuth credentials; пользователям придётся переподключать интеграции.

## 14. Мониторинг

Минимальные алерты:

- `/healthz` недоступен;
- контейнер restart loop;
- всплеск 401/403/429/5xx;
- свободное место диска менее 20%;
- backup не создан или слишком мал;
- TLS истекает менее чем через 21 день;
- PostgreSQL container unhealthy;
- необычный рост outbound traffic.

Настройте ротацию системных логов и не логируйте request/response bodies OAuth или passwords.

## 15. Регулярные обновления

Не реже раза в месяц и перед каждым релизом:

```bash
git diff --exit-code  # либо убедитесь, что работаете с чистой копией release
./deploy/security-checks.sh
docker compose -f docker-compose.yml -f docker-compose.prod.yml build --pull --no-cache app
```

Проверяйте changelog и CVE для Go, Node, Alpine, PostgreSQL, Nginx, React/Vite и npm dependencies. Обновления сначала применяйте на staging, затем делайте backup и production rollout.

## 16. Rollback без удаления данных

1. Не удаляйте `pgdata`.
2. Сохраните текущие image tags и backup до релиза.
3. При проблеме остановите только app, верните предыдущий image и запустите его с тем же volume.
4. Учтите: schema migration может быть совместима вперёд, но не обязана поддерживать старую бинарную версию. Поэтому database snapshot перед migration обязателен.
5. Restore production DB выполняйте только после подтверждения причины и с отдельной копией текущего состояния.

## Финальный checklist

- [ ] DNS указывает на VPS.
- [ ] SSH keys и firewall проверены.
- [ ] 5432/8080 не доступны извне.
- [ ] Backup до миграции создан и скопирован offsite.
- [ ] DB password заменён; `avatar/avatar` нигде не используется.
- [ ] `DATA_ENCRYPTION_KEY` создан, сохранён отдельно и не потеряется.
- [ ] Provider secrets находятся только в `secrets/` с mode 600.
- [ ] Vite обновлён минимум до `6.4.3`, lockfile пересоздан, `npm ci`, `npm audit --audit-level=high` и production build прошли.
- [ ] Полный `deploy/security-checks.sh` прошёл в сетевой среде.
- [ ] `deploy/staging-dast.sh` и ручные authenticated/IDOR-тесты прошли на staging.
- [ ] Docker image собран с `--pull --no-cache`.
- [ ] Миграции применены отдельной командой.
- [ ] Все используемые аккаунты получили уникальные новые пароли.
- [ ] Неиспользуемые аккаунты отключены.
- [ ] `security-status` успешен.
- [ ] HTTPS/Nginx установлен, `certbot renew --dry-run` успешен.
- [ ] OAuth callback URL обновлены на production domain.
- [ ] `deploy/verify-production.sh` успешен.
- [ ] Два аккаунта проверены на изоляцию данных.
- [ ] Backup restore реально проверен.
- [ ] Мониторинг и алерты включены.
- [ ] Privacy/retention/delete policy определены.

## Web Push-напоминания

Напоминания используют серверный Web Push и продолжают работать после закрытия вкладки. Для production обязательны HTTPS, неизменный `DATA_ENCRYPTION_KEY` и контактный VAPID subject:

```dotenv
VAPID_SUBJECT=mailto:admin@your-domain.example
```

Отдельный `VAPID_PRIVATE_KEY` обычно не нужен: backend выводит стабильный P-256 VAPID-ключ из `DATA_ENCRYPTION_KEY`. Если вы всё же задаёте `VAPID_PRIVATE_KEY`, это 32 случайных байта в base64url без padding, и этот секрет нельзя менять после подписки устройств. После смены VAPID-ключа старые подписки перестанут принимать уведомления и пользователям потребуется повторно включить их.

Проверка после деплоя:

1. установить PWA на тестовое устройство;
2. создать задачу с напоминанием через 2–3 минуты и разрешить уведомления;
3. полностью закрыть вкладку/окно PWA;
4. убедиться, что системное уведомление пришло и по нажатию открылась вкладка «Задачи»;
5. повторить для второго аккаунта и убедиться, что уведомления не пересекаются.

На iOS/iPadOS проверяйте именно приложение, добавленное на экран «Домой». Обычная вкладка Safari не является эквивалентом установленной PWA для Web Push.
