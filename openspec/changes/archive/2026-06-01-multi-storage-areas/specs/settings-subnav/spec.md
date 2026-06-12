## MODIFIED Requirements

### Requirement: Server Settings appears in sidebar navigation
The sidebar navigation SHALL include both a "Server Settings" link and a "Storage" link that are persistently visible to admin users. Both links SHALL be visually subordinate to the "Admin" item (indented) to communicate hierarchy.

#### Scenario: Admin subnav links are visible in the sidebar
- **WHEN** an authenticated admin views any page in the application
- **THEN** both "Server Settings" and "Storage" links are visible in the sidebar beneath the "Admin" link

#### Scenario: Server Settings link navigates to the settings page
- **WHEN** a user clicks the "Server Settings" link in the sidebar
- **THEN** the application navigates to `/admin/settings`

#### Scenario: Storage link navigates to the storage page
- **WHEN** a user clicks the "Storage" link in the sidebar
- **THEN** the application navigates to `/admin/storage`

#### Scenario: Server Settings link shows active state on settings route
- **WHEN** the user is on the `/admin/settings` route
- **THEN** the "Server Settings" sidebar link is highlighted as active

#### Scenario: Storage link shows active state on storage route
- **WHEN** the user is on the `/admin/storage` route
- **THEN** the "Storage" sidebar link is highlighted as active

#### Scenario: Sidebar closes on mobile after clicking admin subnav links
- **WHEN** the user is on a mobile viewport and clicks either "Server Settings" or "Storage"
- **THEN** the mobile sidebar overlay closes
