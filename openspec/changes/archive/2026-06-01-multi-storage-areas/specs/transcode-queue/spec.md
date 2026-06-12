## ADDED Requirements

### Requirement: Workers place transcode output in the eligible segment storage area with the most free bytes
When processing a transcode job, the worker SHALL evaluate enabled `segments` storage areas and place output in the eligible area with the greatest free raw bytes.

#### Scenario: Worker chooses area with highest free bytes
- **WHEN** multiple enabled `segments` storage areas are eligible
- **THEN** the worker writes transcode output to the storage area with the highest free raw bytes

#### Scenario: Worker skips unavailable or unwritable paths
- **WHEN** a configured enabled `segments` storage area is missing, unavailable, or unwritable
- **THEN** that area is excluded from candidate selection and other eligible areas are still considered

#### Scenario: Worker enforces reserve headroom
- **WHEN** a configured enabled `segments` storage area has free bytes less than or equal to `storage_min_free_bytes`
- **THEN** that area is excluded from candidate selection

#### Scenario: Worker fails job when no eligible area exists
- **WHEN** all enabled `segments` storage areas are unavailable, unwritable, disabled, or below reserve headroom
- **THEN** the transcode job is marked failed with an error indicating no eligible storage area
