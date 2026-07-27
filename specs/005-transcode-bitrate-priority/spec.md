# Feature Specification: Transcode Bitrate Priority

**Feature Branch**: `005-transcode-bitrate-priority`

**Created**: 2026-07-27

**Status**: Specified

**Input**: User description: "When I enqueue a new item to be transcoded, I want the sub-jobs to be done in order of bitrate. The smallest job, likely to take the least amount of time, should be prioritized over the full-quality job that will take longer."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ascending Bitrate Sub-Job Execution (Priority: P1)

As a media server user, when I add a media item to be transcoded, I want the lowest bitrate (smallest output size) sub-jobs to process first so that playable content becomes available as quickly as possible.

**Why this priority**: Lower bitrate renditions complete in significantly less time than full-quality high bitrate renditions. Delivering the lowest bitrate sub-job first minimizes initial waiting time for video playback capability.

**Independent Test**: Enqueue a media item for transcoding and observe sub-job processing order. Verify that the lowest target bitrate sub-job finishes first, followed sequentially by higher target bitrate sub-jobs up to the highest quality.

**Acceptance Scenarios**:

1. **Given** a media item with sub-jobs generated for 360p, 480p, 720p, and 1080p renditions, **When** the item is enqueued for transcoding, **Then** the worker pool processes sub-jobs in strictly ascending order of target bitrate (e.g., 360p first, 1080p last).
2. **Given** a media item with active transcoding sub-jobs, **When** the lowest bitrate sub-job completes, **Then** the stream manifest is updated and lower-quality video playback is immediately available while higher bitrate sub-jobs continue processing.

---

### User Story 2 - Multiple Enqueued Media Items Priority Handling (Priority: P2)

As a media server administrator, when multiple media items are enqueued for transcoding, I want sub-job execution rules to consistently apply across all items according to defined queue priority policies.

**Why this priority**: Ensures multi-item queuing behaves predictably when multiple movies or TV episodes are queued simultaneously.

**Independent Test**: Enqueue two media items in succession and verify whether low-bitrate sub-jobs across items are prioritized according to queue policy without job starvation or worker deadlocks.

**Acceptance Scenarios**:

1. **Given** multiple media items queued for transcoding, **When** transcode workers become available, **Then** sub-jobs are dispatched according to the configured bitrate priority ordering policy across active items.

---

### Edge Cases

- What happens when two sub-jobs have identical target bitrates? (The sub-job associated with lower resolution or earlier creation order is prioritized first).
- What happens if a sub-job fails during processing? (The queue moves on to the next lowest bitrate sub-job, ensuring failure of one rendition does not block remaining sub-jobs).
- How are audio sub-jobs handled relative to video bitrates? (Audio extraction/transcoding sub-jobs are prioritized alongside or ahead of the lowest video bitrate rendition because audio is essential for playback).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST sequence transcode sub-jobs for an item in ascending order of target bitrate (lowest bitrate / smallest output file processed first).
- **FR-002**: System MUST allow playable media streams to be served as soon as the lowest bitrate sub-job completes, without waiting for higher bitrate sub-jobs to finish.
- **FR-003**: System MUST prioritize audio track processing sub-jobs prior to or alongside the lowest video bitrate sub-job to ensure complete audio-visual playback capability as early as possible.
- **FR-004**: System MUST handle sub-job failures gracefully without blocking subsequent higher or lower bitrate sub-jobs in the queue.
- **FR-005**: System MUST process enqueued media items in order of arrival (FIFO), and prioritize sub-jobs within each media item in strictly ascending order of target bitrate.

### Key Entities

- **Transcode Item**: Represents a parent media asset (movie or episode) queued for transcoding containing one or more sub-jobs.
- **Transcode Sub-Job**: A discrete transcoding task targeting a specific rendition (resolution, video target bitrate, audio track) belonging to a parent Transcode Item. Key attributes include target bitrate, quality level, state (pending, processing, completed, failed), and priority rank.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Time to initial playable stream availability for newly enqueued media is reduced by at least 60% compared to high-bitrate-first or unordered transcoding.
- **SC-002**: 100% of multi-rendition transcoding tasks process their video sub-jobs in strictly ascending target bitrate order.
- **SC-003**: Media server users can start playing media in initial low resolution within 30 seconds of transcode start for typical 1080p source video files.

## Assumptions

- Target bitrate metadata is deterministically calculable for every sub-job prior to dispatching to worker processes.
- Media player UI supports adaptive streaming manifests that dynamically expose newly finished renditions as sub-jobs complete.
- Worker processes have equal capability to process any bitrate rendition sub-job.
