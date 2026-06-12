# artifact-relinking Specification

## Purpose
TBD - created by archiving change artifact-recovery. Update Purpose after archive.
## Requirements
### Requirement: Relinking uses deterministic fingerprint matching first
The system SHALL relink discovered artifacts to media items using exact source fingerprint matching as the primary method.

#### Scenario: Relink after database rebuild
- **WHEN** media items are recreated after a database loss and indexed artifacts contain matching source fingerprints
- **THEN** the relinking operation attaches artifacts to the corresponding media items without retranscoding

#### Scenario: Exact match required for deterministic relink
- **WHEN** an artifact source fingerprint does not exactly match any current media item fingerprint
- **THEN** the artifact is not auto-linked through deterministic matching

### Requirement: Relinking explicitly classifies unmatched and ambiguous results
The system SHALL produce explicit unmatched and ambiguous outcomes for artifacts that cannot be safely attached.

#### Scenario: No candidate media item
- **WHEN** no media item matches an indexed artifact
- **THEN** the artifact is classified as unmatched with actionable status

#### Scenario: Multiple candidate media items
- **WHEN** multiple media items satisfy non-deterministic fallback criteria for a single artifact
- **THEN** the artifact is classified as ambiguous and auto-linking is blocked

### Requirement: Recovery operations are admin-invokable and auditable
The system SHALL provide admin-invokable operations for relinking and SHALL return auditable result summaries.

#### Scenario: Admin triggers relink and receives summary
- **WHEN** an authenticated admin invokes the relinking operation
- **THEN** the response includes counts of linked, unmatched, ambiguous, and invalid artifacts

#### Scenario: Non-admin relink request is rejected
- **WHEN** a non-admin user invokes the relinking operation
- **THEN** the operation is rejected with forbidden access

