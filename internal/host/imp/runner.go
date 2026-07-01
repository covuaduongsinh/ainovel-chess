package imp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/tools"
)

// Deps Truyen cac phu thuoc co the cai vao cua runner mot lan, thuan tien cho mock trong test.
type Deps struct {
	Store      *store.Store
	CommitTool *tools.CommitChapterTool
	LLM        LLMChat // cùng một model là đủ, foundation/analyzer đều là suy ngược có cấu trúc
	Prompts    Prompts
}

// Prompts La hai doan prompt duoc su dung trong quy trinh imp.
type Prompts struct {
	Foundation string // Suy nguoc foundation
	Analyzer   string // Suy nguoc tung chuong
}

// Run Thuc hien quy trinh import day du: split -> foundation -> vong lap chuong.
// Chay trong goroutine rieng; kenh Events do ham nay dong.
//
// Lua chon thiet ke:
//   - Quy trinh day du la thuc thi blocking (tac vu dai tren CLI), ben goi chiu trach nhiem mo goroutine lang nghe kenh;
//   - Bat ky buoc nao that bai deu ket thuc ngay, phat su kien StageError;
//   - Giai doan chapter bo qua im lang cac chuong da hoan thanh (idempotent cua commit_chapter la luoi an toan, nhung bo qua LLM tiet kiem token hon).
func Run(ctx context.Context, deps Deps, opts Options) (<-chan Event, error) {
	if deps.Store == nil || deps.CommitTool == nil || deps.LLM == nil {
		return nil, fmt.Errorf("deps incomplete")
	}
	if strings.TrimSpace(opts.SourcePath) == "" {
		return nil, fmt.Errorf("source path is required")
	}

	events := make(chan Event, 32)

	go func() {
		defer close(events)
		emit := func(stage Stage, current, total int, msg string, err error) {
			ev := Event{Time: time.Now(), Stage: stage, Current: current, Total: total, Message: msg, Err: err}
			select {
			case events <- ev:
			case <-ctx.Done():
			}
		}

		// ── 1. Phân tách ──
		emit(StageSplitting, 0, 0, "Đang phân tách chương...", nil)
		chapters, err := SplitFile(opts.SourcePath)
		if err != nil {
			emit(StageError, 0, 0, "Phân tách thất bại", err)
			return
		}
		total := len(chapters)
		if total == 0 {
			emit(StageError, 0, 0,
				"Khong nhan dien duoc chuong nao: Ho tro cac tieu de 'Chuong/Hoi/Tap/Muc N', 'Prologue', 'Epilogue', "+
					"'Chapter N' v.v., tuong thich Markdown #, khoang trang toan goc, bo bao 【】 va ma GBK. "+
					"Vui long xac nhan file la van ban tieu thuyet co phan chuong.",
				fmt.Errorf("no chapters matched"))
			return
		}
		emit(StageSplitting, 0, total, fmt.Sprintf("Phân tách hoàn thành: %d chương", total), nil)

		// ── 2. Suy nguoc Foundation (bo qua neu da day du) ──
		if needsFoundation(deps.Store, opts) {
			emit(StageFoundation, 0, total, "Dang suy nguoc Foundation (mot lan goi LLM)...", nil)
			fr, err := ReverseFoundation(ctx, deps.LLM, deps.Prompts.Foundation, chapters)
			if err != nil {
				emit(StageError, 0, total, "Suy nguoc Foundation that bai", err)
				return
			}
			scale := pickScale(total)
			if err := PersistFoundation(ctx, deps.Store, scale, fr); err != nil {
				emit(StageError, 0, total, "Luu Foundation xuong dia that bai", err)
				return
			}
			emit(StageFoundation, 0, total,
				fmt.Sprintf("Foundation san sang: %d nhan vat / %d quy tac / %d chuong de cuong (tap dau)",
					len(fr.Characters), len(fr.WorldRules), len(domain.FlattenOutline(fr.Volumes))),
				nil)
		} else {
			emit(StageFoundation, 0, total, "Foundation da ton tai, bo qua suy nguoc", nil)
		}

		// ── 3. Vong lap chuong ──
		premise, _ := deps.Store.Outline.LoadPremise()
		charactersBlock := loadCharactersBlock(deps.Store)

		startIdx := 0
		if opts.ResumeFrom > 1 {
			startIdx = opts.ResumeFrom - 1
		}
		for i := startIdx; i < total; i++ {
			if err := ctx.Err(); err != nil {
				emit(StageError, i+1, total, "Người dùng hủy", err)
				return
			}
			chNum := i + 1
			ch := chapters[i]

			// Đã hoàn thành → bỏ qua LLM
			if deps.Store.Progress.IsChapterCompleted(chNum) {
				emit(StageChapter, chNum, total, fmt.Sprintf("Chương %d đã hoàn thành, bỏ qua", chNum), nil)
				continue
			}

			emit(StageChapter, chNum, total, fmt.Sprintf("Đang phân tích chương %d/%d: %s", chNum, total, ch.Title), nil)

			activeHooks, _ := deps.Store.World.LoadActiveForeshadow()
			analysis, err := AnalyzeChapter(ctx, deps.LLM, deps.Prompts.Analyzer,
				chNum, ch.Title, ch.Content, premise, charactersBlock, activeHooks)
			if err != nil {
				emit(StageError, chNum, total, fmt.Sprintf("Phân tích chương %d thất bại", chNum), err)
				return
			}

			if err := PersistChapter(ctx, deps.Store, deps.CommitTool, chNum, ch.Title, ch.Content, analysis); err != nil {
				emit(StageError, chNum, total, fmt.Sprintf("Lưu chương %d thất bại", chNum), err)
				return
			}
			emit(StageChapter, chNum, total, fmt.Sprintf("Chương %d nhập hoàn thành", chNum), nil)
		}

		emit(StageDone, total, total, fmt.Sprintf("Nhập hoàn thành: %d chương", total), nil)
	}()

	return events, nil
}

// needsFoundation xác định xem có cần suy ngược lại foundation hay không.
// Nếu người dùng chỉ định rõ ResumeFrom > 1 thì coi là "tiếp tục nhập", bỏ qua suy ngược; ngược lại dựa vào trạng thái Store.
func needsFoundation(st *store.Store, opts Options) bool {
	if opts.ResumeFrom > 1 {
		return false
	}
	return len(st.FoundationMissing()) > 0
}

// pickScale chọn giá trị khởi đầu hợp lý cho mức quy hoạch dựa vào số chương; short ≤25, mid ≤80, ngược lại là long.
// Không ảnh hưởng đến bản thân quá trình import, chỉ ảnh hưởng đến việc Coordinator chọn prompt architect khi tiếp tục viết.
func pickScale(total int) domain.PlanningTier {
	switch {
	case total <= 25:
		return domain.PlanningTierShort
	case total <= 80:
		return domain.PlanningTierMid
	default:
		return domain.PlanningTierLong
	}
}

// loadCharactersBlock render hồ sơ nhân vật thành khối văn bản ngắn (name/role + một câu mô tả),
// chỉ để tham khảo trong context LLM, không cần cấu trúc chặt chẽ.
func loadCharactersBlock(st *store.Store) string {
	chars, err := st.Characters.Load()
	if err != nil || len(chars) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, c := range chars {
		fmt.Fprintf(&sb, "- **%s**（%s）：%s\n", c.Name, c.Role, oneLine(c.Description))
	}
	return sb.String()
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
