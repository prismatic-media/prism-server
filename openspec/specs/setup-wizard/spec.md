### Requirement: Fresh install detected via setup_complete setting

The system SHALL check the `setup_complete` setting on every request. If the value is not `"true"`, the system SHALL treat the server as not yet configured.

#### Scenario: Setup not complete blocks normal access

- **WHEN** a request is made to any route other than `/setup` or `/api/v1/setup/*` and `setup_complete` is not `"true"`
- **THEN** the server returns a redirect or 503 response directing the client to `/setup`

#### Scenario: Setup routes are always accessible

- **WHEN** a request is made to `/setup` or any `/api/v1/setup/*` endpoint
- **THEN** the request is processed regardless of `setup_complete` value

#### Scenario: Normal access after setup complete

- **WHEN** `setup_complete` is `"true"` and a request is made to any route
- **THEN** the request proceeds normally with no setup-related gating

---

### Requirement: Setup wizard creates initial admin account

The system SHALL provide a `POST /api/v1/setup` endpoint that accepts a username and password, creates the first admin user, and marks `setup_complete = "true"` in the settings table. This endpoint SHALL only be callable when `setup_complete` is not `"true"`.

#### Scenario: Successful wizard completion

- **WHEN** POST `/api/v1/setup` is called with a valid username and password and setup is not yet complete
- **THEN** the admin user is created, `setup_complete` is set to `"true"`, and the response is 201

#### Scenario: Setup endpoint disabled after completion

- **WHEN** POST `/api/v1/setup` is called after `setup_complete` is already `"true"`
- **THEN** the response is 409 Conflict (or 403)

#### Scenario: Invalid credentials rejected

- **WHEN** POST `/api/v1/setup` is called with a missing or empty username or password
- **THEN** the response is 400 Bad Request

---

### Requirement: Setup wizard UI shown on first visit

The Angular application SHALL display a setup wizard when the server has not been configured (detected via a 503/redirect from the server, or by calling a status endpoint). The wizard SHALL collect the admin username and password and submit them to `POST /api/v1/setup`.

#### Scenario: User visits app before setup

- **WHEN** a user navigates to the application root before setup is complete
- **THEN** they are shown the setup wizard UI, not the normal login screen

#### Scenario: Wizard redirects to login after completion

- **WHEN** the user completes the setup wizard successfully
- **THEN** they are redirected to the login page (or auto-logged in)

#### Scenario: Normal login shown after setup

- **WHEN** a user navigates to the application root after setup is complete
- **THEN** the normal login screen is displayed
