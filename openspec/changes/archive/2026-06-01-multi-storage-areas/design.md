## Context

The current system assumes a single segments root for both transcode output placement and segment serving. This creates a storage hot spot and limits operational flexibility when transcode output grows. The target behavior is a storage-aware backend that can choose from multiple admin-managed storage areas, while giving admins visibility into utilization and path health from the UI.

Constraints and system realities:
- Storage decisions happen in a transcode worker path that must remain robust under disk/path failures.
- Segment playback URLs are stable API routes (`/stream/{id}/...`) and should remain independent of specific mount roots.
- Existing admin settings and sidebar navigation patterns should be reused where possible.
- This change can be breaking because deployments are test-only and do not require compatibility retention.

## Goals / Non-Goals

**Goals:**
- Introduce a normalized `storage_areas` model with enabled/disabled state and type (`segments`, `thumbnails`).
- Select the transcode output area by maximum free raw bytes among eligible segment areas.
- Enforce configurable reserve headroom (`storage_min_free_bytes`, default 20 GiB).
- Skip missing/unwritable/unavailable paths without failing the entire worker when alternatives exist.
- Add admin storage APIs and a Storage UI page nested under Admin navigation.
- Show per-area utilization plus thumbnail storage utilization on the Storage page.
- Ensure segment serving resolves from persisted media output location (`mpd_path`) rather than a global segments root.

**Non-Goals:**
- Automatic rebalancing/migration of already transcoded media between storage areas.
- Multi-area thumbnail writing behavior (thumbnail writes remain single-area for this change).
- Cross-host distributed placement or cluster-wide storage orchestration.
- Background health daemons beyond request-time / job-time path probing.

## Decisions

1. Storage areas use a dedicated table, not settings JSON.
- Decision: Add `storage_areas` table (`id`, `kind`, `path`, `enabled`, timestamps) and manage via dedicated APIs.
- Rationale: Enables lifecycle actions and future metadata without overloading key/value settings.
- Alternatives considered:
  - JSON array in settings: simpler short-term but poor queryability and evolvability.
  - One table per kind: unnecessary duplication.

2. Keep reserve headroom as a setting, not per-area property.
- Decision: Add `storage_min_free_bytes` in settings with default `21474836480`.
- Rationale: One global policy is easy to reason about and satisfy current requirement.
- Alternatives considered:
  - Per-area reserve: more flexible but adds complexity and UI burden now.

3. Eligibility-first then max-free selection for transcode output.
- Decision: For each enabled `segments` area, require successful statfs, existing path, writable check, and `free_bytes > storage_min_free_bytes`; choose candidate with greatest free bytes.
- Rationale: Matches requested semantics and safely avoids bad targets.
- Alternatives considered:
  - Percentage-based selection: rejected (requirement is raw bytes).
  - Weighted round-robin: smoother spread but not required and less deterministic.

4. Segment serving derives from media `mpd_path`.
- Decision: Resolve segment file path relative to directory containing the persisted `mpd_path` for the media item.
- Rationale: Playback correctness across multiple roots and future storage relocation flexibility.
- Alternatives considered:
  - Continue global `segmentsDir`: breaks for non-default roots.
  - Persist separate `segments_root` column: redundant if `mpd_path` is authoritative.

5. Storage management gets dedicated admin endpoints and page.
- Decision: Add `/api/v1/admin/storage` surface for list/create/update/config and add `/admin/storage` page under Admin subnav.
- Rationale: Separates operational storage concerns from generic settings editing; provides richer typed responses.
- Alternatives considered:
  - Reuse generic `/admin/settings`: weak typing and poor fit for list-based resources.

6. Bootstrap defaults through migrations/bootstrap path.
- Decision: On bootstrap, ensure at least one default segments area (`/data/segments`) and one default thumbnails area (`/data/thumbs`) exist when table is empty.
- Rationale: Predictable startup behavior and clean migration for fresh/test deployments.
- Alternatives considered:
  - Require manual area creation before startup: more fragile initial UX.

## Risks / Trade-offs

- [No eligible segment area available] -> Mitigation: fail job with actionable error; surface area health and reserve diagnostics in storage API/UI.
- [Writable check differences across environments] -> Mitigation: use conservative writeability probe and clear status fields (`missing`, `permission_denied`, `stat_error`) in API.
- [Additional DB/API complexity] -> Mitigation: isolate storage-area CRUD/store logic and keep API contracts narrow.
- [Potential confusion between settings and storage APIs] -> Mitigation: move storage path management exclusively to storage endpoints/page; reserve settings for scalar runtime config.
- [Transient free-space race between selection and write] -> Mitigation: treat as operational race; ffmpeg failure propagates to job failure with clear error.

## Migration Plan

1. Add migration creating `storage_areas` and any supporting index/constraints.
2. Add/bootstrap `storage_min_free_bytes` default setting (`21474836480`).
3. Seed default rows when absent:
- `segments:/data/segments enabled=true`
- `thumbnails:/data/thumbs enabled=true`
4. Update runtime settings loading and admin APIs/UI to use new model.
5. Update transcode worker placement and stream segment resolution.
6. Rollout test plan:
- Single-area baseline behavior remains valid.
- Multi-area selection chooses largest free bytes.
- Unavailable/unwritable/disabled/below-reserve areas are skipped.

Rollback strategy (test deployments):
- Revert binary and database to previous snapshot. Since this is explicitly breaking and test-only, no in-place compatibility downgrade path is required.

## Open Questions

- Should disabled areas be hidden by default in UI lists or always shown with state badges?
- Should the writable check perform only metadata permission checks or attempt temporary-file create/delete in each area?
- Do we need explicit API fields for both `available_bytes` and `free_bytes` semantics from statfs to avoid ambiguity across filesystems?
