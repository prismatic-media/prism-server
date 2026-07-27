# Quickstart & Validation Guide: Transcode Bitrate Priority

## Overview

This guide outlines how to verify that transcode sub-jobs are claimed and executed in ascending target bitrate order.

For architectural details, refer to:
- [Specification](file:///home/benwelker/repos/prism-server/specs/005-transcode-bitrate-priority/spec.md)
- [Data Model](file:///home/benwelker/repos/prism-server/specs/005-transcode-bitrate-priority/data-model.md)
- [Interface Contracts](file:///home/benwelker/repos/prism-server/specs/005-transcode-bitrate-priority/contracts/subjob_claiming.md)

---

## Automated Verification

Run unit tests verifying sub-job claim ordering:

```bash
go test -v ./internal/store/sqlite -run TestClaimNextSubJob
```

### Expected Output

The test suite validates:
1. When multiple video sub-jobs with profiles (e.g. 360p @ 800k, 720p @ 2500k, 1080p @ 4500k) are enqueued for a media item, `ClaimNextSubJob` returns the 360p sub-job first.
2. Subsequent calls to `ClaimNextSubJob` return 720p and 1080p in strictly ascending bitrate order.

---

## End-to-End Server Verification

1. **Start Development Server**:
   ```bash
   make dev
   ```

2. **Enqueue Media Asset**:
   Use API or UI to trigger transcoding for a media file:
   ```bash
   curl -X POST http://localhost:8080/api/v1/jobs -H "Authorization: Bearer <TOKEN>" -d '{"media_item_id": "<ITEM_UUID>"}'
   ```

3. **Verify Sub-Job Claim Order in Server Logs**:
   Observe server stdout for sub-job processing events. Confirm that low-bitrate profiles finish first and stream manifests reflect 360p rendition availability prior to 1080p completion.
