## Context

The sidebar nav in `shell.component.ts` is a flat list of top-level links. Server Settings (`/admin/settings`) currently only appears as a link in the Admin panel's own header — it has no presence in the sidebar. The goal is to surface it without restructuring the nav.

## Goals / Non-Goals

**Goals:**
- Add a "Server Settings" link to the sidebar, visually subordinate to "Admin"
- Use existing `routerLinkActive` mechanics — no new state or logic needed

**Non-Goals:**
- Collapsible/expandable admin sub-menu
- Role-based visibility (both Admin and Server Settings are already guarded at the route level)
- Removing or changing the existing link in the admin panel header

## Decisions

**Inline sub-item via indented `<li>`, not a nested `<ul>`**
A dedicated CSS class (`.subnav-item`) with left padding and a smaller font gives the visual hierarchy without introducing a nested list structure. This keeps the template minimal and the CSS easy to override later if a collapsible menu is ever wanted.

**No separate component**
The shell is a single-component sidebar. Adding one `<li>` and one SCSS rule does not warrant extracting a component.

## Risks / Trade-offs

- If admin grows significantly (user management, audit logs, etc.), this flat approach becomes harder to read → at that point, extract to a collapsible sub-menu. For now, two items is fine.
