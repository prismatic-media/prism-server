# discovery-deduplication Specification

## Purpose
Prevent redundant transcoding by detecting at scan time that a newly discovered source file matches an existing transcode bundle or media item, using deterministic fingerprint matching.

## Requirements

### Requirement: Scanner computes source fingerprint at discovery time
The scanner SHALL compute a deterministic source fingerprint (SHA-256 of the first 64 KB) for each discovered video file and store it on the media item record.

#### Scenario: Fingerprint computed and stored for new file
- **WHEN** the scanner discovers a new video file that does not match any existing media item fingerprint in the same library
- **THEN** the scanner computes the source fingerprint, stores it on the new media item record, and proceeds with normal upsert behavior

#### Scenario: Fingerprint computed on rescan of existing item
- **WHEN** the scanner encounters a video file whose media item record has no stored fingerprint (e.g., after upgrade)
- **THEN** the scanner computes and stores the fingerprint on the existing record

### Requirement: Scanner detects file moves via fingerprint matching
The scanner SHALL detect file moves and renames within a library by matching the source fingerprint of a discovered file against existing media item fingerprints in the same library.

#### Scenario: File moved to a new path within the same library
- **WHEN** the scanner discovers a video file whose fingerprint matches an existing media item in the same library at a different path
- **THEN** the scanner updates the existing record's file path to the new location and sets source status to available without creating a new media item or firing a media-created event

#### Scenario: File fingerprint matches no existing item
- **WHEN** the scanner discovers a video file whose fingerprint does not match any existing media item in the same library
- **THEN** the scanner creates a new media item record as normal

### Requirement: Scanner links discovered files to existing bundles
The scanner SHALL check whether an existing transcode bundle matches a newly discovered file's fingerprint and link the media item to that bundle, preventing redundant transcoding.

#### Scenario: Discovered file matches an existing artifact bundle
- **WHEN** the scanner discovers a video file whose fingerprint matches an artifact record's source fingerprint
- **AND** the matched artifact belongs to an enabled storage area
- **THEN** the scanner links the media item to the existing bundle, sets bundle status to available, sets transcode status to done, assigns the bundle's MPD path, and does not enqueue a transcode job

#### Scenario: Discovered file has no matching artifact bundle
- **WHEN** the scanner discovers a video file whose fingerprint does not match any artifact record
- **THEN** the scanner does not modify bundle status and the item is eligible for normal transcode enqueueing

### Requirement: Deduplication is scoped to the same library
The scanner SHALL NOT match fingerprints across different libraries for deduplication purposes.

#### Scenario: Same file in two libraries creates independent items
- **WHEN** the same video file exists in two different libraries
- **THEN** each library has its own independent media item record and each is eligible for its own transcode job
