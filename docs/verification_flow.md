# Регистрация и подтверждение email (manual test)

## Шаги

1) **Регистрация**
```bash
curl -X POST http://localhost:4000/register \
  -H "Content-Type: application/json" \
  -d '{"company_name":"Acme","bin_iin":"123","email":"sales1@example.com","password":"sales12345","phone":"+77000000000"}'
```

2) **Подтверждение**

- В DEV режиме код можно взять из логов (`[DEV][email][verify]`).
```bash
curl -X POST http://localhost:4000/register/confirm \
  -H "Content-Type: application/json" \
  -d '{"user_id":1,"code":"123456"}'
```

3) **Повторное подтверждение тем же кодом**
```bash
curl -X POST http://localhost:4000/register/confirm \
  -H "Content-Type: application/json" \
  -d '{"user_id":1,"code":"123456"}'
```
Ожидаемый ответ: `No pending verification` или `Already verified`.

4) **Resend**
```bash
curl -X POST http://localhost:4000/register/resend \
  -H "Content-Type: application/json" \
  -d '{"user_id":1}'
```

5) **Проверка старого кода**
```bash
curl -X POST http://localhost:4000/register/confirm \
  -H "Content-Type: application/json" \
  -d '{"user_id":1,"code":"старый_код"}'
```
Ожидаемый ответ: `Invalid code`.
