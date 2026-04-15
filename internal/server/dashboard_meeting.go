package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/naozhi/naozhi/internal/knowledge"
)

// MeetingHandlers holds handlers for meeting-related API endpoints.
type MeetingHandlers struct {
	store     *knowledge.MeetingStore
	processor *knowledge.MeetingProcessor
}

// NewMeetingHandlers creates a MeetingHandlers instance.
func NewMeetingHandlers(store *knowledge.MeetingStore, processor *knowledge.MeetingProcessor) *MeetingHandlers {
	return &MeetingHandlers{store: store, processor: processor}
}

// GET /api/meetings -- list all meetings (newest first).
func (mh *MeetingHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	if mh.store == nil {
		writeJSONStatus(w, []struct{}{}, http.StatusOK)
		return
	}
	meetings := mh.store.List()
	if meetings == nil {
		meetings = []knowledge.Meeting{}
	}
	writeJSONStatus(w, map[string]any{
		"meetings": meetings,
		"total":    len(meetings),
	}, http.StatusOK)
}

// GET /api/meetings/{id} -- single meeting detail with transcript.
func (mh *MeetingHandlers) handleGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/meetings/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeJSONStatus(w, map[string]string{"error": "id required"}, http.StatusBadRequest)
		return
	}
	if mh.store == nil {
		writeJSONStatus(w, map[string]string{"error": "meetings not configured"}, http.StatusServiceUnavailable)
		return
	}

	meeting, err := mh.store.Get(id)
	if err != nil {
		writeJSONStatus(w, map[string]string{"error": err.Error()}, http.StatusNotFound)
		return
	}
	writeJSONStatus(w, meeting, http.StatusOK)
}

// POST /api/meetings/upload -- receive audio file, process async.
// Accepts multipart form: file (audio), title, participants (comma-separated).
func (mh *MeetingHandlers) handleUpload(w http.ResponseWriter, r *http.Request) {
	if mh.processor == nil {
		writeJSONStatus(w, map[string]string{"error": "meeting processor not configured"}, http.StatusServiceUnavailable)
		return
	}

	// Limit to 100 MB
	r.Body = http.MaxBytesReader(w, r.Body, 100<<20)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeJSONStatus(w, map[string]string{"error": "file too large or invalid form (max 100MB)"}, http.StatusRequestEntityTooLarge)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSONStatus(w, map[string]string{"error": "file field required"}, http.StatusBadRequest)
		return
	}
	defer file.Close()

	// I2: Validate audio format before reading entire file into memory.
	allowedExts := map[string]bool{
		".mp3": true, ".m4a": true, ".wav": true, ".flac": true,
		".ogg": true, ".opus": true, ".amr": true, ".webm": true,
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExts[ext] {
		writeJSONStatus(w, map[string]string{
			"error": "unsupported audio format: " + ext + "; allowed: .mp3, .m4a, .wav, .flac, .ogg, .opus, .amr, .webm",
		}, http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSONStatus(w, map[string]string{"error": "read file failed"}, http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	if title == "" {
		title = header.Filename
	}

	var participants []string
	if p := r.FormValue("participants"); p != "" {
		for _, s := range strings.Split(p, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				participants = append(participants, s)
			}
		}
	}

	// Save audio to disk
	audioPath, err := mh.processor.SaveUploadedAudio(header.Filename, data)
	if err != nil {
		writeJSONStatus(w, map[string]string{"error": "save audio failed: " + err.Error()}, http.StatusInternalServerError)
		return
	}

	// Create a pending meeting record and return immediately; process async.
	meeting := &knowledge.Meeting{
		Title:        title,
		Participants: participants,
		AudioPath:    audioPath,
		Status:       "pending",
	}
	if err := mh.store.Add(meeting); err != nil {
		writeJSONStatus(w, map[string]string{"error": "create meeting record: " + err.Error()}, http.StatusInternalServerError)
		return
	}

	// C1: Process asynchronously — use context.Background() because r.Context()
	// will be cancelled when the HTTP handler returns.
	go func() {
		if _, err := mh.processor.ProcessMeeting(context.Background(), audioPath, title, participants); err != nil {
			slog.Warn("meeting processing failed", "id", meeting.ID, "err", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "accepted",
		"meeting_id": meeting.ID,
	})
}
