package web

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/host/comic"
	"github.com/voocel/ainovel-cli/internal/imggen"
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
	ArtPass   string   `json:"artPass"`
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
		ArtPass:     comic.ArtPass(strings.TrimSpace(req.ArtPass)),
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

// comicConfigDTO là cấu hình sinh ảnh gửi ra/vào trình duyệt. Khoá luôn được CHE khi gửi ra.
type comicConfigDTO struct {
	APIKey     string `json:"apiKey"`
	BaseURL    string `json:"baseUrl"`
	Model      string `json:"model"`
	Dialect    string `json:"dialect"`
	ImageSize  string `json:"imageSize"`
	KeyInQuery bool   `json:"keyInQuery"`
	Enabled    bool   `json:"enabled"`
}

func (s *Server) handleComicConfig(w http.ResponseWriter, _ *http.Request) {
	cc := s.eng.ComicConfig()
	writeOK(w, map[string]any{
		"config": comicConfigDTO{
			APIKey:     bootstrap.MaskSecret(cc.APIKey),
			BaseURL:    cc.BaseURL,
			Model:      cc.Model,
			Dialect:    cc.Dialect,
			ImageSize:  cc.ImageSize,
			KeyInQuery: cc.KeyInQuery,
			Enabled:    cc.ImageEnabled(),
		},
		"models": s.eng.ComicImageModels(),
	})
}

func (s *Server) handleComicConfigSave(w http.ResponseWriter, r *http.Request) {
	var req comicConfigDTO
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// GHÉP với cấu hình hiện tại, KHÔNG dựng lại từ DTO.
	// Giao diện chỉ gửi vài trường nó có ô nhập; dựng lại từ DTO sẽ âm thầm xoá sạch những
	// trường còn lại (base_url, image_size…) mỗi lần bấm Lưu.
	cc := s.eng.ComicConfig()
	// mergeSecret: rỗng hoặc chứa "****" = giữ khoá cũ; "-" = xoá.
	cc.APIKey = mergeSecret(cc.APIKey, req.APIKey)
	if v := strings.TrimSpace(req.BaseURL); v != "" {
		cc.BaseURL = v
	}
	if v := strings.TrimSpace(req.Model); v != "" {
		cc.Model = v
	}
	if v := strings.TrimSpace(req.Dialect); v != "" {
		cc.Dialect = v
	}
	if v := strings.TrimSpace(req.ImageSize); v != "" {
		cc.ImageSize = v
	}
	cc.KeyInQuery = req.KeyInQuery
	if err := s.eng.SetComicConfig(cc); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeOK(w, map[string]any{"enabled": cc.ImageEnabled()})
}

// handleComicTestImage sinh ĐÚNG MỘT ảnh nhỏ để kiểm chứng khoá và phương ngữ dây.
//
// Bắt buộc phải chạy trước khi sinh cả sách: Google đang có hai bề mặt API cho việc sinh
// ảnh, chạy cả cuốn rồi mới phát hiện sai phương ngữ thì mất cả trăm đô.
func (s *Server) handleComicTestImage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	data, mime, err := s.eng.ComicTestImage(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	if mime == "" {
		mime = "image/png"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// handleComicEstimate ước tính chi phí bằng SỐ KHUNG THẬT đã có trong prompts/, thay vì
// giả định thô. Quan trọng: chi phí thật là số khung CÒN THIẾU, không phải tổng số khung —
// cơ chế bỏ-qua-nếu-đã-có làm việc chạy lại gần như miễn phí.
func (s *Server) handleComicEstimate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	atoi := func(k string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(q.Get(k)))
		return n
	}
	cc := s.eng.ComicConfig()
	size := strings.TrimSpace(q.Get("size"))
	model := strings.TrimSpace(q.Get("model"))
	if model == "" {
		model = cc.Model
	}

	est, err := s.eng.ComicEstimate(atoi("from"), atoi("to"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	price, hasPrice := imggen.PriceFor(model, size)
	caps, hasCaps := imggen.CapsFor(model)

	out := map[string]any{
		"hasData":      est.HasData,
		"chapters":     est.Chapters,
		"totalPanels":  est.TotalPanels,
		"missing":      est.MissingPanels,
		"model":        model,
		"size":         size,
		"supportsSize": imggen.SupportsSize(model, size),
	}
	if hasCaps {
		out["modelSizes"] = caps.Sizes
		out["modelNote"] = caps.Note
	}
	if hasPrice {
		out["usdPerImage"] = price
		out["usdMissing"] = price * float64(est.MissingPanels)
	}
	writeOK(w, out)
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
