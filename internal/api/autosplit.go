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

// maxPromptChars caps a typed split prompt.
const maxPromptChars = 4000

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

// handleAutoSplit turns a host's description of how the bill splits — either
// an audio recording (transcribed via Whisper) or a typed text prompt — into
// per-item assignments with the AI assigner, then replaces the host-managed
// participants and their claims so the existing split math runs unchanged.
//
// Auto-splitting is optional at the product level: a bill the host never
// auto-splits simply stays a normal claim bill where friends self-claim. This
// endpoint is only reached once the host has actually supplied audio or text.
func (s *Server) handleAutoSplit(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(userCtxKey).(user)
	id := r.PathValue("id")

	b, err := s.loadBill(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not found"})
		return
	}
	if err != nil {
		log.Printf("auto split: load: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if b.hostUserID != u.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAudioBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "audio file too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read upload"})
		return
	}

	hostName := strings.TrimSpace(r.FormValue("host_name"))
	if hostName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_name required"})
		return
	}

	items, err := s.loadItems(r.Context(), b.ID)
	if err != nil {
		log.Printf("auto split: items: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if len(items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "add the receipt first"})
		return
	}

	prompt, status, errMsg := s.resolveSplitPrompt(r)
	if status != 0 {
		writeJSON(w, status, map[string]string{"error": errMsg})
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

	assignment, err := s.Assigner.Assign(r.Context(), splitItems, prompt, hostName)
	if err != nil {
		log.Printf("auto split: assign: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not assign items"})
		return
	}
	assignment = autosplit.Validate(assignment, splitItems)

	if err := s.applyAutoSplit(r, b, items, assignment, hostName, prompt); err != nil {
		log.Printf("auto split: apply: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	b.SplitMode = "host"

	resp, err := s.buildSummary(r.Context(), b)
	if err != nil {
		log.Printf("auto split: summary: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	resp["prompt"] = prompt
	resp["notes"] = assignment.Notes
	writeJSON(w, http.StatusOK, resp)
}

// resolveSplitPrompt extracts the host's description of the split as plain
// text: an uploaded "audio" recording is transcribed via Whisper; otherwise a
// typed "text" field is used verbatim. The request's multipart form must
// already be parsed. status is 0 on success; otherwise it and errMsg carry
// the HTTP error the caller should return.
func (s *Server) resolveSplitPrompt(r *http.Request) (prompt string, status int, errMsg string) {
	if file, header, err := r.FormFile("audio"); err == nil {
		defer file.Close()
		audio, rerr := io.ReadAll(file)
		if rerr != nil {
			log.Printf("auto split: read audio: %v", rerr)
			return "", http.StatusBadRequest, "could not read upload"
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
			return "", http.StatusUnsupportedMediaType,
				"unsupported audio type; upload an MP3, M4A, MP4, WAV, WebM, FLAC, or OGG file"
		}
		transcript, terr := s.Transcriber.Transcribe(r.Context(), audio, filename)
		if terr != nil {
			log.Printf("auto split: transcribe: %v", terr)
			return "", http.StatusBadGateway, "could not transcribe audio"
		}
		return transcript, 0, ""
	}

	// No audio part — fall back to a typed text prompt.
	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		return "", http.StatusBadRequest, "describe the split by recording audio or writing a prompt"
	}
	if len(text) > maxPromptChars {
		return "", http.StatusBadRequest, "prompt is too long"
	}
	return text, 0, ""
}

// applyAutoSplit replaces a bill's host-managed participants and their claims
// with the ones from assignment, and flips the bill to host split mode — all
// in a single transaction.
func (s *Server) applyAutoSplit(r *http.Request, b bill, items []billItem, assignment autosplit.Assignment, hostName, prompt string) error {
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
		`UPDATE bills SET split_mode = 'host', split_prompt = ? WHERE id = ?`,
		prompt, b.ID); err != nil {
		return err
	}
	return tx.Commit()
}
