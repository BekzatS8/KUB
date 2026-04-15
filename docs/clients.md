# Clients data model (base + typed profiles)

## Why changed
Old `clients` mixed base CRM identity + individual questionnaire + legal/company data in one row. That made legal cards incomplete and validation/search logic inconsistent.

## New canonical model
- `clients` = base CRM entity (`id`, `owner_id`, `client_type`, `display_name`, `primary_phone`, `primary_email`, `address`, `contact_info`, timestamps).
- `client_individual_profiles` = individual-only fields.
- `client_legal_profiles` = legal/company-only fields.

`deals.client_id`, documents/tasks/files and all existing FK references still point to `clients.id`.

Typed contract на уровне API/сервисов:
- write-пути для сделок и конвертации требуют `client_id + client_type`;
- `POST /documents/create-from-client` требует `client_id + client_type`;
- latest deal для документов выбирается по точной typed-ссылке.

## Migration strategy
This step uses safe two-phase compatibility:
1. Add profile tables + base columns + backfill from existing `clients` rows.
2. Switch repository source of truth to joins with new profile tables.
3. Keep old mixed columns in `clients` as deprecated compatibility layer (not dropped yet).

## Required fields
### Individual
- `first_name`
- `last_name`
- `phone`
- `birth_date`
- `country`
- `trip_purpose`

### Legal
- `company_name`
- `bin`
- `contact_person_name`
- `contact_person_phone`
- `legal_address`

## Backward compatibility
- Existing endpoints kept: `/clients`, `/clients/:id`, `/clients/individual`, `/clients/company`.
- Legacy top-level response fields still returned.
- Added nested structures: `individual_profile` and `legal_profile`.
- Для individual clients добавлены optional questionnaire fields (и в flat compatibility, и в `individual_profile`):
  - `specialty`
  - `trusted_person_phone`
  - `driver_license_number`
  - `driver_license_issue_date`
  - `driver_license_expire_date`
  - `education_institution_name`
  - `education_institution_address`
  - `position`
  - `visas_received`
  - `visa_refusals`

## Archive/Delete policy (stage 3)
- Для `clients` по умолчанию list/get работают с активными (`is_archived = false`) записями.
- Для list endpoints поддержан `archive` query filter:
  - empty/`active` — активные,
  - `archived` — архивные,
  - `all` — все.
- Явные действия:
  - `POST /clients/:id/archive`
  - `POST /clients/:id/unarchive`
- `DELETE /clients/:id` выполняет только hard delete и доступен только `role_id=50`.
- Archive клиента не удаляет профили/связанные данные физически и не ломает typed references (`client_id + client_type`).

## Deprecated
Legacy mixed columns in `clients` remain for compatibility and will be removed in a follow-up migration after consumers fully switch.

## Invariants
- `client_type` у существующего клиента immutable (запрещено менять через `PUT/PATCH /clients/:id`).
- Недопустима «тихая конвертация» клиента между `individual` и `legal`.
