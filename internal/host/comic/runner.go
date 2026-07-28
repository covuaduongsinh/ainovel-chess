package comic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/comicdraw"
)

// Run khởi động một lần chuyển thể sách → trang truyện tranh.
// Trả về kênh Event (caller chịu trách nhiệm tiêu thụ; hủy ctx để dừng sạch).
func Run(ctx context.Context, deps Deps, opts Options) (<-chan Event, error) {
	if deps.Store == nil || deps.LLM == nil {
		return nil, fmt.Errorf("deps chưa đầy đủ")
	}
	if deps.Fonts == nil {
		return nil, fmt.Errorf("thiếu bộ font — không lồng chữ được")
	}
	products := opts.Products
	if len(products) == 0 {
		products = DefaultOrder()
	}
	for _, p := range products {
		if !isKnownProduct(p) {
			return nil, fmt.Errorf("sản phẩm không hợp lệ: %q", p)
		}
	}
	for _, f := range opts.Formats {
		if !isKnownFormat(f) {
			return nil, fmt.Errorf("định dạng không hợp lệ: %q", f)
		}
	}

	events := make(chan Event, 32)
	go func() {
		defer close(events)
		emit := func(ev Event) {
			ev.Time = time.Now()
			select {
			case events <- ev:
			case <-ctx.Done():
			}
		}

		emit(Event{Stage: StageContext, Message: "Đang đọc dữ liệu dự án..."})
		bible, err := loadStoryBible(deps.Store, opts)
		if err != nil {
			emit(Event{Stage: StageError, Message: "Đọc dữ liệu dự án thất bại", Err: err})
			return
		}
		if bible.hasVideoAssets() {
			emit(Event{Stage: StageContext, Message: "Đã tìm thấy tài sản /video — sẽ tái dùng để giữ nhất quán hình ảnh"})
		}

		outDir := strings.TrimSpace(opts.OutDir)
		if outDir == "" {
			outDir = filepath.Join(deps.Store.Dir(), "truyen-tranh")
		}
		spec := comicdraw.SpecFor(opts.PageSize)

		var outputs []Output
		rc := &runCtx{deps: deps, bible: bible, opts: opts, outDir: outDir, spec: spec,
			emit: emit, outputs: &outputs}

		sel := make(map[Product]bool, len(products))
		for _, p := range products {
			if needsImageSource(p) && deps.Img == nil {
				// Giai đoạn 1: không có nguồn ảnh — báo tin, KHÔNG coi là lỗi.
				emit(Event{Stage: StageContext, Product: p,
					Message: "Bỏ qua bước " + string(p) + " (chưa cấu hình nguồn sinh ảnh) — khung sẽ dùng ảnh giữ chỗ"})
				continue
			}
			sel[p] = true
		}

		// 1) Cấp sách.
		if sel[ProductStyle] {
			if err := runStyle(ctx, rc); err != nil {
				emit(Event{Stage: StageError, Product: ProductStyle, Message: "Bước định hướng mỹ thuật thất bại", Err: err})
				return
			}
		}
		if sel[ProductCharacter] {
			if err := runCharacter(ctx, rc); err != nil {
				emit(Event{Stage: StageError, Product: ProductCharacter, Message: "Bước model sheet nhân vật thất bại", Err: err})
				return
			}
		}

		// 2) Cấp chương, duyệt theo tập → chương để tập trước hoàn chỉnh trước.
		if err := runChapterPipeline(ctx, rc, sel); err != nil {
			if errors.Is(err, context.Canceled) {
				emit(Event{Stage: StageError, Message: "Người dùng đã hủy", Err: err})
			} else {
				emit(Event{Stage: StageError, Message: "Đường ống theo chương thất bại", Err: err})
			}
			return
		}

		// 3) Đóng gói xuất bản cả sách.
		if sel[ProductPublish] {
			if err := runPublish(ctx, rc); err != nil {
				emit(Event{Stage: StageError, Product: ProductPublish, Message: "Đóng gói xuất bản thất bại", Err: err})
				return
			}
		}

		if err := rc.writeIndex(); err != nil {
			emit(Event{Stage: StageError, Message: "Ghi mục lục thất bại", Err: err})
			return
		}
		emit(Event{Stage: StageDone,
			Message: fmt.Sprintf("Hoàn thành: đã ghi %d tệp vào %s", len(outputs), outDir)})
	}()
	return events, nil
}

// runChapterPipeline chạy các bước cấp-chương theo thứ tự tập → chương.
// Mỗi chương làm trọn kịch bản → bố cục → prompt → dàn trang rồi ghi đóng gói lồng ngay.
func runChapterPipeline(ctx context.Context, rc *runCtx, sel map[Product]bool) error {
	any := sel[ProductScript] || sel[ProductLayout] || sel[ProductPanelPrompt] ||
		sel[ProductPanelArt] || sel[ProductPage]
	if !any {
		return nil
	}

	groups, skipped := rc.bible.volumeGroupsForRange(rc.opts.From, rc.opts.To)
	if len(groups) == 0 {
		rc.emit(Event{Stage: StageContext, Message: "Không có chương hoàn thành nào trong phạm vi"})
		return nil
	}
	if len(skipped) > 0 {
		rc.emit(Event{Stage: StageContext,
			Message: fmt.Sprintf("Bỏ qua %d chương chưa hoàn thành trong phạm vi", len(skipped))})
	}

	total := 0
	for _, g := range groups {
		total += len(g.Chapters)
	}

	idx := 0
	for _, g := range groups {
		for _, ch := range g.Chapters {
			if err := ctx.Err(); err != nil {
				return err
			}
			idx++
			entry := rc.bible.outlineFor(ch)
			label := fmt.Sprintf("Tập %d · Chương %d — %s", g.Index, ch, entry.Title)

			var script *ScriptResult
			if sel[ProductScript] {
				rc.emit(Event{Stage: StageScript, Current: idx, Total: total, Message: label + ": kịch bản"})
				r, err := scriptChapter(ctx, rc, ch)
				if err != nil {
					return err
				}
				script = r
			}
			if script == nil {
				script = rc.loadScript(ch)
			}
			if script == nil {
				continue
			}

			var layouts []PageLayout
			if sel[ProductLayout] {
				rc.emit(Event{Stage: StageLayout, Current: idx, Total: total, Message: label + ": bố cục trang"})
				var err error
				layouts, err = rc.computeAndWriteLayout(ch, script)
				if err != nil {
					return err
				}
			} else {
				layouts = rc.loadLayouts(ch, script)
			}

			if sel[ProductPanelPrompt] {
				rc.emit(Event{Stage: StagePanelPrompt, Current: idx, Total: total, Message: label + ": prompt khung"})
				if err := rc.writePanelPrompts(ch, script); err != nil {
					return err
				}
			}

			if sel[ProductPanelArt] {
				rc.emit(Event{Stage: StagePanelArt, Current: idx, Total: total, Message: label + ": sinh tranh khung"})
				if err := rc.generatePanelArt(ctx, ch); err != nil {
					return err
				}
			}

			if sel[ProductPage] {
				rc.emit(Event{Stage: StagePage, Current: idx, Total: total, Message: label + ": dàn trang"})
				if err := rc.renderChapterPages(ch, g.Index, script, layouts); err != nil {
					return err
				}
			}
		}
		rc.emit(Event{Stage: StagePage, Message: fmt.Sprintf("Hoàn tất Tập %d (%d chương)", g.Index, len(g.Chapters))})
	}
	return nil
}

// runCtx gom trạng thái dùng chung giữa các bước trong một lần chạy.
type runCtx struct {
	deps    Deps
	bible   *storyBible
	opts    Options
	outDir  string
	spec    comicdraw.PageSpec
	emit    func(Event)
	outputs *[]Output

	// Bộ nhớ đệm trong phiên; downstream đọc đĩa nếu rỗng.
	style      *StyleResult
	characters []CharacterSheet

	imagesMade int // đếm ảnh đã sinh, để tôn trọng Options.MaxImages
}

// path trả về đường dẫn tuyệt đối của một tệp trong outDir.
func (rc *runCtx) path(rel string) string {
	return filepath.Join(rc.outDir, filepath.FromSlash(rel))
}

// write ghi một file (tôn trọng cờ Overwrite). Trả skipped=true nếu bỏ qua.
func (rc *runCtx) write(product Product, rel string, data []byte) (bool, error) {
	p := rc.path(rel)
	if !rc.opts.Overwrite && exists(p) {
		return true, nil
	}
	n, err := atomicWrite(p, data)
	if err != nil {
		return false, err
	}
	*rc.outputs = append(*rc.outputs, Output{Product: product, Path: p, Bytes: n})
	return false, nil
}

// writeAlways ghi bất kể cờ Overwrite — dùng cho tệp phái sinh rẻ (mục lục, bảng prompt).
func (rc *runCtx) writeAlways(product Product, rel string, data []byte) error {
	p := rc.path(rel)
	n, err := atomicWrite(p, data)
	if err != nil {
		return err
	}
	*rc.outputs = append(*rc.outputs, Output{Product: product, Path: p, Bytes: n})
	return nil
}

func (rc *runCtx) writeJSON(product Product, rel string, v any) (bool, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return false, err
	}
	return rc.write(product, rel, data)
}

// loadArtifact đọc một artifact JSON đã ghi trước đó (best-effort).
func (rc *runCtx) loadArtifact(rel string, out any) bool {
	data, err := os.ReadFile(rc.path(rel))
	if err != nil {
		return false
	}
	return json.Unmarshal(data, out) == nil
}

func (rc *runCtx) loadScript(ch int) *ScriptResult {
	var s ScriptResult
	if rc.loadArtifact(fmt.Sprintf("kich-ban/%02d.json", ch), &s) && len(s.Pages) > 0 {
		if s.Chapter == 0 {
			s.Chapter = ch
		}
		return &s
	}
	return nil
}

func (rc *runCtx) ensureStyle() *StyleResult {
	if rc.style != nil {
		return rc.style
	}
	var s StyleResult
	if rc.loadArtifact("style/art-direction.json", &s) {
		rc.style = &s
	}
	return rc.style
}

func (rc *runCtx) ensureCharacters() []CharacterSheet {
	if rc.characters != nil {
		return rc.characters
	}
	var list []CharacterSheet
	if rc.loadArtifact("nhan-vat/characters.json", &list) {
		rc.characters = list
	}
	return rc.characters
}

// styleTokens trả về token phong cách hiệu lực: ưu tiên bước style, rơi về preset.
func (rc *runCtx) styleTokens() []string {
	if s := rc.ensureStyle(); s != nil && len(s.StyleTokens) > 0 {
		return s.StyleTokens
	}
	return strings.Split(rc.bible.Preset.Tokens, ", ")
}

// negativeTokens gộp negative dùng chung + preset + bước style.
func (rc *runCtx) negativeTokens(panelNegative string) string {
	base := rc.bible.Preset.negativeFor(panelNegative)
	if s := rc.ensureStyle(); s != nil && len(s.Negative) > 0 {
		base += ", " + strings.Join(s.Negative, ", ")
	}
	return base
}
