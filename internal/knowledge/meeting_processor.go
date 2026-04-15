package knowledge

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/naozhi/naozhi/internal/transcribe"
)

// MeetingProcessor handles the meeting upload-to-analysis pipeline.
type MeetingProcessor struct {
	store       *MeetingStore
	transcriber transcribe.Service
	audioDir    string // directory to store uploaded audio files
}

// NewMeetingProcessor creates a processor backed by the given store and transcriber.
func NewMeetingProcessor(store *MeetingStore, transcriber transcribe.Service, audioDir string) *MeetingProcessor {
	if audioDir != "" {
		os.MkdirAll(audioDir, 0o755)
	}
	return &MeetingProcessor{
		store:       store,
		transcriber: transcriber,
		audioDir:    audioDir,
	}
}

// ProcessMeeting takes an audio file path and produces a structured Meeting result.
// Phase 1: transcription via the existing transcribe.Service.
// Phase 2 (placeholder): LLM-based extraction of summary, decisions, action items.
func (mp *MeetingProcessor) ProcessMeeting(ctx context.Context, audioPath string, title string, participants []string) (*Meeting, error) {
	meeting := &Meeting{
		Title:        title,
		Date:         time.Now(),
		Participants: participants,
		AudioPath:    audioPath,
		Status:       "processing",
		CreatedAt:    time.Now(),
	}

	// Generate ID and persist immediately so callers can track status.
	if err := mp.store.Add(meeting); err != nil {
		return nil, fmt.Errorf("create meeting record: %w", err)
	}

	// Run transcription in the current goroutine (caller should wrap in go if async).
	transcript, err := mp.transcribeAudio(ctx, audioPath)
	if err != nil {
		meeting.Status = "failed"
		meeting.Error = err.Error()
		_ = mp.store.Update(meeting)
		return meeting, fmt.Errorf("transcribe: %w", err)
	}

	meeting.Transcript = transcript
	meeting.TranscriptPath = mp.transcriptPath(meeting.ID)

	// Write transcript to file for reference
	if meeting.TranscriptPath != "" {
		if writeErr := os.WriteFile(meeting.TranscriptPath, []byte(transcript), 0o644); writeErr != nil {
			slog.Warn("write transcript file", "err", writeErr)
		}
	}

	// Phase 2 placeholder: extract structured data from transcript.
	// In a future iteration this will call an LLM via the session router.
	meeting.Summary = extractPlaceholderSummary(transcript)
	meeting.Decisions = extractPlaceholderDecisions(transcript)
	meeting.ActionItems = extractPlaceholderActionItems(transcript)
	meeting.Duration = estimateDuration(transcript)
	meeting.Status = "completed"

	if err := mp.store.Update(meeting); err != nil {
		return meeting, fmt.Errorf("update meeting record: %w", err)
	}

	// I1: Clean up audio file after successful processing — transcript is on disk.
	if err := os.Remove(audioPath); err != nil {
		slog.Warn("delete audio file after processing", "path", audioPath, "err", err)
	}

	slog.Info("meeting processed", "id", meeting.ID, "title", meeting.Title)
	return meeting, nil
}

// transcribeAudio reads the audio file and sends it through the transcribe service.
func (mp *MeetingProcessor) transcribeAudio(ctx context.Context, audioPath string) (string, error) {
	if mp.transcriber == nil {
		return "", fmt.Errorf("transcribe service not configured")
	}

	data, err := os.ReadFile(audioPath)
	if err != nil {
		return "", fmt.Errorf("read audio file: %w", err)
	}

	// Detect MIME type from file extension as a hint
	mimeType := mimeFromExtension(filepath.Ext(audioPath))

	text, err := mp.transcriber.Transcribe(ctx, data, mimeType)
	if err != nil {
		return "", fmt.Errorf("transcription failed: %w", err)
	}
	return text, nil
}

// transcriptPath returns the path to store the transcript text file.
func (mp *MeetingProcessor) transcriptPath(meetingID string) string {
	if mp.audioDir == "" {
		return ""
	}
	return filepath.Join(mp.audioDir, meetingID+"-transcript.txt")
}

// SaveUploadedAudio writes uploaded audio data to the audio directory.
// Returns the path of the saved file.
func (mp *MeetingProcessor) SaveUploadedAudio(filename string, data []byte) (string, error) {
	if mp.audioDir == "" {
		return "", fmt.Errorf("audio directory not configured")
	}

	// Sanitise filename
	base := filepath.Base(filename)
	id, _ := randomHexID(4)
	safeName := id + "-" + base
	dest := filepath.Join(mp.audioDir, safeName)

	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", fmt.Errorf("save audio file: %w", err)
	}
	return dest, nil
}

// --- placeholder extraction helpers (Phase 2 will use LLM) ---

func extractPlaceholderSummary(transcript string) string {
	if len(transcript) == 0 {
		return "No transcript available."
	}
	// Take first 500 characters as a rough summary placeholder.
	text := strings.TrimSpace(transcript)
	if len(text) > 500 {
		text = text[:500] + "..."
	}
	return text
}

func extractPlaceholderDecisions(transcript string) []string {
	// Placeholder: scan for lines containing decision-related keywords.
	var decisions []string
	for _, line := range strings.Split(transcript, "\n") {
		lower := strings.ToLower(line)
		for _, kw := range []string{"decided", "decision", "agreed", "approved", "选择", "决定", "同意"} {
			if strings.Contains(lower, kw) {
				decisions = append(decisions, strings.TrimSpace(line))
				break
			}
		}
	}
	return decisions
}

func extractPlaceholderActionItems(transcript string) []ActionItem {
	// Placeholder: scan for lines containing action-related keywords.
	var items []ActionItem
	for _, line := range strings.Split(transcript, "\n") {
		lower := strings.ToLower(line)
		for _, kw := range []string{"action item", "todo", "follow up", "task", "待办", "跟进"} {
			if strings.Contains(lower, kw) {
				items = append(items, ActionItem{
					Description: strings.TrimSpace(line),
				})
				break
			}
		}
	}
	return items
}

func estimateDuration(transcript string) string {
	// Very rough heuristic: ~150 words per minute of speech.
	words := len(strings.Fields(transcript))
	if words == 0 {
		return "unknown"
	}
	minutes := words / 150
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("~%dmin", minutes)
}

func mimeFromExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".ogg":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".amr":
		return "audio/amr"
	default:
		return "audio/pcm"
	}
}
