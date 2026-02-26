# PATCH /clients/:id examples

## Update only email
```http
PATCH /clients/123
Content-Type: application/json

{"email":"new.email@example.com"}
```

## Update only birth_date (supported formats)
```json
{"birth_date":"2024-01-31"}
```

```json
{"birth_date":"31.01.2024"}
```

```json
{"birth_date":"2024-01-31T10:15:30Z"}
```

## Clear date
```json
{"birth_date":""}
```

## Error examples
- INVALID_EMAIL — when `email` is malformed (including `"{{clientEmail}}"`).
- INVALID_DATE_FORMAT — when date format is not one of: `YYYY-MM-DD`, `DD.MM.YYYY`, `YYYY-MM-DDTHH:MM:SSZ`.
- EMAIL_ALREADY_USED — when another client already uses the same email.
- CLIENT_NOT_FOUND — when client with given id does not exist.
