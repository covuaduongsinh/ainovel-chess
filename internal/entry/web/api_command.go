package web

import (
	"context"
	"net/http"
	"time"

	"github.com/voocel/ainovel-cli/internal/diag"
	"github.com/voocel/ainovel-cli/internal/host/exp"
	"github.com/voocel/ainovel-cli/internal/store"
)

type exportResult struct {
	Path     string `json:"path"`
	Chapters int    `json:"chapters"`
	Bytes    int    `json:"bytes"`
	Skipped  []int  `json:"skipped"`
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	var req exportRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.eng.Export(ctx, exp.Options{
		OutPath:   req.Path,
		From:      req.From,
		To:        req.To,
		Overwrite: req.Overwrite,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeOK(w, exportResult{
		Path:     res.Path,
		Chapters: res.Chapters,
		Bytes:    res.Bytes,
		Skipped:  res.Skipped,
	})
}

type diagResult struct {
	Report     diag.Report `json:"report"`
	ExportPath string      `json:"exportPath"`
}

// handleDiag chạy chẩn đoán đồng bộ trên thư mục output (vài giây), trả về báo cáo có cấu trúc cho frontend hiển thị.
// Tái sử dụng đường dẫn loadReport của TUI: Diagnose + WriteExport.
func (s *Server) handleDiag(w http.ResponseWriter, _ *http.Request) {
	st := store.NewStore(s.eng.Dir())
	rep, rc := diag.Diagnose(st)
	exportPath, _ := diag.WriteExport(st, rep, rc)
	writeOK(w, diagResult{Report: rep, ExportPath: exportPath})
}
