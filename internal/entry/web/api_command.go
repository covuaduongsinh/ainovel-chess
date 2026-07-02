package web

import (
	"context"
	"net/http"
	"time"

	"github.com/voocel/ainovel-cli/internal/diag"
	"github.com/voocel/ainovel-cli/internal/host/exp"
	"github.com/voocel/ainovel-cli/internal/store"
)

type exportFile struct {
	Format string `json:"format"`
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
}

type exportResult struct {
	Files    []exportFile `json:"files"`
	Chapters int          `json:"chapters"`
	Skipped  []int        `json:"skipped"`
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
		Formats:   exp.FormatsForPath(req.Path),
		From:      req.From,
		To:        req.To,
		Overwrite: req.Overwrite,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	files := make([]exportFile, 0, len(res.Outputs))
	for _, o := range res.Outputs {
		files = append(files, exportFile{Format: string(o.Format), Path: o.Path, Bytes: o.Bytes})
	}
	writeOK(w, exportResult{
		Files:    files,
		Chapters: res.Chapters,
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
