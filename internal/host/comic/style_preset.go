package comic

import "strings"

// StylePreset là một bộ phong cách vẽ dựng sẵn. Người dùng chọn preset trên UI rồi có thể
// nhập thêm chữ tự do (Options.StyleHint) để tinh chỉnh — hint GHÉP THÊM, không thay thế.
type StylePreset struct {
	Key      string
	Label    string // hiển thị cho người dùng (VI)
	Tokens   string // token phong cách (EN) chèn vào MỌI prompt khung
	Negative string // negative riêng của phong cách (EN)
}

// commonNegative là negative dùng chung cho mọi preset.
//
// Nhóm đầu CẤM CHỮ trong tranh — bắt buộc, vì chữ do bộ dàn trang vẽ bằng font thật.
// Mô hình sinh ảnh viết tiếng Việt có dấu gần như luôn sai (xem docs/comic.md), nên để nó
// vẽ chữ là hỏng cả trang. Nhóm sau là các lỗi giải phẫu hay gặp nhất.
const commonNegative = "text, letters, words, writing, caption, speech bubble, speech balloon, " +
	"empty speech balloon, thought bubble, callout box, " +
	// Mô hình rất hay TỰ VẼ khung viền và lề trắng quanh tranh khi prompt có chữ "panel" —
	// rồi bộ dàn trang lại vẽ viền lần nữa, thành khung lồng khung. Cấm thẳng ở đây.
	"panel border, comic panel frame, picture frame, border, framed illustration, " +
	"white margin, page layout, multiple panels, grid of panels, " +
	"watermark, signature, logo, " +
	"bad anatomy, bad hands, extra fingers, missing fingers, deformed face, extra limbs, " +
	"blurry, low quality, jpeg artifacts"

// StylePresets là bảng preset. Thứ tự hiển thị trên UI lấy theo PresetOrder.
var StylePresets = map[string]StylePreset{
	"thieu-nhi": {
		Key:   "thieu-nhi",
		Label: "Thiếu nhi — màu nước ấm",
		Tokens: "warm children's storybook illustration, soft watercolor and pastel textures, " +
			"gentle rounded shapes, cozy hand-drawn charm, warm golden light, muted nostalgic tones, " +
			"clean readable compositions, full color comic book illustration",
		Negative: "harsh contrast, horror, gore, photorealistic, dark grim tones",
	},
	"manga": {
		Key:   "manga",
		Label: "Manga — đen trắng, screentone",
		Tokens: "black and white manga illustration, clean confident ink linework, screentone shading, " +
			"expressive character acting, dynamic speed lines, high contrast, manga illustration",
		Negative: "color, colored, watercolor, painterly, photorealistic",
	},
	"au-my": {
		Key:   "au-my",
		Label: "Âu-Mỹ — nét đậm, màu rực",
		Tokens: "western comic book illustration, bold confident inking, heavy black shadows, " +
			"vibrant saturated flat colors, cel shading, dynamic heroic composition, comic book illustration",
		Negative: "manga, anime, watercolor, pastel, muted colors, photorealistic",
	},
	"ta-thuc": {
		Key:   "ta-thuc",
		Label: "Tả thực — tranh sơn dầu",
		Tokens: "realistic painted illustration, detailed rendering, natural proportions, " +
			"cinematic lighting, rich oil-painting texture, graphic novel illustration",
		Negative: "cartoon, anime, manga, chibi, flat colors, sketch",
	},
	"ky-hoa": {
		Key:   "ky-hoa",
		Label: "Ký hoạ — chì, đen trắng",
		Tokens: "pencil sketch illustration, graphite hatching and cross-hatching, " +
			"loose expressive linework, monochrome, textured paper feel, pencil illustration",
		Negative: "color, colored, digital painting, flat vector, photorealistic",
	},
}

// PresetOrder là thứ tự hiển thị ổn định (map trong Go duyệt ngẫu nhiên).
var PresetOrder = []string{"thieu-nhi", "manga", "au-my", "ta-thuc", "ky-hoa"}

// DefaultPreset là preset dùng khi người dùng không chọn.
const DefaultPreset = "thieu-nhi"

// resolvePreset tra preset theo khoá, rơi về mặc định khi khoá rỗng/không hợp lệ.
func resolvePreset(key string) StylePreset {
	if p, ok := StylePresets[strings.TrimSpace(key)]; ok {
		return p
	}
	return StylePresets[DefaultPreset]
}

// IsMonochrome cho biết preset là đen trắng — dùng để chọn màu bong bóng và
// quyết định có nên nhắc mô hình bỏ màu hay không.
func (p StylePreset) IsMonochrome() bool {
	return p.Key == "manga" || p.Key == "ky-hoa"
}

// negativeFor gộp negative dùng chung + negative của preset + negative riêng của khung.
func (p StylePreset) negativeFor(panelNegative string) string {
	parts := []string{commonNegative}
	if s := strings.TrimSpace(p.Negative); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(panelNegative); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

// PresetLabels trả về danh sách (khoá, nhãn) theo thứ tự hiển thị — cho Web/TUI dùng chung.
func PresetLabels() [][2]string {
	out := make([][2]string, 0, len(PresetOrder))
	for _, k := range PresetOrder {
		if p, ok := StylePresets[k]; ok {
			out = append(out, [2]string{p.Key, p.Label})
		}
	}
	return out
}
