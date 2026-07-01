# CLAUDE.md

Hướng dẫn định hướng cho AI agent / người đóng góp khi làm việc trong repo này.
Ngắn gọn có chủ đích — chi tiết nằm ở [README.md](README.md) và [docs/](docs/).

## Dự án là gì

`ainovel-cli` (module `github.com/voocel/ainovel-cli`, thư mục local `ainovel-chess`) là
engine sáng tác **tiểu thuyết dài hoàn toàn tự động**, do LLM điều khiển. Trong một lần
Prompt, một **Coordinator** điều phối ba sub-agent **Architect / Writer / Editor** để hoàn
thành cả cuốn sách. Lớp **Host** chỉ làm vỏ mỏng: khởi động, khôi phục, quan sát sự kiện —
không đưa ra quyết định lịch trình.

Đây là một bản **Việt hóa**: toàn bộ chuỗi runtime, comment và tài liệu đều bằng **tiếng
Việt**. Xem lịch sử ở [CHANGELOG.md](CHANGELOG.md).

## Ba entry point & cách chạy

Một binary, ba cổng vào (phân nhánh trong [cmd/ainovel-cli/main.go](cmd/ainovel-cli/main.go),
mã lớp giao diện ở [internal/entry/](internal/entry/)):

```bash
ainovel-cli                                  # TUI (mặc định) — internal/entry/tui
ainovel-cli --web [--port N]                 # Web UI (localhost:8765) — internal/entry/web
ainovel-cli --headless --prompt "..."        # headless/batch — internal/entry/headless
ainovel-cli --headless --prompt-file f.txt   # prompt từ tệp (chỉ với --headless)
ainovel-cli --version | update [ver] | eval  # phụ trợ
```

Lưu ý: `--prompt` / `--prompt-file` **chỉ** dùng được với `--headless`; `--port` chỉ với
`--web`. Chế độ Web có wizard lần đầu ngay trong trình duyệt, không qua setup terminal.

## Build & test

Không có `Makefile` — dùng lệnh Go tiêu chuẩn (Go 1.25.5):

```bash
go build ./cmd/ainovel-cli      # build binary
go build ./...                  # biên dịch toàn bộ (kiểm tra nhanh không vỡ)
go test ./...                   # chạy unit test
go test ./internal/bootstrap/   # test một package
```

> Trên Windows box hiện tại có **4 test lỗi do môi trường** (không phải lỗi của thay đổi
> bạn tạo) — xem ghi chú bộ nhớ nội bộ. Đừng cố "sửa" chúng.

## Bản đồ package (internal/)

| Package | Vai trò |
|---|---|
| `entry/{tui,web,headless}` | Ba cổng vào — đều là consumer mỏng của `host.Host`, không chứa logic sáng tác |
| `host/` | Engine điều phối: chạy Coordinator, khôi phục, chiếu sự kiện, `reminder/` sinh `<system-reminder>` mỗi lượt |
| `agents/` | Dựng agent + đóng gói/nén ngữ cảnh cho tiểu thuyết dài |
| `tools/` | Công cụ nguyên tử cho Architect/Writer/Editor (IO + ghi checkpoint), chỉ trả JSON sự kiện |
| `store/` | Lưu bền vững: progress, checkpoints, drafts, outline, summaries, characters, world, signals |
| `domain/` | Model dữ liệu: Phase, Flow, Progress, Checkpoint, sự kiện, quy tắc chuyển trạng thái |
| `bootstrap/` | Nạp config + setup wizard + tích hợp provider (gồm `claude.go` cho Claude Code) |
| `diag/`, `eval/`, `rules/`, `userrules/`, `models/`, `stylestat/`, `notify/` | Chẩn đoán, đánh giá, quy tắc phong cách, registry model, thống kê văn phong, thông báo |

Tài sản nhúng (prompt, style, reference) ở [assets/](assets/), nạp qua `embed.FS`.

## Quy ước quan trọng (đọc trước khi sửa)

- **Ngôn ngữ**: giữ **tiếng Việt** cho mọi chuỗi runtime, comment, tài liệu mới. Đừng chèn
  lại tiếng Anh/Trung vào chuỗi hiển thị cho người dùng.
- **Iron rules kiến trúc** (xem [docs/architecture.md](docs/architecture.md)):
  - Tool **chỉ trả sự kiện**, không kèm chuỗi lệnh; lệnh được `host/reminder/` tính lại mỗi lượt.
  - Coordinator không thể `end_turn` khi `Phase ≠ Complete` (StopGuard canh vật lý).
  - Thêm năng lực cho Web/TUI = **thêm handler gọi phương thức `host.Host`**, KHÔNG nhét
    logic vào package `host`. Web và TUI phải mirror nhau (theo tinh thần khi thêm Web UI).
- **Config**: `~/.ainovel/config.json` (toàn cục) → `./.ainovel/config.json` (dự án) →
  `--config PATH`. Giá trị `provider` là **tên key** trỏ vào `providers`, KHÔNG phải tên
  giao thức. Mẫu đầy đủ: [config.example.jsonc](config.example.jsonc).
- **Ghi tệp** dùng temp + fsync + rename (nguyên tử) — giữ nguyên khi đụng tới I/O của store.

## Tài liệu sâu hơn

- [docs/architecture.md](docs/architecture.md) — kiến trúc runtime, Phase/Flow, iron rules
- [docs/context-management.md](docs/context-management.md) — nén ngữ cảnh, tóm tắt phân tầng
- [docs/evaluation-system.md](docs/evaluation-system.md) — đánh giá chất lượng 7 chiều
- [docs/observability.md](docs/observability.md) — chẩn đoán & quan sát (`/diag`)
- [docs/refactor-flow-driven.md](docs/refactor-flow-driven.md) — thiết kế Flow Router
- [docs/user-rules-runtime.md](docs/user-rules-runtime.md) — quy tắc người dùng runtime
- [docs/huong-dan-su-dung.md](docs/huong-dan-su-dung.md) — hướng dẫn cho người mới (giao diện Web)
- [docs/claude-code.md](docs/claude-code.md) — dùng model Claude qua Claude Code
- [CHANGELOG.md](CHANGELOG.md) — nhật ký thay đổi
