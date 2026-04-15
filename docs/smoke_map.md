# End-to-end smoke map

| Scenario | Endpoints | Required role | Dependencies | Expected result |
|---|---|---|---|---|
| A0. Branch scope baseline | `GET /branches`, `GET /users/me` | any authenticated role | `branches`, `users.branch_id` | user has branch binding; branches list returns seed branches |
| A. Register -> confirm -> login | `POST /register`, `POST /register/confirm`, `POST /auth/login` | Public | DB, verification storage, email code in dev logs | User becomes verified and can login |
| B. Create individual client | `POST /clients`, `GET /clients/:id` | leadership/operations/sales (control = read-only, system_admin = denied by business policy) | JWT, clients tables | 201 + client with `individual_profile` |
| C. Create legal client | `POST /clients`, `GET /clients/company` | Same as above | JWT, `client_legal_profiles` | legal client listed with display name |
| D. Create lead/deal with client | `POST /leads`, conversion endpoints, `POST/PUT /deals`, `GET /deals/:id` | Role per lead/deal policy | clients, leads, deals tables | typed ref valid (`client_id` + `client_type`) |
| D1. Cross-branch protection | `GET/PUT /leads/:id`, `GET/PUT /deals/:id`, list with/without `branch_id` | sales/operations/control/leadership/system_admin | branch_id indexes, RBAC | non-elevated users do not read foreign branch data |
| E. Document sign flow | document create + sign session endpoints (`/api/v1/sign/sessions/*`) | Authenticated + sign participants | documents, sign sessions, config TTL/TZ | successful sign + status transition |
| E1. Document branch inheritance | `POST /documents`, `POST /documents/upload`, `GET /documents` | sales/operations/leadership | deals + documents | created docs carry `branch_id` inherited from deal |
| F1. Chat directory + personal flow (sales) | `GET /chats/users`, `POST /chats/personal`, `POST /chats/:id/messages`, `GET /chats`, `GET /chats/:id/info` | sales | chat tables + realtime hub | `existing_personal_chat_id`, `counterparty`, `sender_profile` present |
| F2. Chat directory + personal flow (operations) | same as F1 + `GET /chats/search` | operations | chat tables + realtime hub | search works by counterparty display/email |
| F3. Control chat policy check | `GET /chats/users`, `GET /chats`, `GET /chats/:id/info`, `POST /chats/personal`, `POST /chats/:id/messages`, `POST /chats/:id/read` | control/read-only | read-only middleware + chat authz | chat writes allowed by narrow exception; business writes still forbidden |
| G. Wazzup outbound | `POST /integrations/wazzup/setup`, `POST /integrations/wazzup/send` | setup: any authenticated known role; send: authenticated | Wazzup config, repo integration row, provider API | setup/send returns success or controlled provider error |
| H. Reports branch filter | `/reports/funnel`, `/reports/leads`, `/reports/revenue` (+ `branch_id`) | sales/operations/control/leadership/system_admin | deals/leads stats + branch scope | elevated roles can filter by branch; non-elevated stay on own branch |
