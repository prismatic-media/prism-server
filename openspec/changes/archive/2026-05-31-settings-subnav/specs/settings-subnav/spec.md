## ADDED Requirements

### Requirement: Server Settings appears in sidebar navigation

The sidebar navigation SHALL include a "Server Settings" link that is persistently visible to admin users. It SHALL be visually subordinate to the "Admin" item (indented) to communicate hierarchy.

#### Scenario: Server Settings link is visible in the sidebar

- **WHEN** an authenticated admin views any page in the application
- **THEN** a "Server Settings" link is visible in the sidebar beneath the "Admin" link

#### Scenario: Server Settings link navigates to the settings page

- **WHEN** a user clicks the "Server Settings" link in the sidebar
- **THEN** the application navigates to `/admin/settings`

#### Scenario: Server Settings link shows active state on settings route

- **WHEN** the user is on the `/admin/settings` route
- **THEN** the "Server Settings" sidebar link is highlighted as active

#### Scenario: Sidebar closes on mobile after clicking Server Settings

- **WHEN** the user is on a mobile viewport and clicks the "Server Settings" link
- **THEN** the mobile sidebar overlay closes
