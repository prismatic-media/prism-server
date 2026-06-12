## ADDED Requirements

### Requirement: Transcode completion writes durable artifact metadata
The system SHALL write durable artifact metadata for each successful transcode bundle.

#### Scenario: Write metadata sidecar on successful transcode
- **WHEN** a transcode job completes successfully
- **THEN** the output bundle includes artifact metadata that records source identity and artifact provenance

#### Scenario: Metadata includes source fingerprint
- **WHEN** artifact metadata is written
- **THEN** it includes the deterministic source fingerprint used for future relinking

### Requirement: Artifact metadata is persisted independently of media row identifiers
The system SHALL persist artifact metadata in a store that does not depend on the current media_items row identifier.

#### Scenario: Media row identifier changes after database rebuild
- **WHEN** media rows are recreated with new identifiers
- **THEN** previously indexed artifact metadata remains valid for matching

#### Scenario: Artifact metadata persists across server restarts
- **WHEN** the server restarts
- **THEN** indexed artifact metadata remains available for subsequent relinking operations

### Requirement: Artifact metadata tracks health and integrity
The system SHALL track artifact health state and metadata validation outcomes.

#### Scenario: Metadata parse failure is tracked
- **WHEN** artifact metadata is unreadable or malformed
- **THEN** the artifact is marked with a metadata-invalid health state and excluded from deterministic relinking

#### Scenario: Artifact files become unavailable
- **WHEN** indexed artifact files are missing at validation time
- **THEN** the artifact health is updated to unavailable and reported to recovery operations