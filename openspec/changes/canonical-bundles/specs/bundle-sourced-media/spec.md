# bundle-sourced-media Specification

## Purpose
Allow media items to exist and remain playable when backed solely by a transcode bundle, even when the original source file is no longer present on disk.

## Requirements

### Requirement: Media items track source and bundle availability independently
The system SHALL track source file availability and transcode bundle availability as independent status fields on each media item.

#### Scenario: New media item has source available and no bundle
- **WHEN** a media item is created from a discovered source file that has not been transcoded
- **THEN** the media item has source status set to available and bundle status set to none

#### Scenario: Transcoded media item has both source and bundle available
- **WHEN** a transcode job completes successfully for a media item
- **THEN** the media item has bundle status set to available

#### Scenario: Source file removed but bundle exists
- **WHEN** the scanner detects that a media item's source file no longer exists on disk
- **AND** the media item has bundle status of available
- **THEN** the scanner sets source status to missing and does not delete the media item record

### Requirement: Scanner preserves bundle-backed items during pruning
The scanner SHALL NOT delete media item records that have an available transcode bundle, even when the source file is no longer on disk.

#### Scenario: Prune deletes items with no source and no bundle
- **WHEN** the scanner prunes stale media items after a full library scan
- **AND** a media item's source file is not found and its bundle status is none
- **THEN** the media item record is deleted

#### Scenario: Prune preserves items with bundle but no source
- **WHEN** the scanner prunes stale media items after a full library scan
- **AND** a media item's source file is not found but its bundle status is available
- **THEN** the media item record is preserved with source status set to missing

### Requirement: Indexer creates media items from bundle sidecar metadata
The artifact indexer SHALL create new media item records from bundle sidecar metadata when no matching media item exists in the database.

#### Scenario: Indexer creates media item from v2 sidecar with no DB match
- **WHEN** the indexer discovers a transcode bundle with a v2 sidecar
- **AND** no media item in the database has a matching source fingerprint
- **THEN** the indexer creates a new media item record using the sidecar's metadata with source status set to missing, bundle status set to available, and transcode status set to done

#### Scenario: Indexer creates TV episode with hierarchy from sidecar
- **WHEN** the indexer discovers a transcode bundle whose v2 sidecar specifies media type episode with show name, season number, and episode number
- **AND** no media item in the database has a matching source fingerprint
- **THEN** the indexer upserts the parent TV show and TV season records and creates the episode media item linked to them

#### Scenario: Indexer skips v1 sidecars for media creation
- **WHEN** the indexer discovers a transcode bundle with a v1 sidecar that lacks the enriched metadata fields
- **THEN** the indexer does not create a new media item record from that sidecar but continues to register the artifact record as before

#### Scenario: Indexer skips bundles that already match a media item
- **WHEN** the indexer discovers a transcode bundle whose fingerprint matches an existing media item
- **THEN** the indexer does not create a duplicate media item record

### Requirement: Bundle-only items remain playable
Media items backed by a transcode bundle SHALL remain fully playable regardless of source file availability.

#### Scenario: Bundle-only item serves streaming manifest
- **WHEN** a client requests the streaming manifest for a media item with source status missing and bundle status available
- **THEN** the system serves the DASH manifest from the bundle's MPD path

#### Scenario: Bundle-only item appears in library browsing
- **WHEN** a user browses a library
- **THEN** media items with source status missing and bundle status available are included in the listing with their full metadata
