# KUB Deploy Ready Guide

## 1) Подготовка сервера (Ubuntu)
1. Создайте директорию приложения:
   ```bash
   sudo mkdir -p /srv/apps/kub && sudo chown -R $USER:$USER /srv/apps/kub
   ```
2. Установите Docker + Compose plugin:
   ```bash
   sudo apt update
   sudo apt install -y ca-certificates curl gnupg
   sudo install -m 0755 -d /etc/apt/keyrings
   curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
   echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo $VERSION_CODENAME) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
   sudo apt update
   sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
   ```
3. Откройте порты в firewall: `80`, `443`, при необходимости `22`.
4. Скопируйте репозиторий в `/srv/apps/kub`.

## 2) Секреты и конфиг
- Храните `.env.prod` только на сервере, не коммитьте его.
- Используйте `config/config.example.yaml` как основу и создайте `config/config.yaml`.
- В `docker-compose.prod.yml` уже используется `CONFIG_PATH` и `DATABASE_URL` для сервиса `api`.

Минимальные переменные в `.env.prod`:
- `JWT_SECRET`
- `DATABASE_URL` (или `POSTGRES_*`)
- `TELEGRAM_ENABLE`, `TELEGRAM_BOT_TOKEN`/`TELEGRAM_APITOKEN`, `TELEGRAM_WEBHOOK_URL`
- `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `EMAIL_FROM`
- `SIGN_EMAIL_TOKEN_PEPPER`, `SIGN_EMAIL_VERIFY_BASE_URL`, `SIGN_BASE_URL`

## 3) Домен и HTTPS (кратко)
- Рекомендуется reverse proxy (Nginx/Caddy) перед `api`.
- Для Nginx:
  - 80 -> редирект на 443
  - 443 -> proxy_pass на `http://api:4000`
  - сертификаты через Let's Encrypt.

## 4) Telegram webhook на прод-домене
Установите:
```env
TELEGRAM_WEBHOOK_URL=https://<domain>/integrations/telegram/webhook
```
Проверка:
```bash
curl "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getWebhookInfo"
```
Убедитесь, что `url` совпадает с прод-доменом.

## 5) Email (Mail.ru app password)
- Используйте пароль приложения, не основной пароль аккаунта.
- Мониторьте лимиты отправки и ошибки SMTP.
- Храните SMTP секреты только в `.env.prod`/секрет-хранилище.

## 6) Preflight и запуск
Перед выкладкой:
```bash
make preflight
```
Запуск:
```bash
make prod-up
make logs
```
Проверка:
```bash
curl -i http://localhost:4000/healthz
```


### Windows PowerShell
Для Windows используйте скрипт:
```powershell
powershell -ExecutionPolicy Bypass -File scripts/preflight.ps1
```

## 7) Откат
1. Остановите текущий релиз:
   ```bash
   make prod-down
   ```
2. Верните предыдущий image tag / commit.
3. Поднимите сервис заново:
   ```bash
   make prod-up
   ```

## 8) Диагностика
- Логи: `make logs`
- SQL shell: `make db-psql`
- Smoke: `make smoke`
