package handler

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/internal/transcoder"
	"github.com/prismatic-media/prism-server/pkg/events"
)

type WorkerHandler struct {
	db   *sql.DB
	pool *transcoder.Pool
	bus  *events.Bus
}

func NewWorkerHandler(db *sql.DB, pool *transcoder.Pool, bus *events.Bus) *WorkerHandler {
	return &WorkerHandler{db: db, pool: pool, bus: bus}
}

type contextKey string

const workerContextKey contextKey = "worker"

func WorkerFromContext(ctx context.Context) *models.TranscodeWorker {
	v, _ := ctx.Value(workerContextKey).(*models.TranscodeWorker)
	return v
}

// Authenticate is a middleware that validates the worker API Key from the X-Worker-API-Key header.
func (h *WorkerHandler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Worker-API-Key")
		if key == "" {
			respondError(w, http.StatusUnauthorized, "missing X-Worker-API-Key header")
			return
		}

		worker, err := sqlite.GetWorkerByAPIKey(r.Context(), h.db, key)
		if errors.Is(err, sqlite.ErrNotFound) {
			respondError(w, http.StatusUnauthorized, "invalid worker api key", err)
			return
		} else if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to validate worker key", err)
			return
		}

		ctx := context.WithValue(r.Context(), workerContextKey, worker)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type WorkerSubJob struct {
	ID           uuid.UUID                `json:"id"`
	JobID        uuid.UUID                `json:"job_id"`
	MediaItemID  uuid.UUID                `json:"media_item_id"`
	Type         string                   `json:"type"` // "video", "subtitles", or "whisper"
	Profile      *models.TranscodeProfile `json:"profile,omitempty"`
	WhisperModel string                   `json:"whisper_model,omitempty"`
}

type heartbeatResponse struct {
	Threads int           `json:"threads"`
	HWAccel string        `json:"hwaccel"`
	Job     *WorkerSubJob `json:"job"` // keep JSON key as "job" for worker compatibility
}

type progressRequest struct {
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
	ErrorMsg string  `json:"error_msg"`
}

// @Summary Worker Heartbeat
// @Description Submit worker status heartbeat. If the worker has capacity, claims and returns the next pending transcode sub-job.
// @Tags Worker Interface
// @Security WorkerAuth
// @Produce json
// @Success 200 {object} heartbeatResponse
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /worker/heartbeat [post]
func (h *WorkerHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	worker := WorkerFromContext(r.Context())
	if worker == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Count active processing sub-jobs for this worker
	var activeCount int
	err := h.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM transcode_sub_jobs 
		WHERE worker_id = ? AND status = 'processing'`, 
		worker.ID.String(),
	).Scan(&activeCount)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count active jobs", err)
		return
	}

	status := "idle"
	if activeCount > 0 {
		status = "transcoding"
	}

	err = sqlite.UpdateWorkerHeartbeat(r.Context(), h.db, worker.ID, status)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update heartbeat", err)
		return
	}

	var claimedSubJob *models.TranscodeSubJob
	if activeCount < worker.Threads {
		// Claim next sub-job
		claimedSubJob, err = sqlite.ClaimNextSubJob(r.Context(), h.db, &worker.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to claim job", err)
			return
		}

		if claimedSubJob != nil {
			// Update status of worker to transcoding
			_ = sqlite.UpdateWorkerHeartbeat(r.Context(), h.db, worker.ID, "transcoding")
			
			// Set media transcode status & publish events
			item, err := sqlite.GetMediaItemByID(r.Context(), h.db, claimedSubJob.MediaItemID)
			if err == nil {
				// Clean up old transcode files if a bundle is currently available,
				// but ONLY if this is the first sub-job of the parent job to start processing.
				isFirstSubJob := false
				if subJobs, err := sqlite.ListTranscodeSubJobsByJob(r.Context(), h.db, claimedSubJob.JobID); err == nil {
					nonPendingCount := 0
					for _, sj := range subJobs {
						if sj.Status != models.TranscodeStatusPending {
							nonPendingCount++
						}
					}
					if nonPendingCount <= 1 {
						isFirstSubJob = true
					}
				}

				if isFirstSubJob && item.BundleStatus == models.BundleStatusAvailable {
					outputDir, err := h.pool.SelectSegmentsOutputDir(r.Context(), claimedSubJob.MediaItemID)
					if err == nil && outputDir != "" {
						_ = os.RemoveAll(outputDir)
					}
					_ = sqlite.SetMediaBundleStatus(r.Context(), h.db, claimedSubJob.MediaItemID, models.BundleStatusNone)
				}

				if h.bus != nil {
					h.bus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
						MediaItemID:     claimedSubJob.MediaItemID,
						LibraryID:       item.LibraryID,
						TranscodeStatus: string(models.TranscodeStatusProcessing),
					})
				}
			}
		}
	}

	var wSubJob *WorkerSubJob
	if claimedSubJob != nil {
		var prof *models.TranscodeProfile
		if claimedSubJob.Type == models.SubJobTypeVideo {
			if claimedSubJob.ProfileID != nil {
				prof, err = sqlite.GetTranscodeProfile(r.Context(), h.db, *claimedSubJob.ProfileID)
				if err != nil {
					respondError(w, http.StatusInternalServerError, "failed to load profile for sub-job", err)
					return
				}
			}
			if prof == nil {
				prof = &models.TranscodeProfile{}
			}
			if claimedSubJob.ProfileName != nil {
				prof.Name = *claimedSubJob.ProfileName
			}
			if claimedSubJob.Width != nil {
				prof.Width = *claimedSubJob.Width
			}
			if claimedSubJob.Height != nil {
				prof.Height = *claimedSubJob.Height
			}
			if claimedSubJob.VideoBitrateK != nil {
				prof.VideoBitrateK = *claimedSubJob.VideoBitrateK
			}
			if claimedSubJob.AudioBitrateK != nil {
				prof.AudioBitrateK = *claimedSubJob.AudioBitrateK
			}
			if claimedSubJob.Codec != nil {
				prof.Codec = *claimedSubJob.Codec
			}
		}

		var whisperModel string
		if claimedSubJob.Type == models.SubJobTypeWhisper {
			whisperModel, _ = sqlite.GetSetting(r.Context(), h.db, "whisper_model")
			if whisperModel == "" {
				whisperModel = "base"
			}
		}

		wSubJob = &WorkerSubJob{
			ID:           claimedSubJob.ID,
			JobID:        claimedSubJob.JobID,
			MediaItemID:  claimedSubJob.MediaItemID,
			Type:         claimedSubJob.Type,
			Profile:      prof,
			WhisperModel: whisperModel,
		}
	}

	respondJSON(w, http.StatusOK, heartbeatResponse{
		Threads: worker.Threads,
		HWAccel: worker.HWAccel,
		Job:     wSubJob,
	})
}

// @Summary Download Source Video
// @Description Download the original raw media source file for transcoding.
// @Tags Worker Interface
// @Security WorkerAuth
// @Produce video/mp4,video/quicktime,video/x-matroska,application/octet-stream
// @Param media_id path string true "Media ID" format(uuid)
// @Success 200 {file} file "Raw media source file"
// @Failure 400 {object} map[string]string "Invalid media ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Media item not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /media/{media_id}/source [get]
func (h *WorkerHandler) DownloadSource(w http.ResponseWriter, r *http.Request) {
	if WorkerFromContext(r.Context()) == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	mediaID, err := uuidParam(r, "media_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, mediaID)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found", err)
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get media item", err)
		return
	}

	http.ServeFile(w, r, item.FilePath)
}

// @Summary Update Sub-Job Progress
// @Description Update progress (0-100) or report failures of the currently assigned transcode sub-job.
// @Tags Worker Interface
// @Security WorkerAuth
// @Accept json
// @Produce json
// @Param job_id path string true "Job ID" format(uuid)
// @Param subjob_id path string true "Sub-Job ID" format(uuid)
// @Param body body progressRequest true "Progress update payload"
// @Success 200 {object} map[string]string "Returns {'status': 'ok'}"
// @Failure 400 {object} map[string]string "Invalid request body or sub-job ID"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Sub-job not assigned to this worker"
// @Failure 404 {object} map[string]string "Sub-job not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /jobs/{job_id}/subjobs/{subjob_id} [patch]
func (h *WorkerHandler) UpdateSubJobProgress(w http.ResponseWriter, r *http.Request) {
	worker := WorkerFromContext(r.Context())
	if worker == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	jobID, err := uuidParam(r, "job_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid job id", err)
		return
	}
	subJobID, err := uuidParam(r, "subjob_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sub-job id", err)
		return
	}

	var req progressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	subJob, err := sqlite.GetTranscodeSubJobByID(r.Context(), h.db, subJobID)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "sub-job not found", err)
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch sub-job", err)
		return
	}

	if subJob.JobID != jobID {
		respondError(w, http.StatusBadRequest, "sub-job does not belong to specified job")
		return
	}

	if subJob.WorkerID == nil || *subJob.WorkerID != worker.ID {
		respondError(w, http.StatusForbidden, "sub-job not assigned to this worker")
		return
	}

	if req.Status == "failed" {
		errStr := req.ErrorMsg
		if errStr == "" {
			errStr = "unknown worker error"
		}
		_ = sqlite.UpdateSubJobStatus(r.Context(), h.db, subJobID, models.TranscodeStatusFailed, &errStr)

		var subJobs []*models.TranscodeSubJob
		parentJob, err := sqlite.GetTranscodeJobByID(r.Context(), h.db, subJob.JobID)
		if err == nil && parentJob != nil {
			subJobs = parentJob.SubJobs
		}

		h.pool.Hub().Publish(transcoder.ProgressEvent{
			JobID:    subJob.JobID,
			Progress: 0,
			Done:     true,
			Error:    errStr,
			SubJobs:  subJobs,
		})

		if h.bus != nil {
			item, _ := sqlite.GetMediaItemByID(r.Context(), h.db, subJob.MediaItemID)
			var libraryID uuid.UUID
			if item != nil {
				libraryID = item.LibraryID
			}
			h.bus.Publish(events.EventJobProgress, events.JobProgressPayload{
				JobID:       subJob.JobID,
				MediaItemID: subJob.MediaItemID,
				Done:        true,
				Error:       errStr,
				SubJobs:     subJobs,
			})
			h.bus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
				MediaItemID:     subJob.MediaItemID,
				LibraryID:       libraryID,
				TranscodeStatus: string(models.TranscodeStatusFailed),
			})
		}
	} else {
		_ = sqlite.UpdateSubJobProgress(r.Context(), h.db, subJobID, req.Progress)

		// Fetch updated parent job to get the averaged progress
		parentJob, err := sqlite.GetTranscodeJobByID(r.Context(), h.db, subJob.JobID)
		if err == nil && parentJob != nil {
			h.pool.Hub().Publish(transcoder.ProgressEvent{
				JobID:    subJob.JobID,
				Progress: parentJob.Progress,
				SubJobs:  parentJob.SubJobs,
			})

			if h.bus != nil {
				h.bus.Publish(events.EventJobProgress, events.JobProgressPayload{
					JobID:       subJob.JobID,
					MediaItemID: subJob.MediaItemID,
					Progress:    parentJob.Progress,
					SubJobs:     parentJob.SubJobs,
				})
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// @Summary Upload Transcode Output Bundle for Sub-Job
// @Description Upload completed stream/segments output files as a ZIP bundle for a sub-job. The server extracts the bundle, merges segments, regenerates the master manifest, and updates statuses.
// @Tags Worker Interface
// @Security WorkerAuth
// @Accept multipart/form-data
// @Produce json
// @Param job_id path string true "Job ID" format(uuid)
// @Param subjob_id path string true "Sub-Job ID" format(uuid)
// @Param bundle formData file true "The ZIP bundle file containing transcode outputs (segments, etc.)"
// @Success 200 {object} map[string]string "Returns {'status': 'ok'}"
// @Failure 400 {object} map[string]string "Invalid sub-job ID or multipart form"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Sub-job not assigned to this worker"
// @Failure 404 {object} map[string]string "Sub-job not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /jobs/{job_id}/subjobs/{subjob_id}/bundle [put]
func (h *WorkerHandler) UploadSubJobBundle(w http.ResponseWriter, r *http.Request) {
	worker := WorkerFromContext(r.Context())
	if worker == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	jobID, err := uuidParam(r, "job_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid job id", err)
		return
	}
	subJobID, err := uuidParam(r, "subjob_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sub-job id", err)
		return
	}

	subJob, err := sqlite.GetTranscodeSubJobByID(r.Context(), h.db, subJobID)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "sub-job not found", err)
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch sub-job", err)
		return
	}

	if subJob.JobID != jobID {
		respondError(w, http.StatusBadRequest, "sub-job does not belong to specified job")
		return
	}

	if subJob.WorkerID == nil || *subJob.WorkerID != worker.ID {
		respondError(w, http.StatusForbidden, "sub-job not assigned to this worker")
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, subJob.MediaItemID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get media item", err)
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse multipart form", err)
		return
	}

	var file io.Reader
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			respondError(w, http.StatusBadRequest, "failed to read next part", err)
			return
		}
		if part.FormName() == "bundle" {
			file = part
			defer func() { _ = part.Close() }()
			break
		}
		_ = part.Close()
	}

	if file == nil {
		respondError(w, http.StatusBadRequest, "missing bundle file")
		return
	}

	outputDir, err := h.pool.SelectSegmentsOutputDir(r.Context(), subJob.MediaItemID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to select output path", err)
		return
	}

	tempFile, err := os.CreateTemp(filepath.Dir(outputDir), "transcode-bundle-*.zip")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create temp file", err)
		return
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	defer func() { _ = tempFile.Close() }()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to write zip file", err)
		return
	}

	// Make sure target output directory exists (we do NOT call RemoveAll here to preserve other sub-jobs' segments)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create output dir", err)
		return
	}

	if err := unzipFile(tempFile.Name(), outputDir); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to extract transcode zip", err)
		return
	}

	if subJob.Type == models.SubJobTypeWhisper {
		entries, err := os.ReadDir(outputDir)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to read whisper output directory", err)
			return
		}
		var vttFile string
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".vtt") {
				vttFile = filepath.Join(outputDir, entry.Name())
				break
			}
		}
		if vttFile == "" {
			respondError(w, http.StatusInternalServerError, "whisper output VTT not found in bundle")
			return
		}
		vttBytes, err := os.ReadFile(vttFile)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to read whisper VTT file", err)
			return
		}
		_ = os.Remove(vttFile)

		whisperLang, _ := sqlite.GetSetting(r.Context(), h.db, "whisper_default_language")
		if whisperLang == "" {
			whisperLang = "en"
		}

		transcription := &models.WhisperTranscription{
			MediaItemID: subJob.MediaItemID,
			Language:    whisperLang,
			VTTContent:  string(vttBytes),
		}
		if err := sqlite.AddWhisperTranscription(r.Context(), h.db, transcription); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to save whisper transcription", err)
			return
		}

		_ = sqlite.UpdateSubJobStatus(r.Context(), h.db, subJob.ID, models.TranscodeStatusDone, nil)
		_ = sqlite.UpdateSubJobProgress(r.Context(), h.db, subJob.ID, 100)

		if h.pool.OnWhisperDone != nil {
			h.pool.OnWhisperDone(r.Context(), subJob.MediaItemID)
		}

		parentJob, err := sqlite.GetTranscodeJobByID(r.Context(), h.db, subJob.JobID)
		if err == nil && parentJob != nil {
			isDone := parentJob.Status == models.TranscodeStatusDone || parentJob.Status == models.TranscodeStatusFailed
			var errStr string
			if parentJob.ErrorMsg != nil {
				errStr = *parentJob.ErrorMsg
			}
			h.pool.Hub().Publish(transcoder.ProgressEvent{
				JobID:    parentJob.ID,
				Progress: parentJob.Progress,
				Done:     isDone,
				Error:    errStr,
				SubJobs:  parentJob.SubJobs,
			})
			if h.bus != nil {
				h.bus.Publish(events.EventJobProgress, events.JobProgressPayload{
					JobID:       parentJob.ID,
					MediaItemID: parentJob.MediaItemID,
					Progress:    parentJob.Progress,
					Done:        isDone,
					Error:       errStr,
					SubJobs:     parentJob.SubJobs,
				})
				if isDone {
					h.bus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
						MediaItemID:     parentJob.MediaItemID,
						LibraryID:       item.LibraryID,
						TranscodeStatus: string(parentJob.Status),
					})
				}
			}
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// Update sub-job status to Done
	_ = sqlite.UpdateSubJobStatus(r.Context(), h.db, subJob.ID, models.TranscodeStatusDone, nil)
	_ = sqlite.UpdateSubJobProgress(r.Context(), h.db, subJob.ID, 100)

	// Regenerate manifest based on completed sub-jobs
	if err := transcoder.RegenerateManifestForJob(r.Context(), h.db, subJob.JobID, outputDir); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to regenerate manifest", err)
		return
	}

	mpdPath := filepath.Join(outputDir, "manifest.mpd")
	h.pool.MPDCache().Set(subJob.MediaItemID, mpdPath)

	// Write sidecar file for recovery
	if err := transcoder.WriteSidecarForMediaItem(r.Context(), h.db, subJob.MediaItemID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to write artifact sidecar", err)
		return
	}

	// Fetch parent job and check final status
	parentJob, err := sqlite.GetTranscodeJobByID(r.Context(), h.db, subJob.JobID)
	if err == nil && parentJob != nil {
		isDone := parentJob.Status == models.TranscodeStatusDone || parentJob.Status == models.TranscodeStatusFailed
		var errStr string
		if parentJob.ErrorMsg != nil {
			errStr = *parentJob.ErrorMsg
		}

		h.pool.Hub().Publish(transcoder.ProgressEvent{
			JobID:    parentJob.ID,
			Progress: parentJob.Progress,
			Done:     isDone,
			Error:    errStr,
			SubJobs:  parentJob.SubJobs,
		})

		if h.bus != nil {
			h.bus.Publish(events.EventJobProgress, events.JobProgressPayload{
				JobID:       parentJob.ID,
				MediaItemID: parentJob.MediaItemID,
				Progress:    parentJob.Progress,
				Done:        isDone,
				Error:       errStr,
				SubJobs:     parentJob.SubJobs,
			})
			if isDone {
				h.bus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
					MediaItemID:     parentJob.MediaItemID,
					LibraryID:       item.LibraryID,
					TranscodeStatus: string(parentJob.Status),
				})
			} else {
				// Publish intermediate update so UI knows bundle_status might have changed to available
				h.bus.Publish(events.EventMediaUpdated, events.MediaUpdatedPayload{
					MediaItemID:     parentJob.MediaItemID,
					LibraryID:       item.LibraryID,
					TranscodeStatus: string(models.TranscodeStatusProcessing),
				})
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func unzipFile(src string, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path (zip slip security violation)", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, os.ModePerm); err != nil {
				return err
			}
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			_ = outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		_ = outFile.Close()
		_ = rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

type registerWorkerRequest struct {
	Name  string `json:"name"`
	Token string `json:"token"`
}

type registerWorkerResponse struct {
	APIKey string `json:"api_key"`
}

// @Summary Register Ephemeral Transcode Worker
// @Description Register an ephemeral remote transcode worker using a registration token.
// @Tags Worker Interface
// @Accept json
// @Produce json
// @Param body body registerWorkerRequest true "Registration details"
// @Success 200 {object} registerWorkerResponse
// @Failure 400 {object} map[string]string "Invalid request or token name missing"
// @Failure 401 {object} map[string]string "Invalid registration token"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /worker/register [post]
func (h *WorkerHandler) RegisterWorker(w http.ResponseWriter, r *http.Request) {
	var req registerWorkerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "worker name is required")
		return
	}
	if req.Token == "" {
		respondError(w, http.StatusBadRequest, "registration token is required")
		return
	}

	// 1. Verify token
	_, err := sqlite.GetEphemeralWorkerTokenByValue(r.Context(), h.db, req.Token)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusUnauthorized, "invalid registration token", err)
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to validate registration token", err)
		return
	}

	// 2. Check if a worker with the same name already exists. If yes, delete it (and requeue its jobs)
	existing, err := sqlite.GetWorkerByName(r.Context(), h.db, req.Name)
	if err == nil && existing != nil {
		err = sqlite.DeleteWorker(r.Context(), h.db, existing.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to clean up existing duplicate worker", err)
			return
		}
	} else if err != nil && !errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusInternalServerError, "failed to query duplicate workers", err)
		return
	}

	// 3. Create the ephemeral worker record
	worker, err := sqlite.CreateEphemeralWorker(r.Context(), h.db, req.Name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to register ephemeral worker", err)
		return
	}

	respondJSON(w, http.StatusOK, registerWorkerResponse{APIKey: worker.APIKey})
}

