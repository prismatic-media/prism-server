# artifact-indexing Specification

## Purpose

TBD - created by archiving change artifact-recovery. Update Purpose after archive.

## Requirements

### Requirement: Indexer discovers transcode bundles in enabled segment storage areas

The system SHALL provide an artifact indexing operation that scans enabled segment storage areas and registers discovered transcode bundles.

#### Scenario: Index newly added storage path with preexisting bundles

- **WHEN** an admin adds and enables a segment storage area that already contains transcode bundles
- **THEN** the indexing operation discovers those bundles and registers them as artifacts

#### Scenario: Skip disabled storage areas during indexing

- **WHEN** a segment storage area is disabled
- **THEN** the indexing operation excludes that area from bundle discovery

### Requirement: Indexing validates bundle shape before registration

The system SHALL validate discovered bundle contents before registering an artifact.

#### Scenario: Reject incomplete bundle

- **WHEN** a discovered directory does not contain a valid manifest and required companion files
- **THEN** the directory is not registered as a valid artifact and is reported as invalid

#### Scenario: Register complete bundle

- **WHEN** a discovered directory contains valid artifact metadata and required transcode outputs
- **THEN** the directory is registered as a valid artifact candidate

### Requirement: Indexing is idempotent and updates observation state

The indexing operation SHALL be idempotent and SHALL update artifact observation timestamps and health state on repeated scans.

#### Scenario: Re-index without creating duplicates

- **WHEN** the same storage area is indexed multiple times with unchanged bundle contents
- **THEN** existing artifact records are updated in place and duplicate artifacts are not created

#### Scenario: Mark previously seen artifact as missing

- **WHEN** a previously indexed artifact is not present in a later scan of its storage area
- **THEN** the artifact state is updated to indicate missing or stale availability
