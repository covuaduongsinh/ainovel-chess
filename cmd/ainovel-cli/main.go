package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/entry/headless"
	"github.com/voocel/ainovel-cli/internal/entry/tui"
	"github.com/voocel/ainovel-cli/internal/entry/web"
	"github.com/voocel/ainovel-cli/internal/eval"
	"github.com/voocel/ainovel-cli/internal/rules"
	buildversion "github.com/voocel/ainovel-cli/internal/version"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// headlessMode ghi lại lần này có phải khởi động headless không, để die quyết định khi thoát lỗi có cần tạm dừng hay không.
var headlessMode bool

func main() {
	// Lệnh con chặn trước khi phân tích flag thông thường: eval là harness đánh giá offline, hệ tham số độc lập.
	if len(os.Args) > 1 && os.Args[1] == "eval" {
		os.Exit(eval.Command(os.Args[2:]))
	}

	opts, args, err := parseCLIOptions(os.Args[1:])
	if err != nil {
		die("flags: %v", err)
	}
	if opts.Version {
		buildversion.Print(os.Stdout, versionInfo())
		return
	}
	if opts.Update {
		if err := runSelfUpdate(opts.UpdateVersion); err != nil {
			fmt.Fprintf(os.Stderr, "update: %v\n", err)
			os.Exit(1)
		}
		return
	}
	headlessMode = opts.Headless

	// Chế độ Web tự có hướng dẫn lần đầu trong trình duyệt, không đi qua setup wizard trên terminal.
	if opts.Web {
		if err := web.RunWeb(opts.ConfigPath, versionInfo().Version, web.Options{Port: opts.Port, Open: true}); err != nil {
			die("lỗi: %v", err)
		}
		return
	}

	// Hướng dẫn lần đầu
	if bootstrap.NeedsSetup(opts.ConfigPath) {
		if opts.Headless {
			die("lỗi: chế độ headless không hỗ trợ hướng dẫn lần đầu, hãy chạy TUI một lần để hoàn tất cấu hình")
		}
		setupCfg, err := bootstrap.RunSetup()
		if err != nil {
			die("setup: %v", err)
		}
		// Sau khi hướng dẫn xong, tiếp tục dùng cấu hình đã tạo
		runWithConfig(setupCfg, opts, args)
		return
	}

	// Tải cấu hình
	cfg, err := bootstrap.LoadConfig(opts.ConfigPath)
	if err != nil {
		die("config: %v", err)
	}

	runWithConfig(cfg, opts, args)
}

// die xử lý thống nhất lỗi nghiêm trọng khi thoát: in ra stderr, ghi vào ~/.ainovel/last-error.log,
// và ở terminal tương tác (không phải headless) thì tạm dừng chờ Enter — khi khởi động bằng double-click
// console sẽ đóng ngay khi tiến trình thoát, không tạm dừng thì lỗi chớp qua, đây chính là nguyên nhân
// người dùng không thể tìm hiểu nguyên nhân như mô tả trong issue #37.
func die(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, msg)
	if path := bootstrap.WriteStartupError(msg); path != "" {
		fmt.Fprintf(os.Stderr, "(chi tiết lỗi đã ghi vào %s)\n", path)
	}
	if !headlessMode && stdinIsTerminal() {
		fmt.Fprint(os.Stderr, "\nNhấn Enter để thoát...")
		fmt.Fscanln(os.Stdin)
	}
	os.Exit(1)
}

// stdinIsTerminal kiểm tra stdin có kết nối với terminal (thiết bị ký tự) không.
// Double-click / terminal tương tác → true; pipe, redirect, CI → false.
// Xấp xỉ không phụ thuộc, đủ để phân biệt có cần tạm dừng hay không.
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func runWithConfig(cfg bootstrap.Config, opts cliOptions, args []string) {
	rules.EnsureHomeRulesDir()

	if len(args) > 0 {
		die("lỗi: không còn hỗ trợ truyền yêu cầu tiểu thuyết trực tiếp qua command line, hãy khởi động rồi nhập vào ô nhập liệu TUI")
	}

	bundle := assets.Load(cfg.Style)
	if opts.Headless {
		prompt, err := loadPrompt(opts)
		if err != nil {
			die("lỗi: %v", err)
		}
		if err := headless.Run(cfg, bundle, headless.Options{Prompt: prompt}); err != nil {
			die("lỗi: %v", err)
		}
		return
	}
	if opts.Prompt != "" || opts.PromptFile != "" {
		die("lỗi: --prompt/--prompt-file chỉ dùng được trong chế độ --headless")
	}
	if err := tui.Run(cfg, bundle, versionInfo().Version); err != nil {
		die("lỗi: %v", err)
	}
}

type cliOptions struct {
	ConfigPath    string
	Headless      bool
	Web           bool
	Port          int
	Prompt        string
	PromptFile    string
	Version       bool
	Update        bool
	UpdateVersion string
}

// parseCLIOptions trích xuất CLI flag, trả về tùy chọn và tham số còn lại.
func parseCLIOptions(argv []string) (cliOptions, []string, error) {
	var opts cliOptions
	var args []string
	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case "--version", "-v":
			opts.Version = true
		case "version":
			if i+1 < len(argv) {
				return opts, nil, fmt.Errorf("version không nhận tham số")
			}
			opts.Version = true
		case "update":
			if opts.Update {
				return opts, nil, fmt.Errorf("update chỉ được chỉ định một lần")
			}
			opts.Update = true
			if i+1 < len(argv) {
				if strings.HasPrefix(argv[i+1], "-") {
					return opts, nil, fmt.Errorf("update chỉ nhận một tham số phiên bản tùy chọn")
				}
				opts.UpdateVersion = argv[i+1]
				i++
			}
			if i+1 < len(argv) {
				return opts, nil, fmt.Errorf("update chỉ nhận một tham số phiên bản tùy chọn")
			}
		case "--config":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--config thiếu giá trị")
			}
			opts.ConfigPath = argv[i+1]
			i++
		case "--headless":
			opts.Headless = true
		case "--web":
			opts.Web = true
		case "--port":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--port thiếu giá trị")
			}
			p, err := strconv.Atoi(argv[i+1])
			if err != nil || p < 0 || p > 65535 {
				return opts, nil, fmt.Errorf("--port phải là số nguyên 0-65535: %q", argv[i+1])
			}
			opts.Port = p
			i++
		case "--prompt":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--prompt thiếu giá trị")
			}
			opts.Prompt = argv[i+1]
			i++
		case "--prompt-file":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--prompt-file thiếu giá trị")
			}
			opts.PromptFile = argv[i+1]
			i++
		default:
			args = append(args, argv[i])
		}
	}
	if opts.Prompt != "" && opts.PromptFile != "" {
		return opts, nil, fmt.Errorf("--prompt và --prompt-file không thể dùng cùng lúc")
	}
	if opts.Headless && opts.Web {
		return opts, nil, fmt.Errorf("--headless và --web không thể dùng cùng lúc")
	}
	if opts.Port != 0 && !opts.Web {
		return opts, nil, fmt.Errorf("--port chỉ dùng được kết hợp với --web")
	}
	if opts.Version && (opts.Update || opts.ConfigPath != "" || opts.Headless || opts.Web || opts.Prompt != "" || opts.PromptFile != "" || len(args) > 0) {
		return opts, nil, fmt.Errorf("version không thể dùng cùng với tham số khởi động khác")
	}
	if opts.Update && (opts.ConfigPath != "" || opts.Headless || opts.Web || opts.Prompt != "" || opts.PromptFile != "" || len(args) > 0) {
		return opts, nil, fmt.Errorf("update không thể dùng cùng với tham số khởi động khác")
	}
	return opts, args, nil
}

func versionInfo() buildversion.Info {
	return buildversion.Resolve(buildversion.Info{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
}

func runSelfUpdate(target string) error {
	info := versionInfo()
	result, err := buildversion.Update(context.Background(), buildversion.UpdateOptions{
		Repo:           "voocel/ainovel-cli",
		BinaryName:     "ainovel-cli",
		TargetVersion:  target,
		CurrentVersion: info.Version,
	})
	if err != nil {
		return err
	}
	if !result.Updated {
		fmt.Printf("ainovel-cli đã là phiên bản mới nhất %s\n", result.Version)
		return nil
	}
	fmt.Printf("ainovel-cli đã cập nhật lên %s\n", result.Version)
	fmt.Printf("Vị trí cài đặt: %s\n", result.Path)
	return nil
}

func loadPrompt(opts cliOptions) (string, error) {
	if opts.PromptFile == "" {
		return strings.TrimSpace(opts.Prompt), nil
	}

	var data []byte
	var err error
	if opts.PromptFile == "-" {
		data, err = os.ReadFile("/dev/stdin")
	} else {
		data, err = os.ReadFile(opts.PromptFile)
	}
	if err != nil {
		return "", fmt.Errorf("đọc prompt thất bại: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
