package adapt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Run khởi động một lần chuyển thể sách → sản phẩm video.
// Trả về kênh Event (caller chịu trách nhiệm tiêu thụ; hủy ctx để dừng sạch).
// Giống imp/sim: LLM-nặng, nhiều bước, chỉ đọc store rồi ghi file ngoài.
func Run(ctx context.Context, deps Deps, opts Options) (<-chan Event, error) {
	if deps.Store == nil || deps.LLM == nil {
		return nil, fmt.Errorf("deps chưa đầy đủ")
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
		bible, err := loadStoryBible(deps.Store, opts.StyleHint)
		if err != nil {
			emit(Event{Stage: StageError, Message: "Đọc dữ liệu dự án thất bại", Err: err})
			return
		}

		outDir := opts.OutDir
		if outDir == "" {
			outDir = filepath.Join(deps.Store.Dir(), "video")
		}
		var outputs []Output
		rc := &runCtx{deps: deps, bible: bible, opts: opts, outDir: outDir, emit: emit, outputs: &outputs}

		for _, p := range products {
			if err := ctx.Err(); err != nil {
				emit(Event{Stage: StageError, Product: p, Message: "Người dùng đã hủy chuyển thể", Err: err})
				return
			}
			var perr error
			switch p {
			case ProductConcept:
				perr = runConcept(ctx, rc)
			case ProductCharacter:
				perr = runCharacter(ctx, rc)
			case ProductProp:
				perr = runProp(ctx, rc)
			case ProductConsistency:
				perr = runConsistency(ctx, rc)
			case ProductScreenplay:
				perr = runScreenplay(ctx, rc)
			case ProductStoryboard:
				perr = runStoryboard(ctx, rc)
			case ProductAnimation:
				perr = runAnimation(ctx, rc)
			case ProductImagePrompt:
				perr = runImagePrompt(ctx, rc)
			case ProductVideoPrompt:
				perr = runVideoPrompt(ctx, rc)
			}
			if perr != nil {
				emit(Event{Stage: StageError, Product: p, Message: "Bước " + string(p) + " thất bại", Err: perr})
				return
			}
		}

		emit(Event{Stage: StageDone, Message: fmt.Sprintf("Hoàn thành chuyển thể: đã ghi %d tệp vào %s", len(outputs), outDir)})
	}()
	return events, nil
}

// runCtx gom trạng thái dùng chung giữa các bước trong một lần chạy.
type runCtx struct {
	deps    Deps
	bible   *storyBible
	opts    Options
	outDir  string
	emit    func(Event)
	outputs *[]Output

	// Bộ nhớ đệm trong phiên, tránh đọc lại đĩa; downstream đọc đĩa nếu rỗng.
	concept     *ConceptResult
	characters  []CharacterDesign
	props       *PropResult
	consistency *ConsistencyBible
}

// write ghi một file (tôn trọng cờ Overwrite). Trả về skipped=true nếu bỏ qua.
func (rc *runCtx) write(product Product, relPath string, data []byte) (bool, error) {
	path := filepath.Join(rc.outDir, relPath)
	if !rc.opts.Overwrite && exists(path) {
		return true, nil
	}
	n, err := atomicWrite(path, data)
	if err != nil {
		return false, err
	}
	*rc.outputs = append(*rc.outputs, Output{Product: product, Path: path, Bytes: n})
	return false, nil
}

// writeJSON marshal + ghi file JSON.
func (rc *runCtx) writeJSON(product Product, relPath string, v any) (bool, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return false, err
	}
	return rc.write(product, relPath, data)
}

// loadArtifact đọc một artifact JSON đã ghi trước đó (best-effort).
func (rc *runCtx) loadArtifact(relPath string, out any) bool {
	data, err := os.ReadFile(filepath.Join(rc.outDir, relPath))
	if err != nil {
		return false
	}
	return json.Unmarshal(data, out) == nil
}

func (rc *runCtx) ensureConcept() *ConceptResult {
	if rc.concept != nil {
		return rc.concept
	}
	var c ConceptResult
	if rc.loadArtifact("concept/art-direction.json", &c) {
		rc.concept = &c
	}
	return rc.concept
}

func (rc *runCtx) ensureCharacters() []CharacterDesign {
	if rc.characters != nil {
		return rc.characters
	}
	var list []CharacterDesign
	if rc.loadArtifact("characters/characters.json", &list) {
		rc.characters = list
	}
	return rc.characters
}

func (rc *runCtx) ensureProps() *PropResult {
	if rc.props != nil {
		return rc.props
	}
	var p PropResult
	if rc.loadArtifact("props/props.json", &p) {
		rc.props = &p
	}
	return rc.props
}

func (rc *runCtx) ensureConsistency() *ConsistencyBible {
	if rc.consistency != nil {
		return rc.consistency
	}
	var cb ConsistencyBible
	if rc.loadArtifact("consistency-bible.json", &cb) {
		rc.consistency = &cb
	}
	return rc.consistency
}

// visualBible gói "style bible" tiêm vào prompt storyboard để giữ nhất quán hình ảnh.
func (rc *runCtx) visualBible() map[string]any {
	vb := map[string]any{}
	if c := rc.ensureConcept(); c != nil {
		vb["style_tokens"] = c.Style.StyleTokens
		vb["locations"] = c.Locations
	}
	if cb := rc.ensureConsistency(); cb != nil {
		vb["consistency"] = cb
	} else {
		if chars := rc.ensureCharacters(); len(chars) > 0 {
			vb["characters"] = chars
		}
		if props := rc.ensureProps(); props != nil {
			vb["props"] = props.Props
		}
	}
	return vb
}
