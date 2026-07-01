# Nhật ký thay đổi

Mọi thay đổi đáng chú ý của dự án được ghi ở đây. Định dạng theo tinh thần
[Keep a Changelog](https://keepachangelog.com/vi/1.0.0/).

Nhật ký này tập trung vào các thay đổi của **bản Việt hóa** so với upstream
`github.com/voocel/ainovel-cli`.

## [Chưa phát hành]

### Đã thêm
- **Tích hợp Claude Code** — dùng chính bộ model Claude (Opus 4.8/4.7, Sonnet 4.6,
  Haiku 4.5) để viết truyện:
  - Provider `claude-code` (type `anthropic`) với danh mục 4 model dựng sẵn.
  - Hai đường xác thực: cầu nối **Meridian** ở `http://127.0.0.1:3456` (thuê bao qua
    Agent SDK) hoặc **API key trực tiếp** `https://api.anthropic.com`.
  - **Preset "cân bằng"** (`bootstrap.BalancedClaudeRoles()`): Writer/Architect →
    `claude-opus-4-8` (high), Coordinator/Editor → `claude-sonnet-4-6` (medium).
  - Áp preset qua wizard, lệnh TUI **`/model auto`**, hoặc nút Web
    **"Tự chọn (Claude cân bằng)"** (`POST /api/model/auto`).
  - Tài liệu: [docs/claude-code.md](docs/claude-code.md).
- **Tài liệu dự án**: `CLAUDE.md` (định hướng cho AI agent/contributor),
  `docs/claude-code.md`, và `CHANGELOG.md` (tệp này).

### Đã sửa
- **`draft_chapter`** — tham số `mode` chuyển từ **bắt buộc** sang **tùy chọn** (bỏ
  `StrictSchema`). Trước đây `mode` bị đánh `required` để tương thích strict tool calling
  của OpenAI, nhưng Gemini không hỗ trợ strict và thỉnh thoảng gọi tool không kèm `mode`
  → agentcore từ chối với "required parameter `mode` is missing" trước cả khi `Execute`
  chạy, đốt lượt của Writer. `Execute` vốn coi `mode` rỗng là `write`, nên để tùy chọn
  vừa khớp hành vi vừa hết lỗi. Xem [internal/tools/draft_chapter.go](internal/tools/draft_chapter.go).

## Việt hóa (đã phát hành trên nhánh `main`)

Việt hóa toàn bộ engine, tài liệu và tài sản, chia thành các giai đoạn:

- **GĐ 1a** — Việt hóa lõi engine (prompt gắn liền với hằng số Go). (`9acfb0d`)
- **GĐ 1b** — Việt hóa toàn bộ `assets/references` và `assets/styles`. (`2f51094`)
- **GĐ 2–4** — Việt hóa chuỗi runtime, test fixtures, config, docs, scripts, evals. (`7f2f3bc`)
- **GĐ 5** — Rà soát cuối: dịch `host/`, `tools/`, docs và mọi tệp còn sót. (`e74545a`)

## Giao diện Web

- Thêm **cổng vào thứ ba** `internal/entry/web` — bàn làm việc trong trình duyệt
  (`ainovel-cli --web`), chỉ localhost `127.0.0.1`, giao diện tiếng Việt, đẩy sự kiện qua
  SSE, tính năng tương đương TUI (start/steer/pause/continue, export, đổi model & cường độ
  suy luận, cùng tạo quy hoạch, import, simulate, `/diag`). Bên dưới tái sử dụng cùng engine
  `host.Host`, không thay đổi logic sáng tác. Hướng dẫn: [docs/huong-dan-su-dung.md](docs/huong-dan-su-dung.md).
