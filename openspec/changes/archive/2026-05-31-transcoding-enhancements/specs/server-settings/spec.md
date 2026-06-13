## ADDED Requirements

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
