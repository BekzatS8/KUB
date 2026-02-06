# Production deploy (from scratch)

## 0) Requirements
- Linux server with Docker + Docker Compose v2 installed.
- Domain pointing to the server (for webhook/HTTPS).

## 1) Clone and update repo
```bash
sudo mkdir -p /opt/kub && sudo chown "$USER":"$USER" /opt/kub
cd /opt/kub
git clone <YOUR_REPO_URL> .
git pull
```

## 2) Prepare configuration and env
1. Create the config directory:
   ```bash
   mkdir -p /opt/kub/config /opt/kub/files
   ```
2. Create `/opt/kub/config/config.yaml` with your production values (example skeleton):
   ```yaml
   server:
     port: 4000

   database:
     url: "postgres://turcompany:CHANGE_ME@postgres:5432/turcompany?sslmode=disable"

   email:
     smtp_host: "smtp.example.com"
     smtp_port: 587
     smtp_user: "noreply@example.com"
     smtp_password: "APP_PASSWORD"
     from_email: "noreply@example.com"

   files:
     root_dir: "/opt/turcompany/files"

   sign_base_url: "https://YOUR-DOMAIN.TLD/sign"
   sign_confirm_policy: "ANY"
   sign_email_verify_base_url: "https://YOUR-DOMAIN.TLD"

   telegram:
     enable: true
     bot_token: "<ENV:TELEGRAM_APITOKEN>"
     webhook_url: "https://YOUR-DOMAIN.TLD/telegram/webhook"

   security:
     jwt_secret: "CHANGE_ME"

   cors:
     allow_origins:
       - "https://YOUR-DOMAIN.TLD"
     allow_methods: "GET, POST, PUT, DELETE, OPTIONS"
     allow_headers: "Origin, Content-Type, Authorization"
     expose_headers: "Content-Disposition, Content-Type, Content-Length"
   ```
3. Create `.env.prod` from the example and fill values:
   ```bash
   cp .env.prod.example .env.prod
   nano .env.prod
   ```

## 3) Optional: Nginx certs
If you want HTTPS with the optional nginx container, place certs in:
```
/opt/kub/deploy/certs/fullchain.pem
/opt/kub/deploy/certs/privkey.pem
```

## 4) Start the stack
### With local Postgres (profile `db`)
```bash
docker compose -f docker-compose.prod.yml --profile db up -d --build
```

### With external Postgres
1. Set `DATABASE_URL` in `.env.prod` to the external DB.
2. Start only the API:
   ```bash
   docker compose -f docker-compose.prod.yml up -d --build api
   ```
3. Run migrations against the external DB:
   ```bash
   docker compose -f docker-compose.prod.yml run --rm migrate
   ```

### Optional services
```bash
# Redis
docker compose -f docker-compose.prod.yml --profile redis up -d

# Nginx (HTTP/HTTPS proxy)
docker compose -f docker-compose.prod.yml --profile nginx up -d
```

## 5) Verify health
```bash
curl -f http://localhost:4000/healthz
```

## 6) Telegram webhook (if enabled)
If `telegram.webhook_url` is set in config, the API will attempt to set it on boot.
You can re-trigger by restarting the API:
```bash
docker compose -f docker-compose.prod.yml restart api
```
