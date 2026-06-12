

## Purpose
Define the server and admin UI settings behavior, including defaults, persistence, and admin-only management surfaces.
## Requirements
### Requirement: Settings table exists with defaults
The system SHALL create a `settings` table in the SQLite database during migration. On first startup after migration, the system SHALL bootstrap the table with all default values for every known setting key if those keys do not already exist.

#### Scenario: Fresh database bootstrapped with defaults
- **WHEN** the server starts against a database that has no rows in the `settings` table
- **THEN** all known settings keys are inserted with their default values, including `storage_min_free_bytes = "21474836480"`

#### Scenario: Existing settings are not overwritten
- **WHEN** the server starts against a database that already has settings rows
- **THEN** existing values are preserved and no rows are overwritten

### Requirement: JWT secret auto-generated and persisted
The system SHALL auto-generate a cryptographically random JWT secret (minimum 32 bytes of entropy) on first startup and persist it to the `settings` table under the key `jwt_secret`. The system SHALL use this persisted value for all JWT signing and verification.

#### Scenario: JWT secret generated on first run
- **WHEN** the server starts and no `jwt_secret` exists in the settings table
- **THEN** a cryptographically random secret is generated, written to the `settings` table, and used for JWT operations

#### Scenario: JWT secret reused across restarts
- **WHEN** the server starts and a `jwt_secret` already exists in the settings table
- **THEN** the existing value is used without modification

#### Scenario: JWT secret never returned by any API
- **WHEN** an admin requests GET `/api/v1/admin/settings`
- **THEN** the response does NOT include the `jwt_secret` key

---

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

### Requirement: Settings available as admin UI page
The Angular application SHALL provide an admin settings page where an authenticated admin can view and edit all user-configurable settings. The page SHALL be accessible only to admin users.

#### Scenario: Admin navigates to settings page
- **WHEN** an authenticated admin navigates to the admin settings page
- **THEN** all current setting values are displayed in an editable form

#### Scenario: Admin saves changes
- **WHEN** an admin edits settings and submits the form
- **THEN** the new values are persisted via the API, and a toast/banner informs the admin that a server restart is required for changes to take effect

#### Scenario: Non-admin cannot access settings page
- **WHEN** a non-admin user attempts to navigate to the admin settings page
- **THEN** they are redirected away (e.g., to home)

### Requirement: auto_transcode_on_discovery setting exists
The system SHALL expose `auto_transcode_on_discovery` as a configurable setting with valid string values `"true"` and `"false"`. The default value SHALL be `"false"`.

#### Scenario: Setting bootstrapped with default
- **WHEN** the server starts against a fresh database
- **THEN** `auto_transcode_on_discovery` is initialized to `"false"`

#### Scenario: Setting readable and writable via settings API
- **WHEN** an admin reads settings via `GET /api/v1/admin/settings`
- **THEN** `auto_transcode_on_discovery` is included in the response

#### Scenario: Setting accepts valid values
- **WHEN** an admin sends `PATCH /api/v1/admin/settings` with `{ "auto_transcode_on_discovery": "true" }`
- **THEN** the setting is updated and subsequent reads return `"true"`

---

### Requirement: transcode_poll_interval setting exists
The system SHALL expose `transcode_poll_interval` as a configurable setting representing the number of seconds idle workers sleep between DB polls. The default value SHALL be `"15"`.

#### Scenario: Setting bootstrapped with default
- **WHEN** the server starts against a fresh database
- **THEN** `transcode_poll_interval` is initialized to `"15"`

#### Scenario: Setting readable and writable via settings API
- **WHEN** an admin reads settings via `GET /api/v1/admin/settings`
- **THEN** `transcode_poll_interval` is included in the response

#### Scenario: Workers use the configured interval
- **WHEN** `transcode_poll_interval` is set to `"5"`
- **AND** a worker finds no pending jobs
- **THEN** the worker sleeps approximately 5 seconds before polling again

