package web

import (
	"net/http"
	"strings"

	"github.com/voocel/ainovel-cli/internal/host/tts"
)

// audiobookRequest là body cho POST /api/audiobook (mirror TUI /sachnoi).
type audiobookRequest struct {
	From         int     `json:"from"`
	To           int     `json:"to"`
	Voice        string  `json:"voice"`
	Speed        float64 `json:"speed"`
	Bitrate      int     `json:"bitrate"`
	SampleRate   int     `json:"sampleRate"`
	OutputFormat string  `json:"outputFormat"`
	OutDir       string  `json:"outDir"`
	Overwrite    bool    `json:"overwrite"`
}

func (s *Server) handleAudiobook(w http.ResponseWriter, r *http.Request) {
	var req audiobookRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	opts := tts.Options{
		From:         req.From,
		To:           req.To,
		VoiceCode:    strings.TrimSpace(req.Voice),
		Speed:        req.Speed,
		Bitrate:      req.Bitrate,
		SampleRate:   req.SampleRate,
		OutputFormat: strings.TrimSpace(req.OutputFormat),
		OutDir:       strings.TrimSpace(req.OutDir),
		Overwrite:    req.Overwrite,
	}

	id, ctx := s.jobs.start()
	ch, err := s.eng.Audiobook(ctx, opts)
	if err != nil {
		s.jobs.finish(id)
		writeErr(w, http.StatusConflict, err)
		return
	}
	go func() {
		defer s.jobs.finish(id)
		for ev := range ch {
			done := ev.Stage == tts.StageDone || ev.Stage == tts.StageError
			s.emitProgress(progressDTO{
				Job: "audiobook", ID: id, Stage: string(ev.Stage),
				Current: ev.Current, Total: ev.Total, Message: ev.Message,
				Error: errString(ev.Err), Done: done,
			})
		}
	}()
	writeOK(w, map[string]any{"id": id})
}
