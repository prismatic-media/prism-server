## Why

Server Settings is only discoverable once a user is already on the Admin panel page — it's a small link in the panel header. Making it a persistent sub-nav item under "Admin" in the sidebar gives it immediate visibility and reduces the clicks needed to reach it.

## What Changes

- Add a "Server Settings" sub-navigation item beneath "Admin" in the sidebar nav
- The item links directly to `/admin/settings`
- It is visually indented to signal it belongs under Admin
- It highlights as active when on the settings route

## Capabilities

### New Capabilities

- `settings-subnav`: A persistent sidebar sub-navigation entry for Server Settings, nested visually under the Admin nav item

### Modified Capabilities

- `server-settings`: The route and page remain unchanged; only its nav entry point is being promoted

## Impact

- `web/src/app/shell/shell.component.ts` — add sub-nav `<li>` for Server Settings
- `web/src/app/shell/shell.component.scss` — add indented sub-item style
- No API changes, no route changes, no auth changes
