// Package store cung cấp lưu trữ bền vững dựa trên hệ thống tệp.
//
// Kiến trúc: 1 IO nền + nhiều lưu trữ con + 1 gốc tổng hợp.
// Mỗi lưu trữ con giữ một instance IO độc lập và một sync.RWMutex riêng.
// Đọc/ghi của các miền chính (Progress, Outline, Drafts, Summaries, v.v.) không chặn nhau;
// WorldStore gộp nhiều miền nhỏ ít dùng vào chung một khóa.
//
// Gốc tổng hợp Store giữ tham chiếu đến tất cả lưu trữ con, và chịu trách nhiệm
// các thao tác nguyên tử liên miền (ExpandArc, AppendVolume, ClearHandledSteer).
//
// Phân chia lưu trữ con:
//   - ProgressStore: trạng thái tiến độ chính (meta/progress.json)
//   - OutlineStore: tiền đề, dàn ý (phẳng/phân tầng), la bàn
//   - DraftStore: cấu tứ chương, bản nháp, bản cuối
//   - SummaryStore: tóm tắt chương/cung/tập
//   - RunMetaStore: siêu dữ liệu chạy (mô hình, lịch sử can thiệp)
//   - SignalStore: tệp tín hiệu một lần (khôi phục PendingCommit)
//   - CheckpointStore: checkpoint cấp step (meta/checkpoints.jsonl)
//   - RuntimeStore: hàng đợi sự kiện thời gian chạy (meta/runtime/*.jsonl)
//   - CharacterStore: hồ sơ nhân vật, ảnh chụp trạng thái
//   - WorldStore: dòng thời gian, phục bút, quan hệ, thay đổi trạng thái, quy tắc thế giới, quy tắc phong cách, thẩm định
package store
