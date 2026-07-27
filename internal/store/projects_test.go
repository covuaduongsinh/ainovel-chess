package store

import (
	"os"
	"path/filepath"
	"testing"
)

// mkProject dựng một thư mục dự án tối thiểu (có progress.json) bên trong root.
func mkProject(t *testing.T, root, slug, novelName string) string {
	t.Helper()
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
	if err := NewStore(dir).Progress.Init(novelName, 10); err != nil {
		t.Fatalf("Progress.Init %s: %v", dir, err)
	}
	return dir
}

// mkPremiseOnly dựng dự án chưa bắt đầu: chỉ có premise.md, không có meta/progress.json.
func mkPremiseOnly(t *testing.T, root, slug string) string {
	t.Helper()
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "premise.md"), []byte("# Truyện thử\n"), 0o644); err != nil {
		t.Fatalf("WriteFile premise.md: %v", err)
	}
	return dir
}

func TestListSkipsArchiveDir(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "sach-mot", "Sách Một")
	mkProject(t, filepath.Join(root, ArchiveDirName), "sach-cu", "Sách Cũ")

	list, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Slug != "sach-mot" {
		t.Fatalf("List phải chỉ trả về dự án ngoài kho, got %+v", list)
	}

	archived, err := ListArchived(root)
	if err != nil {
		t.Fatalf("ListArchived: %v", err)
	}
	if len(archived) != 1 || archived[0].Name != "Sách Cũ" {
		t.Fatalf("ListArchived sai, got %+v", archived)
	}
}

func TestListArchivedMissingDir(t *testing.T) {
	archived, err := ListArchived(t.TempDir())
	if err != nil {
		t.Fatalf("ListArchived khi chưa có kho phải không lỗi: %v", err)
	}
	if len(archived) != 0 {
		t.Fatalf("mong đợi danh sách rỗng, got %+v", archived)
	}
}

func TestRenameUpdatesNameAndDir(t *testing.T) {
	root := t.TempDir()
	dir := mkProject(t, root, "ten-cu", "Tên Cũ")

	newDir, err := Rename(dir, "Kiếm Khách Phương Nam")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if filepath.Base(newDir) != "Kiếm-Khách-Phương-Nam" {
		t.Fatalf("thư mục chưa đổi đúng, got %q", filepath.Base(newDir))
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("thư mục cũ phải biến mất, err=%v", err)
	}
	p, err := NewStore(newDir).Progress.Load()
	if err != nil || p == nil {
		t.Fatalf("Progress.Load sau khi đổi tên: p=%v err=%v", p, err)
	}
	if p.NovelName != "Kiếm Khách Phương Nam" {
		t.Fatalf("novelName chưa cập nhật, got %q", p.NovelName)
	}
}

func TestRenameKeepsDirWhenSlugUnchanged(t *testing.T) {
	root := t.TempDir()
	dir := mkProject(t, root, "Ten-Sach", "Tên gì đó")

	newDir, err := Rename(dir, "Ten Sach")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if newDir != dir {
		t.Fatalf("slug không đổi thì phải giữ nguyên thư mục, got %q muốn %q", newDir, dir)
	}
}

func TestRenamePremiseOnlyDoesNotCreateProgress(t *testing.T) {
	root := t.TempDir()
	dir := mkPremiseOnly(t, root, "chua-bat-dau")

	newDir, err := Rename(dir, "Đã Đặt Tên")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newDir, "meta", "progress.json")); !os.IsNotExist(err) {
		t.Fatalf("không được tạo progress.json cho dự án chưa bắt đầu, err=%v", err)
	}
	if !List1Has(t, root, "Đã-Đặt-Tên") {
		t.Fatalf("dự án sau khi đổi tên không thấy trong danh sách")
	}
}

// List1Has kiểm tra root có đúng một dự án với slug cho trước.
func List1Has(t *testing.T, root, slug string) bool {
	t.Helper()
	list, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return len(list) == 1 && list[0].Slug == slug
}

func TestRenameRejectsEmptyName(t *testing.T) {
	root := t.TempDir()
	dir := mkProject(t, root, "sach", "Sách")
	if _, err := Rename(dir, "   "); err == nil {
		t.Fatal("Rename với tên rỗng phải lỗi")
	}
}

func TestArchiveAndUnarchiveRoundTrip(t *testing.T) {
	root := t.TempDir()
	dir := mkProject(t, root, "sach-cat-di", "Sách Cất Đi")

	archived, err := Archive(root, dir)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if filepath.Dir(archived) != filepath.Join(root, ArchiveDirName) {
		t.Fatalf("đích lưu trữ sai: %q", archived)
	}
	if list, _ := List(root); len(list) != 0 {
		t.Fatalf("dự án đã lưu trữ vẫn hiện ở danh sách chính: %+v", list)
	}

	back, err := Unarchive(root, archived)
	if err != nil {
		t.Fatalf("Unarchive: %v", err)
	}
	if back != dir {
		t.Fatalf("bỏ lưu trữ phải về đúng chỗ cũ, got %q muốn %q", back, dir)
	}
	if archived, _ := ListArchived(root); len(archived) != 0 {
		t.Fatalf("kho phải rỗng sau khi bỏ lưu trữ: %+v", archived)
	}
}

func TestArchiveAvoidsNameCollision(t *testing.T) {
	root := t.TempDir()
	mkProject(t, filepath.Join(root, ArchiveDirName), "trung-ten", "Bản cũ trong kho")
	dir := mkProject(t, root, "trung-ten", "Bản mới")

	archived, err := Archive(root, dir)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if filepath.Base(archived) != "trung-ten-2" {
		t.Fatalf("phải thêm hậu tố tránh trùng, got %q", filepath.Base(archived))
	}
	if list, _ := ListArchived(root); len(list) != 2 {
		t.Fatalf("kho phải có 2 dự án, got %d", len(list))
	}
}

func TestArchiveRejectsAlreadyArchived(t *testing.T) {
	root := t.TempDir()
	dir := mkProject(t, filepath.Join(root, ArchiveDirName), "trong-kho", "Trong kho")
	if _, err := Archive(root, dir); err == nil {
		t.Fatal("lưu trữ lại dự án đã trong kho phải lỗi")
	}
}

func TestUnarchiveRejectsProjectOutsideArchive(t *testing.T) {
	root := t.TempDir()
	dir := mkProject(t, root, "ngoai-kho", "Ngoài kho")
	if _, err := Unarchive(root, dir); err == nil {
		t.Fatal("bỏ lưu trữ dự án không nằm trong kho phải lỗi")
	}
}

func TestDeleteRemovesProject(t *testing.T) {
	root := t.TempDir()
	dir := mkProject(t, root, "sach-xoa", "Sách Xóa")

	if err := Delete(dir); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("thư mục phải bị xóa, err=%v", err)
	}
}

// Hàng rào quan trọng nhất: thao tác phá hủy không được chạm vào thư mục không phải dự án
// (đặc biệt là chính kho _archive — xóa nhầm sẽ mất toàn bộ sách đã cất).
func TestDestructiveOpsRejectNonProjectDir(t *testing.T) {
	root := t.TempDir()
	mkProject(t, filepath.Join(root, ArchiveDirName), "sach-trong-kho", "Sách trong kho")
	archiveRoot := filepath.Join(root, ArchiveDirName)

	plain := filepath.Join(root, "thu-muc-la")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	for _, dir := range []string{archiveRoot, plain} {
		if err := Delete(dir); err == nil {
			t.Fatalf("Delete(%q) phải bị từ chối", dir)
		}
		if _, err := Archive(root, dir); err == nil {
			t.Fatalf("Archive(%q) phải bị từ chối", dir)
		}
		if _, err := Rename(dir, "Tên mới"); err == nil {
			t.Fatalf("Rename(%q) phải bị từ chối", dir)
		}
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("thư mục %q phải còn nguyên: %v", dir, err)
		}
	}
	if list, _ := ListArchived(root); len(list) != 1 {
		t.Fatalf("kho lưu trữ phải còn nguyên vẹn, got %+v", list)
	}
}
