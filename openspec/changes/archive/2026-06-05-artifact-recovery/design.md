## Context

Transcode outputs are currently discovered through media row state, especially mpd_path and media-item UUID-derived output directories. This makes the system vulnerable to database loss and storage reconfiguration: artifacts can exist on disk but become unreachable because current media IDs and persisted paths no longer match prior transcode outputs.

The proposal introduces three capabilities that separate artifact identity from current media row identity:

- Artifact indexing across enabled segment storage areas
- Persisted artifact metadata and health tracking
- Deterministic relinking from discovered artifacts to current media rows

Constraints:

- Recovery operations must be safe under partial data and mixed-quality artifacts.
- Recovery must avoid unsafe auto-linking when confidence is ambiguous.
- Existing transcode and streaming workflows should continue to function during rollout.

## Goals / Non-Goals

**Goals:**

- Allow operators to recover playable transcodes after database rebuilds without retranscoding all content.
- Support importing and matching preexisting transcode bundles when a new storage path is added.
- Persist durable artifact metadata independent of ephemeral media row IDs.
- Provide operationally safe recovery behavior with explicit unmatched and ambiguous states.

**Non-Goals:**

- Redesigning the transcoder profile pipeline or DASH packaging format.
- Solving cross-instance global deduplication for all transcoded outputs.
- Replacing existing transcode queue scheduling behavior.
- Introducing fully automatic fallback matching that can silently attach uncertain artifacts.

## Decisions

### Decision 1: Introduce first-class artifact persistence independent of media_items.id

Rationale:

- Current linkage is tightly coupled to media UUID and mpd_path persistence.
- A first-class artifact model allows rediscovery and reattachment after identity churn in media rows.

Alternatives considered:

- Deterministic media IDs from path: rejected because path changes break identity.
- Deterministic media IDs only from content hash: rejected because it still lacks explicit artifact lifecycle and health tracking.

### Decision 2: Use deterministic source fingerprint as primary relink key

Rationale:

- Fingerprint matching is stable across DB rebuilds and storage path migration.
- It provides high-confidence deterministic relinking when present.

Alternatives considered:

- Title and duration heuristic-only matching: rejected as primary because false positives are likely.
- File path matching: rejected because path changes are expected during migration.

### Decision 3: Store self-describing artifact metadata with each transcode bundle

Rationale:

- On-disk metadata allows indexers to recover provenance from storage alone.
- This enables disaster recovery even when DB state is incomplete.

Alternatives considered:

- DB-only artifact metadata: rejected because DB loss removes the source of truth.
- Directory-name-only conventions: rejected because conventions are insufficient for reliable matching and version validation.

### Decision 4: Explicitly model unmatched and ambiguous outcomes

Rationale:

- Recovery operations must prefer safety over aggressive auto-attachment.
- Operators need visibility and control for uncertain mappings.

Alternatives considered:

- Force-best-match behavior: rejected due to risk of incorrect media playback mapping.
- Dropping unmatched artifacts silently: rejected because this hides recoverable content and operational risk.

### Decision 5: Expose admin-invokable indexing and relinking operations

Rationale:

- Recovery timing should be operator-controlled for disaster response and staged migrations.
- Storage additions need immediate operator-invokable import workflows.

Alternatives considered:

- Startup-only automatic repair: rejected because it can increase startup latency and hide operator intent.
- Background-only eventual repair: rejected because disaster recovery needs deterministic completion signals.

## Risks / Trade-offs

- [Fingerprint calculation cost on large libraries] -> Mitigation: support incremental indexing and cached fingerprint state.
- [Legacy bundles missing metadata sidecars] -> Mitigation: classify as low-confidence and require explicit relink policy for heuristic matching.
- [Operator confusion around ambiguous states] -> Mitigation: provide clear status reporting and actionable remediation paths.
- [Schema and workflow complexity increases] -> Mitigation: rollout in phases with additive schema and backward-compatible read paths.
- [Storage scans can be expensive on very large volumes] -> Mitigation: allow scoped scans per storage area and track last-indexed timestamps.

## Migration Plan

1. Add additive persistence for artifact records and artifact-to-media links.
2. Start writing artifact metadata sidecars for newly completed transcodes.
3. Introduce artifact indexing operation for enabled segment storage areas.
4. Introduce deterministic relinking operation with explicit unmatched/ambiguous outputs.
5. Update streaming manifest resolution to use repaired mappings after relink.
6. Run recovery playbook in staging and then production for existing libraries.

Rollback strategy:

- Keep existing mpd_path resolution behavior as fallback during rollout.
- Disable indexing/relink operations if anomalies are detected.
- Revert to prior transcode and streaming behavior without deleting discovered artifact records.

## Open Questions

- What exact fingerprint algorithm and normalization rules should be standard for source identity?
- Should heuristic fallback matching ever be auto-accepted, or always require admin confirmation?
- How should duplicate-source media rows across multiple libraries share or isolate artifact mappings?
- What API and UI affordances are required to resolve ambiguous matches at scale?
- Should indexing run automatically on storage-area create, or remain strictly operator-triggered?