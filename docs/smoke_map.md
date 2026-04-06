# End-to-end smoke map

| Scenario | Endpoints | Required role | Dependencies | Expected result |
|---|---|---|---|---|
| A. Register -> confirm -> login | `POST /register`, `POST /register/confirm`, `POST /auth/login` | Public | DB, verification storage, email code in dev logs | User becomes verified and can login |
| B. Create individual client | `POST /clients`, `GET /clients/:id` | leadership/operations/sales/backoffice (control = read-only, system_admin = denied by business policy) | JWT, clients tables | 201 + client with `individual_profile` |
| C. Create legal client | `POST /clients`, `GET /clients/company` | Same as above | JWT, `client_legal_profiles` | legal client listed with display name |
| D. Create lead/deal with client | `POST /leads`, conversion endpoints, `POST/PUT /deals`, `GET /deals/:id` | Role per lead/deal policy | clients, leads, deals tables | typed ref valid (`client_id` + `client_type`) |
| E. Document sign flow | document create + sign session endpoints (`/api/v1/sign/sessions/*`) | Authenticated + sign participants | documents, sign sessions, config TTL/TZ | successful sign + status transition |
| F. Chat message flow | create chat endpoints + `/chats/:id/messages` + websocket stream | roles allowed by chat authz | chat tables + realtime hub | message visible with sender profile |
| G. Wazzup outbound | `POST /integrations/wazzup/setup`, `POST /integrations/wazzup/send` | `system_admin` for setup, authenticated for send | Wazzup config, repo integration row, provider API | setup/send returns success or controlled provider error |
