## ADDED Requirements

### Requirement: Indexer creates media items from v2 bundle metadata

The artifact indexer SHALL create new media item records from v2 sidecar metadata when no matching media item exists in the database, making the indexer a co-equal creator of media items alongside the scanner.

#### Scenario: Create media item from orphaned v2 bundle

- **WHEN** the indexer discovers a transcode bundle with a v2 sidecar containing full media metadata
- **AND** no media item in the database has a source fingerprint matching the sidecar's fingerprint
- **THEN** the indexer creates a new media item record with metadata from the sidecar, source status set to missing, bundle status set to available, and transcode status set to done

#### Scenario: Create TV episode hierarchy from v2 bundle

- **WHEN** the indexer creates a media item from a v2 sidecar with media type episode
- **THEN** the indexer upserts the parent TV show and TV season records using the show name and season number from the sidecar and links the new episode to them

#### Scenario: Skip media creation for v1 sidecars

- **WHEN** the indexer discovers a bundle with a v1 sidecar lacking enriched metadata
- **THEN** the indexer registers the artifact record but does not create a media item from it

#### Scenario: Skip media creation when fingerprint already matches

- **WHEN** the indexer discovers a bundle whose fingerprint matches an existing media item
- **THEN** the indexer links the artifact to the existing item without creating a duplicate

#### Scenario: Index summary includes created media items count

- **WHEN** an indexing operation creates media items from orphaned bundles
- **THEN** the index summary includes the count of media items created
