package api

import (
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jjtny1/splitit/internal/auth"
	"github.com/jjtny1/splitit/internal/autosplit"
)

// maxAudioBytes caps an uploaded audio recording. Whisper's own limit is 25MB.
const maxAudioBytes = 25 << 20

// supportedAudioTypes are the media types the Whisper transcription API can
// read. The check is lenient — a missing or generic type still passes via the
// file-extension fallback (supportedAudioExts).
var supportedAudioTypes = map[string]bool{
	"audio/mpeg":  true,
	"audio/mp4":   true,
	"audio/m4a":   true,
	"audio/x-m4a": true,
	"audio/wav":   true,
	"audio/x-wav": true,
	"audio/webm":  true,
	"audio/flac":  true,
	"audio/ogg":   true,
	"video/mp4":   true,
	"video/webm":  true,
}

// supportedAudioExts are the file extensions accepted when the media type is
// missing or unrecognized.
var supportedAudioExts = map[string]bool{
	".mp3":  true,
	".m4a":  true,
	".mp4":  true,
	".wav":  true,
	".webm": true,
	".flac": true,
	".ogg":  true,
	".mpga": true,
}

// handleAudioSplit transcribes a host's audio recording, maps it onto the
// bill's items with the AI assigner, and replaces the host-managed
// participants and their claims so the existing split math runs unchanged.
func (s *Server) handleAudioSplit(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userCtxKey).(user)
	id := r.PathValue("id")

	b, err := s.loadBill(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("audio split: load: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if b.hostUserID != u.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAudioBytes)
	file, header, err := r.FormFile("audio")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "audio file too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "audio file required"})
		return
	}
	defer file.Close()

	hostName := strings.TrimSpace(r.FormValue("host_name"))
	if hostName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_name required"})
		return
	}

	audio, err := io.ReadAll(file)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "audio file too large"})
			return
		}
		log.Printf("audio split: read: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read upload"})
		return
	}

	filename := header.Filename
	if filename == "" {
		filename = "audio"
	}
	mediaType := header.Header.Get("Content-Type")
	base, _, _ := strings.Cut(mediaType, ";")
	base = strings.TrimSpace(strings.ToLower(base))
	ext := strings.ToLower(filepath.Ext(filename))
	if !supportedAudioTypes[base] && !supportedAudioExts[ext] {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
			"error": "unsupported audio type; upload an MP3, M4A, MP4, WAV, WebM, FLAC, or OGG file",
		})
		return
	}

	items, err := s.loadItems(r.Context(), b.ID)
	if err != nil {
		log.Printf("audio split: items: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if len(items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "add the receipt first"})
		return
	}

	transcript, err := s.Transcriber.Transcribe(r.Context(), audio, filename)
	if err != nil {
		log.Printf("audio split: transcribe: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not transcribe audio"})
		return
	}

	splitItems := make([]autosplit.Item, 0, len(items))
	for i, it := range items {
		splitItems = append(splitItems, autosplit.Item{
			Index:      i + 1,
			Name:       it.Name,
			PriceCents: it.PriceCents,
		})
	}

	assignment, err := s.Assigner.Assign(r.Context(), splitItems, transcript, hostName)
	if err != nil {
		log.Printf("audio split: assign: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not assign items"})
		return
	}
	assignment = autosplit.Validate(assignment, splitItems)

	if err := s.applyAudioSplit(r, b, items, assignment, hostName, transcript); err != nil {
		log.Printf("audio split: apply: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	b.SplitMode = "host"

	resp, err := s.buildSummary(r.Context(), b)
	if err != nil {
		log.Printf("audio split: summary: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	resp["transcript"] = transcript
	resp["notes"] = assignment.Notes
	writeJSON(w, http.StatusOK, resp)
}

// applyAudioSplit replaces a bill's host-managed participants and their claims
// with the ones from assignment, and flips the bill to host split mode — all
// in a single transaction.
func (s *Server) applyAudioSplit(r *http.Request, b bill, items []billItem, assignment autosplit.Assignment, hostName, transcript string) error {
	ctx := r.Context()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM claims WHERE participant_id IN
		 (SELECT id FROM participants WHERE bill_id = ? AND host_managed = 1)`, b.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM participants WHERE bill_id = ? AND host_managed = 1`, b.ID); err != nil {
		return err
	}

	now := time.Now().Unix()
	// partID maps a lowercased person name to its new participant id.
	partID := map[string]string{}
	for _, name := range assignment.People {
		token, err := auth.NewToken()
		if err != nil {
			return err
		}
		pid := uuid.NewString()
		isHost := strings.EqualFold(name, hostName)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO participants
			 (id, bill_id, display_name, participant_token, host_managed, is_host, created_at)
			 VALUES (?, ?, ?, ?, 1, ?, ?)`,
			pid, b.ID, name, token, isHost, now); err != nil {
			return err
		}
		partID[strings.ToLower(name)] = pid
	}

	for _, ia := range assignment.Items {
		// Validate guarantees the index is in [1..len(items)].
		itemID := items[ia.Index-1].ID
		for _, name := range ia.People {
			pid, ok := partID[strings.ToLower(name)]
			if !ok {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO claims (item_id, participant_id) VALUES (?, ?)`,
				itemID, pid); err != nil {
				return err
			}
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE bills SET split_mode = 'host', audio_transcript = ? WHERE id = ?`,
		transcript, b.ID); err != nil {
		return err
	}
	return tx.Commit()
}
