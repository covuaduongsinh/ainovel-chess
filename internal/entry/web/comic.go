package web

import (
	"net/http"
	"strings"

	"github.com/voocel/ainovel-cli/internal/host/comic"
)

// comicRequest là body cho POST /api/comic (mirror lệnh /truyentranh trên TUI).
type comicRequest struct {
	Products  []string `json:"products"` // rỗng = tất cả (DefaultOrder)
	From      int      `json:"from"`
	To        int      `json:"to"`
	OutDir    string   `json:"outDir"`
	Preset    string   `json:"preset"`
	Style     string   `json:"style"`
	PageSize  string   `json:"pageSize"`
	Formats   []string `json:"formats"`
	Overwrite bool     `json:"overwrite"`
	MaxImages int      `json:"maxImages"`
	ImageSize string   `json:"imageSize"`
}

func (s *Server) handleComic(w http.ResponseWriter, r *http.Request) {
	var req comicRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var products []comic.Product
	for _, p := range req.Products {
		p = strings.TrimSpace(p)
		if p == "" || p == "all" {
			continue
		}
		products = append(products, comic.Product(p))
	}
	var formats []comic.Format
	for _, f := range req.Formats {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		formats = append(formats, comic.Format(f))
	}

	opts := comic.Options{
		Products:    products,
		From:        req.From,
		To:          req.To,
		OutDir:      strings.TrimSpace(req.OutDir),
		StylePreset: strings.TrimSpace(req.Preset),
		StyleHint:   strings.TrimSpace(req.Style),
		PageSize:    strings.TrimSpace(req.PageSize),
		Formats:     formats,
		Overwrite:   req.Overwrite,
		MaxImages:   req.MaxImages,
		ImageSize:   strings.TrimSpace(req.ImageSize),
	}

	id, ctx := s.jobs.start()
	ch, err := s.eng.Comic(ctx, opts)
	if err != nil {
		s.jobs.finish(id)
		writeErr(w, http.StatusConflict, err)
		return
	}
	go func() {
		defer s.jobs.finish(id)
		for ev := range ch {
			done := ev.Stage == comic.StageDone || ev.Stage == comic.StageError
			s.emitProgress(progressDTO{
				Job: "comic", ID: id, Stage: string(ev.Stage),
				Current: ev.Current, Total: ev.Total, Message: ev.Message,
				Error: errString(ev.Err), Done: done,
			})
		}
	}()
	writeOK(w, map[string]any{"id": id})
}

// handleComicPresets trả danh sách preset phong cách để Web và TUI hiển thị giống nhau.
func (s *Server) handleComicPresets(w http.ResponseWriter, _ *http.Request) {
	type presetDTO struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}
	out := make([]presetDTO, 0, 8)
	for _, kv := range comic.PresetLabels() {
		out = append(out, presetDTO{Key: kv[0], Label: kv[1]})
	}
	writeOK(w, map[string]any{"presets": out})
}
