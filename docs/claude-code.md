# Dùng Claude Code với ainovel-cli

Tài liệu này hướng dẫn chi tiết cách dùng chính bộ mô hình Claude trong Claude Code
(Opus 4.8, Opus 4.7, Sonnet 4.6, Haiku 4.5) để viết truyện mới hoặc viết tiếp bộ truyện
hiện có — kèm cách xác thực, thiết lập, preset "cân bằng" và xử lý sự cố.

> Bản tóm tắt ngắn nằm trong [README.md](../README.md) (mục "Dùng gói thuê bao Claude Code").
> Tài liệu này là bản đầy đủ.

## 1. Vì sao phải qua một cầu nối / proxy

Engine bên dưới dùng [litellm](https://github.com/voocel/litellm) để nói chuyện với LLM.
Với giao thức Anthropic, litellm **chỉ xác thực bằng header `x-api-key`** — nó **không**
biết cách đăng nhập gói thuê bao Claude Code trực tiếp trong binary. Do đó có **hai đường
hợp lệ**, và bạn chọn một:

| Đường | Base URL | `api_key` | Chi phí | Hợp với |
|---|---|---|---|---|
| **Thuê bao (qua Agent SDK)** | `http://127.0.0.1:3456` (cầu nối nội bộ) | không cần key thật (giữ chỗ `sk-local`) | rút từ hạn mức Agent SDK của gói, có trần tháng | thử nghiệm, truyện ngắn/vừa |
| **API key trực tiếp** | `https://api.anthropic.com` | `sk-ant-...` (thật) | trả theo token, không trần | truyện dài chạy liên tục |

> ⚠️ **Không** dùng proxy phát lại (replay) token OAuth thẳng lên `api.anthropic.com`.
> Cách đó vi phạm điều khoản và đã bị chặn từ 04/2026. Đường thuê bao hợp lệ là đi qua
> **Claude Agent SDK chính thức** (ví dụ Meridian bên dưới).

Các hằng số/hàm ở [internal/bootstrap/claude.go](../internal/bootstrap/claude.go) là
**nguồn sự thật duy nhất** cho mọi giá trị nêu trong tài liệu này (base URL, danh mục
model, preset). Nếu code đổi, tài liệu này phải đổi theo.

## 2. Đường thuê bao: cầu nối Meridian

[Meridian](https://github.com/rynfar/meridian) là một cầu nối chạy trên máy bạn, đi qua
Claude Agent SDK và phơi ra một endpoint nói **giao thức Anthropic Messages** ở cổng 3456:

```bash
npm i -g @rynfar/meridian
claude login          # đăng nhập gói Claude Code của bạn (Pro / Max)
meridian              # mở http://127.0.0.1:3456
```

Cứ để Meridian chạy nền, rồi thiết lập ainovel-cli trỏ tới `http://127.0.0.1:3456`.
Nếu bạn dùng một cầu nối/proxy Agent SDK khác, chỉ cần đổi lại cổng cho khớp.

## 3. Thiết lập trong ainovel-cli

### Qua wizard (khuyến nghị)

Chạy `ainovel-cli` (TUI) hoặc `ainovel-cli --web` (trình duyệt), rồi:

1. Ở bước chọn Provider, chọn **"Claude Code"**.
2. Base URL: để mặc định `http://127.0.0.1:3456` (đường Meridian) — hoặc đổi thành
   `https://api.anthropic.com` và điền `api_key = sk-ant-...` (đường trả-theo-token).
3. Đồng ý **bật preset tự-chọn cân bằng** khi được hỏi.

Wizard sẽ dựng sẵn provider `claude-code` (type `anthropic`) kèm danh mục 4 model, và nếu
bạn bật preset thì gán luôn model theo vai (xem bảng ở mục 4).

### Sửa tay tệp cấu hình

Tương đương với việc chỉnh trực tiếp `~/.ainovel/config.json` (đồng bộ với
[config.example.jsonc](../config.example.jsonc)):

```jsonc
{
  "provider": "claude-code",
  "model": "claude-sonnet-4-6",
  "reasoning_effort": "medium",
  "providers": {
    "claude-code": {
      "type": "anthropic",
      "base_url": "http://127.0.0.1:3456",
      "models": ["claude-opus-4-8", "claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5"]
    }
  },
  "roles": {
    "writer":      { "provider": "claude-code", "model": "claude-opus-4-8",   "reasoning_effort": "high" },
    "architect":   { "provider": "claude-code", "model": "claude-opus-4-8",   "reasoning_effort": "high" },
    "editor":      { "provider": "claude-code", "model": "claude-sonnet-4-6", "reasoning_effort": "medium" },
    "coordinator": { "provider": "claude-code", "model": "claude-sonnet-4-6", "reasoning_effort": "medium" }
  }
}
```

> Với đường trả-theo-token, đổi `base_url` thành `https://api.anthropic.com` và thêm
> `"api_key": "sk-ant-..."` trong khối `providers.claude-code`. Với đường Meridian, có thể
> bỏ trống `api_key` — hệ thống tự điền giữ chỗ `sk-local` vì litellm đòi `x-api-key` khác rỗng.

## 4. Preset "cân bằng" (tự động chọn model)

Preset ưu tiên **chất lượng** ở nơi quan trọng (viết prose, dựng thế giới) và **tiết kiệm**
cho điều phối/xét duyệt. Nguồn sự thật: `bootstrap.BalancedClaudeRoles()`.

| Vai trò | Model | Cường độ suy luận |
|---|---|---|
| Writer | `claude-opus-4-8` | high |
| Architect | `claude-opus-4-8` | high |
| Editor | `claude-sonnet-4-6` | medium |
| Coordinator | `claude-sonnet-4-6` | medium |

Có ba cách áp preset này:

- **Wizard**: đồng ý "bật preset tự-chọn cân bằng" khi thiết lập lần đầu.
- **TUI**: gõ `/model auto` — áp preset cho cả 4 vai, in sự kiện SYSTEM xác nhận
  (xem `applyModelAutoPreset` trong
  [internal/entry/tui/command_model.go](../internal/entry/tui/command_model.go)).
- **Web**: bấm nút **"Tự chọn (Claude cân bằng)"** trong bảng Mô hình — gọi
  `POST /api/model/auto` (xem `handleModelAuto` trong
  [internal/entry/web/api_model.go](../internal/entry/web/api_model.go)).

Mọi thay đổi được ghi vào `~/.ainovel/config.json`. Muốn chỉnh khác đi thì đổi tay từng vai:

- **TUI**: gõ `/model`, chọn Vai trò → Provider → Model → Cường độ suy luận.
- **Web**: mở bảng Mô hình và chọn theo từng vai.

Ví dụ: đưa Editor lên Opus cho khắt khe hơn, hoặc chọn `claude-haiku-4-5` cho rẻ nhất.

## 5. Viết tiếp bộ truyện hiện có

Mỗi tiểu thuyết bind vào thư mục khởi động. Để viết tiếp bằng model Claude đã chọn:

```bash
cd /duong/dan/truyen-cu
ainovel-cli            # (hoặc --web) — engine tự khôi phục từ checkpoint và viết tiếp
```

Không cần thao tác gì thêm: cấu hình model nằm ở `~/.ainovel/config.json` (toàn cục), còn
tiến độ/checkpoint nằm trong `output/novel/meta/` của thư mục truyện.

## 6. Xử lý sự cố

| Triệu chứng | Nguyên nhân thường gặp | Cách xử lý |
|---|---|---|
| `connection refused` tới `127.0.0.1:3456` | Meridian chưa chạy | Mở terminal khác chạy `meridian`; kiểm tra `claude login` đã xong |
| `/model auto` hoặc nút web báo lỗi / trả **400** | Provider `claude-code` chưa được cấu hình | Chạy lại setup và chọn "Claude Code", hoặc thêm khối `providers.claude-code` vào config |
| `401 Unauthorized` khi dùng `api.anthropic.com` | `api_key` sai/thiếu (giữ chỗ `sk-local` chỉ hợp lệ cho cầu nối nội bộ) | Điền `api_key = sk-ant-...` thật |
| Hết hạn mức giữa chừng (đường thuê bao) | Chạm trần Agent SDK của gói | Đổi sang đường API key trực tiếp, hoặc đợi chu kỳ reset |
| Muốn giảm chi phí | Opus đắt | Đổi vai không quá quan trọng sang `claude-sonnet-4-6` hoặc `claude-haiku-4-5` trong `/model` |

## 7. Proxy Anthropic tùy chỉnh (nâng cao)

Nếu proxy của bạn dùng giao thức Anthropic và yêu cầu header nhận dạng của client Claude
Code, đặt `type: "anthropic"`, để `anthropic_beta` ở tầng trên của `extra`, và các HTTP
header (Stainless…) trong `extra.headers`. Xem ví dụ đầy đủ ở mục "Tùy chỉnh proxy
Anthropic (nâng cao)" trong [README.md](../README.md).

## Tham chiếu

- [internal/bootstrap/claude.go](../internal/bootstrap/claude.go) — hằng số & preset (nguồn sự thật)
- [internal/entry/tui/command_model.go](../internal/entry/tui/command_model.go) — lệnh `/model auto`
- [internal/entry/web/api_model.go](../internal/entry/web/api_model.go) — `POST /api/model/auto`
- [config.example.jsonc](../config.example.jsonc) — mẫu cấu hình đầy đủ
- [README.md](../README.md) — tổng quan & các provider khác
