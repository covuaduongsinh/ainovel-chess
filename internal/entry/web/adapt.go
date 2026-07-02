package web

import (
	"net/http"
	"strings"

	"github.com/voocel/ainovel-cli/internal/host/adapt"
)

// adaptRequest là body cho POST /api/adapt (mirror TUI /video).
type adaptRequest struct {
	Products  []string `json:"products"` // rỗng = tất cả (DefaultOrder)
	From      int      `json:"from"`
	To        int      `json:"to"`
	OutDir    string   `json:"outDir"`
	Style     string   `json:"style"`
	Overwrite bool     `json:"overwrite"`
}

func (s *Server) handleAdapt(w http.ResponseWriter, r *http.Request) {
	var req adaptRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var products []adapt.Product
	for _, p := range req.Products {
		p = strings.TrimSpace(p)
		if p == "" || p == "all" {
			continue
		}
		products = append(products, adapt.Product(p))
	}
	opts := adapt.Options{
		Products:  products,
		From:      req.From,
		To:        req.To,
		OutDir:    strings.TrimSpace(req.OutDir),
		StyleHint: strings.TrimSpace(req.Style),
		Overwrite: req.Overwrite,
	}

	id, ctx := s.jobs.start()
	ch, err := s.eng.Adapt(ctx, opts)
	if err != nil {
		s.jobs.finish(id)
		writeErr(w, http.StatusConflict, err)
		return
	}
	go func() {
		defer s.jobs.finish(id)
		for ev := range ch {
			done := ev.Stage == adapt.StageDone || ev.Stage == adapt.StageError
			s.emitProgress(progressDTO{
				Job: "adapt", ID: id, Stage: string(ev.Stage),
				Current: ev.Current, Total: ev.Total, Message: ev.Message,
				Error: errString(ev.Err), Done: done,
			})
		}
	}()
	writeOK(w, map[string]any{"id": id})
}
