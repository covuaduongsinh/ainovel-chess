package imggen

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// ---------------------------------------------------------- generateContent

// generateContentDialect dùng đường `POST /v1beta/models/{model}:generateContent`.
// Tài liệu Google đã gắn nhãn "Legacy" nhưng cả bốn model sinh ảnh vẫn được liệt kê ở đó
// và chưa công bố ngày khai tử — đây vẫn là đường dùng phổ biến nhất.
type generateContentDialect struct{}

func (generateContentDialect) name() string { return "generatecontent" }

func (generateContentDialect) path(model string) string {
	return "/v1beta/models/" + model + ":generateContent"
}

func (generateContentDialect) build(model string, r Request) ([]byte, error) {
	var parts []genPart

	// Ảnh tham chiếu ĐỨNG TRƯỚC, mỗi ảnh kèm một nhãn chữ ngắn; chỉ thị chính đặt CUỐI.
	// Thứ tự "chỉ thị đứng cuối" bám prompt tốt hơn hẳn thứ tự ngược lại.
	for i, ref := range r.Refs {
		label := strings.TrimSpace(ref.Label)
		if label == "" {
			label = fmt.Sprintf("Reference %d", i+1)
		}
		mime := ref.MimeType
		if mime == "" {
			mime = "image/png"
		}
		parts = append(parts,
			genPart{Text: "Reference image — " + label + ". Keep this character's design consistent."},
			genPart{InlineData: &genBlob{MimeType: mime, Data: base64.StdEncoding.EncodeToString(ref.Data)}},
		)
	}

	prompt := strings.TrimSpace(r.Prompt)
	// Gemini KHÔNG có trường negative riêng — diễn đạt thành câu cấm ở cuối prompt.
	if neg := strings.TrimSpace(r.Negative); neg != "" {
		prompt += "\n\nDo NOT include any of the following: " + neg + "."
	}
	parts = append(parts, genPart{Text: prompt})

	cfg := &genConfig{ResponseModalities: []string{"TEXT", "IMAGE"}}
	if r.AspectRatio != "" || r.ImageSize != "" {
		cfg.ImageConfig = &genImageConfig{AspectRatio: r.AspectRatio, ImageSize: r.ImageSize}
	}
	if t := strings.TrimSpace(r.Thinking); t != "" {
		cfg.ThinkingConfig = &genThinking{ThinkingLevel: t}
	}

	return json.Marshal(genRequest{
		Contents:         []genContent{{Role: "user", Parts: parts}},
		GenerationConfig: cfg,
	})
}

func (generateContentDialect) parse(raw []byte) (*Result, error) {
	var resp genResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, &Err{Code: "BAD_RESPONSE", Message: "không giải mã được phản hồi: " + err.Error(), StatusCode: 200}
	}
	// 1) Prompt bị chặn ngay từ đầu vào.
	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != "" {
		return nil, &Err{Code: resp.PromptFeedback.BlockReason,
			Message: "prompt bị bộ lọc chặn (" + resp.PromptFeedback.BlockReason + ")", StatusCode: 200}
	}
	// 2) Không có ứng viên nào.
	if len(resp.Candidates) == 0 {
		return nil, &Err{Code: "NO_CANDIDATES", Message: "phản hồi không có ứng viên nào", StatusCode: 200}
	}
	cand := resp.Candidates[0]
	// 3) Kết thúc vì lý do an toàn.
	if isSafetyCode(cand.FinishReason) {
		return nil, &Err{Code: cand.FinishReason,
			Message: "ảnh bị chặn (" + cand.FinishReason + ")", StatusCode: 200}
	}
	// 4) Duyệt part, gom chữ và lấy ảnh đầu tiên.
	out := &Result{FinishReason: cand.FinishReason, TotalTokens: resp.UsageMetadata.TotalTokenCount}
	var text strings.Builder
	for _, p := range cand.Content.Parts {
		if p.Text != "" {
			text.WriteString(p.Text)
		}
		if p.InlineData != nil && p.InlineData.Data != "" && out.Image == nil {
			data, err := base64.StdEncoding.DecodeString(p.InlineData.Data)
			if err != nil {
				return nil, &Err{Code: "BAD_IMAGE", Message: "giải mã base64 ảnh thất bại: " + err.Error(), StatusCode: 200}
			}
			out.Image, out.MimeType = data, p.InlineData.MimeType
		}
	}
	out.Text = strings.TrimSpace(text.String())
	if len(out.Image) == 0 {
		msg := "phản hồi không chứa ảnh"
		if out.Text != "" {
			msg += " (mô hình trả về chữ: " + truncate(out.Text, 200) + ")"
		}
		return nil, &Err{Code: "NO_IMAGE", Message: msg, StatusCode: 200}
	}
	return out, nil
}

// ---------------------------------------------------------- interactions

// interactionsDialect dùng đường `POST /v1beta/interactions` — bề mặt API mới hơn mà tài
// liệu Google hiện giới thiệu. Giữ ở đây để đổi được bằng cấu hình khi đường cũ ngừng chạy.
//
// ⚠ CHƯA kiểm chứng trên tài khoản thật. Nếu đường mặc định lỗi, thử đặt
// comic.image.dialect = "interactions" rồi bấm "Kiểm tra kết nối".
type interactionsDialect struct{}

func (interactionsDialect) name() string        { return "interactions" }
func (interactionsDialect) path(string) string  { return "/v1beta/interactions" }

type interReq struct {
	Model          string      `json:"model"`
	Input          []interPart `json:"input"`
	ResponseFormat *interFmt   `json:"response_format,omitempty"`
}

type interPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Data     string `json:"data,omitempty"`
}

type interFmt struct {
	Type        string `json:"type"`
	MimeType    string `json:"mime_type,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	ImageSize   string `json:"image_size,omitempty"`
}

type interResp struct {
	Steps []struct {
		Type    string `json:"type"`
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			MimeType string `json:"mime_type,omitempty"`
			Data     string `json:"data,omitempty"`
		} `json:"content"`
	} `json:"steps"`
}

func (interactionsDialect) build(model string, r Request) ([]byte, error) {
	var in []interPart
	for i, ref := range r.Refs {
		label := strings.TrimSpace(ref.Label)
		if label == "" {
			label = fmt.Sprintf("Reference %d", i+1)
		}
		mime := ref.MimeType
		if mime == "" {
			mime = "image/png"
		}
		in = append(in,
			interPart{Type: "text", Text: "Reference image — " + label + ". Keep this character's design consistent."},
			interPart{Type: "image", MimeType: mime, Data: base64.StdEncoding.EncodeToString(ref.Data)},
		)
	}
	prompt := strings.TrimSpace(r.Prompt)
	if neg := strings.TrimSpace(r.Negative); neg != "" {
		prompt += "\n\nDo NOT include any of the following: " + neg + "."
	}
	in = append(in, interPart{Type: "text", Text: prompt})

	return json.Marshal(interReq{
		Model: model,
		Input: in,
		ResponseFormat: &interFmt{
			Type:        "image",
			MimeType:    "image/png",
			AspectRatio: r.AspectRatio,
			ImageSize:   r.ImageSize,
		},
	})
}

func (interactionsDialect) parse(raw []byte) (*Result, error) {
	var resp interResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, &Err{Code: "BAD_RESPONSE", Message: "không giải mã được phản hồi: " + err.Error(), StatusCode: 200}
	}
	out := &Result{}
	var text strings.Builder
	for _, st := range resp.Steps {
		for _, c := range st.Content {
			if c.Text != "" {
				text.WriteString(c.Text)
			}
			if c.Data != "" && out.Image == nil {
				data, err := base64.StdEncoding.DecodeString(c.Data)
				if err != nil {
					return nil, &Err{Code: "BAD_IMAGE", Message: "giải mã base64 ảnh thất bại: " + err.Error(), StatusCode: 200}
				}
				mime := c.MimeType
				if mime == "" {
					mime = "image/png"
				}
				out.Image, out.MimeType = data, mime
			}
		}
	}
	out.Text = strings.TrimSpace(text.String())
	if len(out.Image) == 0 {
		return nil, &Err{Code: "NO_IMAGE", Message: "phản hồi không chứa ảnh", StatusCode: 200}
	}
	return out, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
