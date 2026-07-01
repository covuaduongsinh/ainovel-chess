package startup

import (
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/host"
)

// PrepareQuick sắp xếp đầu vào trực tiếp thành kế hoạch khởi động nhanh có thể vào Engine.
func PrepareQuick(req Request) (Plan, error) {
	prompt := strings.TrimSpace(req.UserPrompt)
	if prompt == "" {
		return Plan{}, fmt.Errorf("prompt is required")
	}
	return Plan{
		Mode:        ModeQuick,
		DisplayName: "Bắt đầu nhanh",
		StartPrompt: host.BuildStartPrompt(prompt),
		RawPrompt:   prompt,
	}, nil
}
