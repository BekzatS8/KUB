# Postman: KUB API

## Импорт
1. Импортируйте коллекцию: `postman/KUB API.postman_collection.json`.
2. Импортируйте окружение: `postman/KUB Local.postman_environment.json`.
3. Выберите окружение **KUB Local**.
4. Проверьте `baseUrl = http://localhost:4000`.

## Основные переменные (smoke)

### Users / auth
- `adminEmail`, `adminPassword`
- `salesEmail`, `salesPassword`
- `operationsEmail`, `operationsPassword`
- `controlEmail`, `controlPassword`
- `salesRoleId`, `operationsRoleId`, `controlRoleId`
- `createdUserId`, `createdUserEmail`, `salesUserId`, `operationsUserId`, `controlUserId`

### Typed clients
- `individualClientId` — основной ID для `client_type=individual`
- `legalClientId` — основной ID для `client_type=legal`
- `clientId`, `client_id` — **DEPRECATED aliases** (оставлены для обратной совместимости, не использовать в typed smoke)

### Individual profile
- `individualFullName`, `individualLastName`, `individualFirstName`, `individualMiddleName`
- `individualIin`, `individualIdNumber`, `individualPassportSeries`, `individualPassportNumber`
- `individualPhone`, `individualEmail`
- `individualCountry`, `individualTripPurpose`, `individualBirthDate`
- `individualRegistrationAddress`, `individualActualAddress`, `individualContactInfo`

### Legal profile
- `legalCompanyName`, `legalBin`, `legalAddress`
- `contactPersonName`, `contactPersonPhone`, `contactPersonEmail`
- `signerFullName`, `signerPosition`

### Deals / leads / chat
- `leadId`, `individualLeadId`, `legalLeadId`
- `dealId`, `dealAmount`, `dealCurrency`, `dealStatus`
- `chatQuery`, `chatTargetUserId`, `chatId`

## POST /users: privileged verified flow
- Для privileged `POST /users` поддерживается optional поле `is_verified`.
- Если `is_verified=true`, пользователь создаётся сразу verified.
- Это относится **только** к privileged flow `POST /users`.
- Публичный `POST /register` не меняется и не используется для auto-verified сценария.

В коллекции добавлены отдельные запросы:
- `Users / Create Sales User (verified)`
- `Users / Create Operations User (verified)`
- `Users / Create Control User (verified)`
- `Users / Create System Admin User (verified)` (используйте только если ваша policy это допускает)

Если в контуре запрещено создавать `system_admin` через API, используйте только sales/operations/control.

## Typed client contract (без путаницы)
Используйте только typed ссылки:
- `individualClientId` + `client_type=individual`
- `legalClientId` + `client_type=legal`

Контракт:
- `POST /deals` требует `client_id + client_type`
- `PUT /deals/:id` требует `client_id + client_type`
- `PUT /leads/:id/convert` требует `client_id + client_type`
- `POST /documents/create-from-client` требует `client_id + client_type`

## Актуальный chat flow
Используйте:
1. `GET /chats/users?query={{chatQuery}}&limit=20&offset=0`
2. `POST /chats/personal` с body:
   ```json
   {
     "user_id": {{chatTargetUserId}}
   }
   ```
3. `POST /chats/:id/messages`
4. `GET /chats`
5. `GET /chats/search`
6. `GET /chats/:id/info`

Почему так:
- chat picker теперь через `GET /chats/users`;
- фронту больше не нужен privileged `/users` для picker;
- для personal chat использовать `counterparty`;
- для group chat использовать `participants_preview` и `member_profiles`;
- в directory ответе использовать `existing_personal_chat_id`;
- в сообщениях использовать `sender_profile`.

## Ручные smoke sequences

### 1) Verified user smoke
1. `Auth / Login as Admin`
2. `Users / Create Sales User (verified)`
3. `Auth / Login as Sales`

### 2) Individual client + deal smoke
1. `Clients / Individual / Create Individual Client (full flat)`
2. Проверить `individualClientId`
3. `Deals / Create Deal for Individual`

### 3) Legal client + deal smoke
1. `Clients / Legal / Create Legal Client (full legal_profile)`
2. Проверить `legalClientId`
3. `Deals / Create Deal for Legal`

### 4) Chat smoke
1. `Auth / Login as Sales`
2. `Chats / Chat Users Directory`
3. Проверить `chatTargetUserId`
4. `Chats / Create Personal Chat`
5. `Chats / Send Message`
6. `Chats / List Chats`
7. `Chats / Search Chats`
8. `Chats / Get Chat Info`

### 5) Control/read-only chat smoke
1. `Auth / Login as Control`
2. `Chats / Chat Users Directory`
3. `Chats / Create Personal Chat`
4. `Chats / Send Message`
5. `Chats / Mark Read`
6. Проверить, что бизнес write endpoint (`POST /clients` или `PATCH /clients/:id`) остаётся запрещён для control.

## Archive/RBAC модель (business entities)

Новая модель покрывает только business entities:
- `leads`
- `deals`
- `clients`
- `documents`
- `tasks`

Ключевые правила:
- `users` и `roles` **не** входят в archive scope;
- hard delete (`DELETE`) для business entities — только для `role_id=50` (`system_admin`);
- для business ролей обычный lifecycle: `archive/unarchive` через явные endpoints;
- list endpoints по умолчанию должны работать как `archive=active`;
- для smoke добавлены фильтры `archive=archived` и `archive=all`.

## Что добавлено в коллекцию (archive model)

### Archive / Unarchive endpoints
- Leads: `POST /leads/:id/archive`, `POST /leads/:id/unarchive`
- Deals: `POST /deals/:id/archive`, `POST /deals/:id/unarchive`
- Clients: `POST /clients/:id/archive`, `POST /clients/:id/unarchive`
- Documents: `POST /documents/:id/archive`, `POST /documents/:id/unarchive`
- Tasks: `POST /tasks/:id/archive`, `POST /tasks/:id/unarchive`

### Archive list/filter smoke requests
- Leads: `GET /leads?archive=archived|all`, `GET /leads/my?archive=archived|all`
- Deals: `GET /deals?archive=archived|all`, `GET /deals/my?archive=archived|all`
- Clients:
  - `GET /clients?archive=archived|all`
  - `GET /clients/my?archive=archived|all`
  - `GET /clients/individual?...&archive=archived|all`
  - `GET /clients/company?...&archive=archived|all`
- Documents:
  - `GET /documents?archive=archived|all`
  - `GET /documents/deal/:dealid?archive=archived|all`
- Tasks: `GET /tasks?archive=archived|all`

## Archive smoke flow (быстрый прогон)

В коллекции добавлена папка **Archive Smoke / Business Entities** с цепочками:
- Leads: create → list active → archive → list archived → list all → unarchive → delete as admin
- Deals: create → list active → archive → list archived → list all → unarchive → delete as admin
- Clients: create individual → create legal → archive → list archived → unarchive → delete as admin
- Documents: create → archive → list archived → unarchive → delete as admin
- Tasks: create → archive → list archived → unarchive → delete as admin

## Negative RBAC checks

Добавлена папка **RBAC / Negative checks**:
- non-admin DELETE для `lead/deal/client/document/task` → ожидается `403`;
- read-only (`control`) archive для `lead/deal/client/document/task` → ожидается `403`;
- admin (`system_admin`) DELETE для `lead/deal/client/document/task` → success path (`200/204`).

## Environment переменные для archive smoke

Добавлены/используются:
- `archivedLeadId`
- `archivedDealId`
- `archivedClientId`
- `archivedDocumentId`
- `archivedTaskId`

Сохранены основные рабочие переменные:
- `leadId`, `dealId`, `individualClientId`, `legalClientId`, `documentId`, `taskId`.

## Примеры payload

### Create Sales User (verified)
```json
{
  "company_name": "KUB Sales Team",
  "bin_iin": "111111111111",
  "email": "{{salesEmail}}",
  "password": "{{salesPassword}}",
  "phone": "+77010000001",
  "role_id": {{salesRoleId}},
  "is_verified": true
}
```

### Create Operations User (verified)
```json
{
  "company_name": "KUB Operations Team",
  "bin_iin": "222222222222",
  "email": "{{operationsEmail}}",
  "password": "{{operationsPassword}}",
  "phone": "+77010000002",
  "role_id": {{operationsRoleId}},
  "is_verified": true
}
```

### Create Control User (verified)
```json
{
  "company_name": "KUB Control Team",
  "bin_iin": "333333333333",
  "email": "{{controlEmail}}",
  "password": "{{controlPassword}}",
  "phone": "+77010000003",
  "role_id": {{controlRoleId}},
  "is_verified": true
}
```

### Create Individual Client (flat)
```json
{
  "client_type": "individual",
  "name": "Petrova Marina Olegovna",
  "bin_iin": "950312400321",
  "address": "Astana, Mangilik El 32, apt 45",
  "contact_info": "Telegram: @marina_petrova, WhatsApp: +77015554433",
  "last_name": "Petrova",
  "first_name": "Marina",
  "middle_name": "Olegovna",
  "iin": "950312400321",
  "id_number": "ID987654321",
  "passport_series": "N",
  "passport_number": "1234567",
  "phone": "+77015554433",
  "email": "marina.petrova@example.com",
  "registration_address": "Astana, Respublika 15, apt 12",
  "actual_address": "Astana, Mangilik El 32, apt 45",
  "country": "Italy",
  "trip_purpose": "tourism",
  "birth_date": "1995-03-12"
}
```

### Create Legal Client (full)
```json
{
  "client_type": "legal",
  "name": "TOO Silk Road Travel",
  "bin_iin": "240540012345",
  "phone": "+77015550011",
  "email": "office@silkroadtravel.kz",
  "address": "Astana, Kabanbay Batyr 42, office 301",
  "actual_address": "Astana, Kabanbay Batyr 42, office 301",
  "registration_address": "Astana, Kabanbay Batyr 42, office 301",
  "contact_info": "Main contact: Dana Mukasheva, WhatsApp +77015550011, working hours 09:00-18:00",
  "legal_profile": {
    "company_name": "TOO Silk Road Travel",
    "bin": "240540012345",
    "legal_form": "TOO",
    "director_full_name": "Dana Mukasheva",
    "contact_person_name": "Dana Mukasheva",
    "contact_person_position": "Director",
    "contact_person_phone": "+77015550011",
    "contact_person_email": "office@silkroadtravel.kz",
    "legal_address": "Astana, Kabanbay Batyr 42, office 301",
    "actual_address": "Astana, Kabanbay Batyr 42, office 301",
    "bank_name": "Kaspi Bank",
    "iban": "KZ689261501234567890",
    "bik": "CASPKZKA",
    "kbe": "17",
    "tax_regime": "general",
    "website": "https://silkroadtravel.kz",
    "industry": "travel",
    "company_size": "11-50",
    "additional_info": "Corporate visa support, outbound tourism, B2B partner network"
  }
}
```

### Create Deal for Individual
```json
{
  "lead_id": {{individualLeadId}},
  "client_id": {{individualClientId}},
  "client_type": "individual",
  "amount": {{dealAmount}},
  "currency": "{{dealCurrency}}",
  "status": "{{dealStatus}}"
}
```

### Create Deal for Legal
```json
{
  "lead_id": {{legalLeadId}},
  "client_id": {{legalClientId}},
  "client_type": "legal",
  "amount": {{dealAmount}},
  "currency": "{{dealCurrency}}",
  "status": "{{dealStatus}}"
}
```

### Documents create-from-client (individual)
```json
{
  "client_id": {{individualClientId}},
  "client_type": "individual",
  "deal_id": {{dealId}},
  "doc_type": "addendum_korea",
  "extra": {}
}
```

### Documents create-from-client (legal)
```json
{
  "client_id": {{legalClientId}},
  "client_type": "legal",
  "deal_id": {{dealId}},
  "doc_type": "addendum_korea",
  "extra": {}
}
```
