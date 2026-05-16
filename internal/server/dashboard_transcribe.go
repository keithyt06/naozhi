package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/naozhi/naozhi/internal/transcribe"
)

// transcribeSemCap is the maximum number of concurrent ffmpeg transcriptions.
// Exceeded requests receive 503 immediately to prevent CPU/memory DoS.
const transcribeSemCap = 3

// TranscribeHandler handles the audio transcription API endpoint.
type TranscribeHandler struct {
	transcriber       transcribe.Service
	transcribeLimiter *ipLimiter    // per-IP transcribe rate limiter (5/min)
	sem               chan struct{} // concurrency limiter (capacity transcribeSemCap)
}

// handleTranscribe accepts an audio file upload and returns transcribed text.
// POST /api/transcribe  (multipart/form-data, field "audio")
func (h *TranscribeHandler) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	slog.Info("transcribe request arrived",
		"content_type", r.Header.Get("Content-Type"),
		"content_length", r.ContentLength,
		"ua", r.Header.Get("User-Agent"))
	if h.transcribeLimiter != nil && !h.transcribeLimiter.AllowRequest(r) {
		slog.Warn("transcribe rate limited")
		writeJSONStatus(w, http.StatusTooManyRequests, map[string]string{"error": "transcribe rate limit exceeded"})
		return
	}
	if h.transcriber == nil {
		http.Error(w, "transcription not configured", http.StatusNotImplemented)
		return
	}

	// Acquire concurrency slot; reject immediately if all slots are busy.
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	case <-r.Context().Done():
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]string{"error": "transcribe busy"})
		return
	default:
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]string{"error": "transcribe busy"})
		return
	}

	const maxAudioSize = 10 << 20 // 10 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxAudioSize+4096)
	if err := r.ParseMultipartForm(maxAudioSize); err != nil {
		slog.Warn("transcribe parse multipart failed", "err", err, "ct", r.Header.Get("Content-Type"))
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	files := r.MultipartForm.File["audio"]
	if len(files) == 0 {
		http.Error(w, "missing audio field", http.StatusBadRequest)
		return
	}
	fh := files[0]

	f, err := fh.Open()
	if err != nil {
		http.Error(w, "failed to read audio", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "failed to read audio", http.StatusInternalServerError)
		return
	}

	// Step 1: allowlist the client-supplied Content-Type so obviously wrong
	// uploads are rejected cheaply before we run DetectContentType.
	// Strip parameters (";codecs=opus", ";boundary=...") so matches don't
	// fail for legitimate MIMEs like "audio/webm; codecs=opus" from
	// iPhone Safari 26 / Chromium.
	declaredMIME := fh.Header.Get("Content-Type")
	declaredBase := declaredMIME
	if i := strings.IndexByte(declaredBase, ';'); i >= 0 {
		declaredBase = strings.TrimSpace(declaredBase[:i])
	}
	switch declaredBase {
	case "audio/ogg", "audio/mpeg", "audio/wav", "audio/flac", "audio/mp4",
		"audio/amr", "audio/webm", "audio/aac", "audio/x-m4a",
		"video/mp4", "video/webm": // some browsers tag voice memos as video
	default:
		slog.Warn("transcribe unsupported declared mime", "declared", declaredMIME, "base", declaredBase, "filename", fh.Filename, "size", len(data))
		http.Error(w, "unsupported audio format", http.StatusBadRequest)
		return
	}
	// Step 2: magic-byte validation. http.DetectContentType returns
	// "application/ogg" for legitimate OGG streams (Feishu voice); accept that
	// too. The transcribe package runs a stricter DetectFormat before dispatch
	// so ffmpeg never sees content that lacks the right magic.
	detected := http.DetectContentType(data)
	if !strings.HasPrefix(detected, "audio/") &&
		!strings.HasPrefix(detected, "video/") &&
		detected != "application/ogg" {
		// Capture first bytes (hex) to diagnose what the client actually sent.
		preview := data
		if len(preview) > 16 {
			preview = preview[:16]
		}
		slog.Warn("transcribe content not audio", "declared", declaredMIME, "detected", detected, "size", len(data), "first_bytes_hex", fmt.Sprintf("%x", preview))
		http.Error(w, "file content is not audio", http.StatusBadRequest)
		return
	}
	// Use the sniffed MIME (not the client-supplied header) as the hint handed
	// to the transcriber. This prevents a caller from mislabelling content to
	// coerce ffmpeg dispatch into a format that doesn't match the actual bytes.
	// Normalize application/ogg → audio/ogg so transcribe's streaming path
	// can pick up OGG uploads without spawning ffmpeg unnecessarily.
	mimeType := detected
	if mimeType == "application/ogg" {
		mimeType = "audio/ogg"
	}
	text, err := h.transcriber.Transcribe(r.Context(), data, mimeType)
	if err != nil {
		slog.Warn("transcribe failed", "err", err, "mime", mimeType, "declared", declaredMIME, "size", len(data))
		http.Error(w, "transcription failed", http.StatusInternalServerError)
		return
	}

	slog.Info("transcribe ok", "text_len", len(text), "mime", mimeType, "size", len(data))

	writeJSON(w, map[string]string{"text": text})
}
