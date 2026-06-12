## Why

Transcoded artifacts are currently coupled to database state, so database loss or storage path changes can orphan existing manifests and segments. We need a durable way to rediscover, verify, and relink existing transcode outputs so playback can recover without unnecessary retranscoding.

## What Changes

- Add a storage artifact indexing workflow that scans configured segment storage paths and registers discovered transcode bundles.
- Add persisted transcode artifact metadata that records stable source identity and artifact health for each discovered bundle.
- Add relinking behavior that matches discovered artifacts back to current media items after database rebuilds or storage reconfiguration.
- Add explicit handling for unmatched and ambiguous matches so unsafe auto-linking is avoided.
- Add admin-invokable recovery operations for indexing and relinking to support disaster recovery and storage migrations.

## Capabilities

### New Capabilities
- `artifact-indexing`: Discover and register transcode artifact bundles from enabled segment storage areas.
- `artifact-metadata`: Persist and maintain artifact identity and health metadata independent of media row IDs.
- `artifact-relinking`: Relink discovered artifacts to current media items using deterministic matching with safe ambiguity handling.

### Modified Capabilities
- None.

## Impact

- Affected systems: scanner/recovery workflows, transcode output lifecycle, streaming manifest resolution, and admin storage operations.
- Affected code areas: storage and transcode persistence, recovery orchestration, and admin API handlers for storage/recovery controls.
- Data model impact: new artifact-oriented persistence and linkage records.
- Operational impact: adds explicit disaster-recovery and storage-migration playbooks, reducing retranscode load after incidents.
