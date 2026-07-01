package domain

import (
	"fmt"
	"strings"
)

// FidelityAnchored là mức bám sát duy nhất của v1: "neo sự thật, tự do sáng tạo".
// Các dữ kiện MustHold là ràng buộc cứng (tên/mốc thời gian/thành tựu/tính cách/bối cảnh),
// phần phiêu lưu/tình tiết vẫn được hư cấu tự do miễn không mâu thuẫn mỏ neo.
const FidelityAnchored = "anchored"

// Dossier là hồ sơ dữ kiện về một nhân vật/chủ thể có thật, dùng làm "mỏ neo sự thật"
// cho chế độ viết bám sát nhân vật thật. Lưu per-novel tại meta/dossier.json.
//
// Nguồn gốc: người dùng dán tư liệu (NormalizeSource) hoặc AI soạn nháp (DraftFromSubject).
// Bản nháp AI dựa vào trí nhớ mô hình nên có thể sai — bắt buộc người duyệt trước khi viết
// (Confirmed). Khi Enabled=false, hồ sơ tồn tại nhưng không được bơm vào ngữ cảnh.
type Dossier struct {
	Enabled    bool          `json:"enabled"`
	Subject    string        `json:"subject"`
	Fidelity   string        `json:"fidelity"`
	Facts      []DossierFact `json:"facts"`
	RawSource  string        `json:"raw_source,omitempty"`
	Draft      bool          `json:"draft"`     // true = do AI sinh, chưa người xác nhận
	Confirmed  bool          `json:"confirmed"` // true khi người dùng đã duyệt & bấm Bắt đầu
	Disclaimer string        `json:"disclaimer,omitempty"`
}

// DossierFact là một dữ kiện đơn về chủ thể.
type DossierFact struct {
	Category string `json:"category"` // tiểu sử | mốc thời gian | thành tựu | tính cách | bối cảnh | chuyên môn
	Fact     string `json:"fact"`
	MustHold bool   `json:"must_hold"` // true = ràng buộc cứng; false = tham khảo mềm
}

// HasContent báo hồ sơ có đủ nội dung để bơm vào ngữ cảnh sáng tác.
func (d *Dossier) HasContent() bool {
	if d == nil || !d.Enabled {
		return false
	}
	if strings.TrimSpace(d.Subject) != "" {
		return true
	}
	for _, f := range d.Facts {
		if strings.TrimSpace(f.Fact) != "" {
			return true
		}
	}
	return false
}

// MustHoldFacts trả về các dữ kiện ràng buộc cứng (mỏ neo bắt buộc giữ đúng).
func (d *Dossier) MustHoldFacts() []DossierFact {
	if d == nil {
		return nil
	}
	var out []DossierFact
	for _, f := range d.Facts {
		if f.MustHold && strings.TrimSpace(f.Fact) != "" {
			out = append(out, f)
		}
	}
	return out
}

// SoftFacts trả về các dữ kiện tham khảo mềm (được phép uyển chuyển khi hư cấu).
func (d *Dossier) SoftFacts() []DossierFact {
	if d == nil {
		return nil
	}
	var out []DossierFact
	for _, f := range d.Facts {
		if !f.MustHold && strings.TrimSpace(f.Fact) != "" {
			out = append(out, f)
		}
	}
	return out
}

// Payload tạo bản đồ gọn để bơm vào novel_context (working_memory.fact_dossier).
// Tách rõ mỏ neo bắt buộc (must_hold) với dữ kiện tham khảo, kèm chỉ dẫn ngắn để LLM
// hiểu cách dùng: giữ đúng mỏ neo, tự do hư cấu phần còn lại.
func (d *Dossier) Payload() map[string]any {
	if d == nil {
		return nil
	}
	payload := map[string]any{
		"subject":  d.Subject,
		"fidelity": d.Fidelity,
		"usage": "Đây là mỏ neo sự thật về nhân vật/chủ thể có thật. must_hold là ràng buộc CỨNG " +
			"(tên thật, mốc thời gian, thành tựu, tính cách, bối cảnh thời đại) — không được viết mâu thuẫn. " +
			"reference là dữ kiện tham khảo mềm. Phần phiêu lưu/tình tiết vẫn hư cấu tự do miễn không phá vỡ mỏ neo.",
	}
	if must := d.MustHoldFacts(); len(must) > 0 {
		items := make([]string, 0, len(must))
		for _, f := range must {
			items = append(items, factLine(f))
		}
		payload["must_hold"] = items
	}
	if soft := d.SoftFacts(); len(soft) > 0 {
		items := make([]string, 0, len(soft))
		for _, f := range soft {
			items = append(items, factLine(f))
		}
		payload["reference"] = items
	}
	if d.Draft && strings.TrimSpace(d.Disclaimer) != "" {
		payload["disclaimer"] = d.Disclaimer
	}
	return payload
}

func factLine(f DossierFact) string {
	cat := strings.TrimSpace(f.Category)
	fact := strings.TrimSpace(f.Fact)
	if cat == "" {
		return fact
	}
	return "[" + cat + "] " + fact
}

// RenderMarkdown kết xuất hồ sơ thành Markdown dễ đọc để người dùng duyệt/sửa
// (dùng cho bản nháp AI trả về giao diện web).
func (d *Dossier) RenderMarkdown() string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	subject := strings.TrimSpace(d.Subject)
	if subject == "" {
		subject = "(chưa đặt tên chủ thể)"
	}
	fmt.Fprintf(&b, "# Hồ sơ nhân vật thật: %s\n\n", subject)
	if strings.TrimSpace(d.Disclaimer) != "" {
		fmt.Fprintf(&b, "> ⚠ %s\n\n", strings.TrimSpace(d.Disclaimer))
	}
	if must := d.MustHoldFacts(); len(must) > 0 {
		b.WriteString("## Mỏ neo bắt buộc giữ đúng\n")
		for _, f := range must {
			fmt.Fprintf(&b, "- %s\n", factLine(f))
		}
		b.WriteString("\n")
	}
	if soft := d.SoftFacts(); len(soft) > 0 {
		b.WriteString("## Dữ kiện tham khảo\n")
		for _, f := range soft {
			fmt.Fprintf(&b, "- %s\n", factLine(f))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
