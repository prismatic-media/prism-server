## MODIFIED Requirements

### Requirement: Auto-transcode on discovery can be enabled via settings
The system SHALL support a configurable setting `auto_transcode_on_discovery` with valid values `"true"` and `"false"` (default `"false"`). When `"true"`, the system SHALL automatically enqueue a transcode job for each newly discovered media item that has no existing transcode job and no existing transcode bundle.

#### Scenario: Auto-transcode is disabled by default
- **WHEN** the server starts for the first time
- **THEN** the `auto_transcode_on_discovery` setting has value `"false"` and no automatic enqueueing occurs on media discovery

#### Scenario: New item auto-enqueued when setting is enabled
- **WHEN** `auto_transcode_on_discovery` is `"true"` and a new media item is discovered by the scanner
- **AND** the media item has no existing transcode job (`transcode_status` is empty/unset)
- **AND** the media item has no existing transcode bundle (`bundle_status` is not `available`)
- **THEN** a transcode job is automatically created for the item with `priority = 0`

#### Scenario: Already-queued item is not re-enqueued
- **WHEN** `auto_transcode_on_discovery` is `"true"` and a media item is re-discovered (e.g., on re-scan)
- **AND** the item already has a transcode job (status is `pending`, `processing`, `done`, or `failed`)
- **THEN** no new transcode job is created

#### Scenario: Auto-transcode disabled — no job created on discovery
- **WHEN** `auto_transcode_on_discovery` is `"false"` and a new media item is discovered
- **THEN** no transcode job is automatically created

#### Scenario: Bundle-linked item not auto-enqueued
- **WHEN** `auto_transcode_on_discovery` is `"true"` and a media item is discovered that was linked to an existing bundle via fingerprint matching
- **THEN** no transcode job is created because the item already has transcode status done
