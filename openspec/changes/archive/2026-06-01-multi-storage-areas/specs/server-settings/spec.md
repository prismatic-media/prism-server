## MODIFIED Requirements

### Requirement: Settings table exists with defaults

The system SHALL create a `settings` table in the SQLite database during migration. On first startup after migration, the system SHALL bootstrap the table with all default values for every known setting key if those keys do not already exist.

#### Scenario: Fresh database bootstrapped with defaults

- **WHEN** the server starts against a database that has no rows in the `settings` table
- **THEN** all known settings keys are inserted with their default values, including `storage_min_free_bytes = "21474836480"`

#### Scenario: Existing settings are not overwritten

- **WHEN** the server starts against a database that already has settings rows
- **THEN** existing values are preserved and no rows are overwritten

### Requirement: Admin can read all exposed settings

The system SHALL provide a `GET /api/v1/admin/settings` endpoint that returns all user-configurable scalar settings as a JSON object. This endpoint SHALL require admin authentication.

#### Scenario: Admin fetches settings

- **WHEN** an authenticated admin sends GET `/api/v1/admin/settings`
- **THEN** the response is 200 with a JSON body containing all configurable scalar setting keys and their current values, including `storage_min_free_bytes`

#### Scenario: Storage area paths are not returned as scalar settings

- **WHEN** an authenticated admin sends GET `/api/v1/admin/settings`
- **THEN** storage area path management data is not returned from this endpoint and is available from storage management endpoints

#### Scenario: Non-admin cannot read settings

- **WHEN** an authenticated non-admin user sends GET `/api/v1/admin/settings`
- **THEN** the response is 403 Forbidden

#### Scenario: Unauthenticated request rejected

- **WHEN** an unauthenticated request is made to GET `/api/v1/admin/settings`
- **THEN** the response is 401 Unauthorized

### Requirement: Admin can update settings

The system SHALL provide a `PUT /api/v1/admin/settings` endpoint that accepts a JSON object of setting key-value pairs and persists them to the `settings` table. Only known, user-configurable scalar keys SHALL be accepted. This endpoint SHALL require admin authentication.

#### Scenario: Admin updates a setting

- **WHEN** an authenticated admin sends PUT `/api/v1/admin/settings` with a valid JSON body containing one or more known scalar setting keys
- **THEN** the response is 200, and the new values are persisted to the database

#### Scenario: Storage area paths cannot be updated via settings endpoint

- **WHEN** an authenticated admin sends PUT `/api/v1/admin/settings` with storage area path keys
- **THEN** the response is 400 Bad Request

#### Scenario: Unknown setting keys are rejected

- **WHEN** an admin sends PUT `/api/v1/admin/settings` with an unrecognized key
- **THEN** the response is 400 Bad Request

#### Scenario: Non-admin cannot update settings

- **WHEN** an authenticated non-admin user sends PUT `/api/v1/admin/settings`
- **THEN** the response is 403 Forbidden
