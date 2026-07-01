package exp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// Run thuc hien mot lan xuat. Tra ve dong bo, IO nho (doc ghi file cuc bo).
//
// Ngu nghia that bai:
//   - deps/opts khong hop le -> loi cau hinh tra ve ngay
//   - Khong co chuong da hoan thanh nao -> tra ve loi (de caller ro rang)
//   - Chuong nao do trong pham vi thieu chapters/{ch}.md -> tra ve loi (progress va he thong file khong nhat quan la bug tang su that, nen de nguoi dung thay)
//   - Duong dan dau ra da ton tai va khong chi dinh Overwrite -> tra ve loi
//
// Skipped dung cho truong hop "hop le trong pham vi nhung chua hoan thanh" (nguoi dung truyen to=100 nhung moi viet den 80).
func Run(ctx context.Context, deps Deps, opts Options) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if deps.Store == nil {
		return nil, fmt.Errorf("exp: deps.Store is nil")
	}

	if opts.Format == "" {
		f, err := inferFormat(opts.OutPath)
		if err != nil {
			return nil, err
		}
		opts.Format = f
	}
	if opts.Format != FormatTXT && opts.Format != FormatEPUB {
		return nil, fmt.Errorf("exp: dinh dang chua duoc ho tro %q", opts.Format)
	}

	progress, err := deps.Store.Progress.Load()
	if err != nil {
		return nil, fmt.Errorf("tai progress that bai: %w", err)
	}
	if progress == nil || len(progress.CompletedChapters) == 0 {
		return nil, fmt.Errorf("chua co chuong hoan thanh nao, khong co noi dung de xuat")
	}

	completed := make(map[int]struct{}, len(progress.CompletedChapters))
	maxCh := 0
	for _, c := range progress.CompletedChapters {
		completed[c] = struct{}{}
		if c > maxCh {
			maxCh = c
		}
	}

	from := opts.From
	if from <= 0 {
		from = 1
	}
	to := opts.To
	if to <= 0 {
		to = maxCh
	}
	if from > to {
		return nil, fmt.Errorf("pham vi chuong khong hop le: from=%d > to=%d", from, to)
	}

	var chapters, skipped []int
	for ch := from; ch <= to; ch++ {
		if _, ok := completed[ch]; ok {
			chapters = append(chapters, ch)
		} else {
			skipped = append(skipped, ch)
		}
	}
	if len(chapters) == 0 {
		return nil, fmt.Errorf("khong co chuong hoan thanh trong pham vi %d..%d", from, to)
	}

	bodies := make(map[int]string, len(chapters))
	for _, ch := range chapters {
		text, err := deps.Store.Drafts.LoadChapterText(ch)
		if err != nil {
			return nil, fmt.Errorf("doc chuong %d that bai: %w", ch, err)
		}
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("progress danh dau chuong %d da hoan thanh, nhung chapters/%02d.md thieu hoac rong", ch, ch)
		}
		bodies[ch] = text
	}

	outline, _ := deps.Store.Outline.LoadOutline()
	var volumes []domain.VolumeOutline
	if progress.Layered {
		volumes, _ = deps.Store.Outline.LoadLayeredOutline()
	}

	outPath := opts.OutPath
	if outPath == "" {
		name := strings.TrimSpace(progress.NovelName)
		if name == "" {
			name = filepath.Base(deps.Store.Dir())
		}
		outPath = filepath.Join(deps.Store.Dir(), sanitizeFileName(name)+"."+string(opts.Format))
	}

	if !opts.Overwrite {
		if _, err := os.Stat(outPath); err == nil {
			return nil, fmt.Errorf("file da ton tai: %s (them --overwrite de ghi de)", outPath)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("kiem tra duong dan dau ra that bai: %w", err)
		}
	}

	titleIdx := buildTitleIndex(outline)
	var locations map[int]chapterLocation
	if len(volumes) > 0 {
		locations = buildLocations(volumes)
	}

	var data []byte
	switch opts.Format {
	case FormatTXT:
		data = []byte(renderTXT(progress.NovelName, chapters, titleIdx, locations, bodies))
	case FormatEPUB:
		buf, err := renderEPUB(progress.NovelName, chapters, titleIdx, locations, bodies)
		if err != nil {
			return nil, fmt.Errorf("hien thi EPUB that bai: %w", err)
		}
		data = buf
	}

	if err := atomicWrite(outPath, data); err != nil {
		return nil, fmt.Errorf("ghi that bai: %w", err)
	}

	return &Result{
		Path:     outPath,
		Chapters: len(chapters),
		Bytes:    len(data),
		Skipped:  skipped,
	}, nil
}

// inferFormat suy ra dinh dang tu hau to duong dan dau ra. Duong dan rong tro ve TXT; hau to khong ro bao loi (tranh loi im lang).
func inferFormat(path string) (Format, error) {
	if path == "" {
		return FormatTXT, nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case "", ".txt":
		return FormatTXT, nil
	case ".epub":
		return FormatEPUB, nil
	default:
		return "", fmt.Errorf("khong the suy ra dinh dang tu phan mo rong %q (ho tro .txt / .epub)", filepath.Ext(path))
	}
}

// atomicWrite co cau truc giong WriteFile cua store/io.go: tmp + sync + rename.
// Khong tai su dung store.IO vi duong dan dau ra co the nam ngoai store.Dir().
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// sanitizeFileName thay the cac ky tu trong ten file khong duoc phep hoac de gay nham lan tren phan lon he thong file.
// Khong bien doi manh, chi chan phan cach duong dan va ky tu dieu khien.
func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "novel"
	}
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"\x00", "_",
	)
	return replacer.Replace(name)
}
