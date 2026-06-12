## MODIFIED Requirements

### Requirement: Transcode completion writes durable artifact metadata
The system SHALL write durable artifact metadata for each successful transcode bundle. The sidecar SHALL include comprehensive media metadata sufficient to reconstruct a full media item record.

#### Scenario: Write metadata sidecar on successful transcode
- **WHEN** a transcode job completes successfully
- **THEN** the output bundle includes artifact metadata that records source identity, artifact provenance, and full media metadata including title, year, TMDB ID, overview, media type, codecs, resolution, file size, and TV episode hierarchy fields

#### Scenario: Metadata includes source fingerprint
- **WHEN** artifact metadata is written
- **THEN** it includes the deterministic source fingerprint used for future relinking

#### Scenario: Sidecar includes poster image reference
- **WHEN** artifact metadata is written and the media item has a poster or still image
- **THEN** the poster image is copied into the bundle directory and the sidecar records the poster filename

#### Scenario: Sidecar written as version 2
- **WHEN** artifact metadata is written by the current system version
- **THEN** the sidecar version field is set to 2

## ADDED Requirements

### Requirement: Sidecar metadata is updated when media metadata changes
The system SHALL update the on-disk sidecar metadata when media item metadata is modified through enrichment or refresh operations.

#### Scenario: Sidecar updated after TMDB enrichment
- **WHEN** TMDB metadata enrichment completes successfully for a media item that has an existing transcode bundle
- **THEN** the sidecar in the bundle directory is updated with the new title, year, TMDB ID, overview, and poster filename, and the poster image is copied into the bundle directory

#### Scenario: Sidecar not updated when no bundle exists
- **WHEN** TMDB metadata enrichment completes for a media item with no transcode bundle
- **THEN** no sidecar update is attempted

#### Scenario: Sidecar update failure does not block enrichment
- **WHEN** the sidecar file cannot be written during a metadata update
- **THEN** the enrichment operation still succeeds and the failure is logged

### Requirement: System reads both v1 and v2 sidecars
The system SHALL accept and correctly parse sidecar metadata in both v1 and v2 formats.

#### Scenario: V1 sidecar read successfully
- **WHEN** the system reads a v1 sidecar that lacks enriched metadata fields
- **THEN** the sidecar is parsed successfully with enriched fields treated as absent

#### Scenario: V2 sidecar read successfully
- **WHEN** the system reads a v2 sidecar with full media metadata
- **THEN** all fields including enriched metadata are parsed and available
