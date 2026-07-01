package headless

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/tools"
)

func TestTerminalAskUserSingleSelect(t *testing.T) {
	handler := newTerminalAskUser(strings.NewReader("2\n"), &strings.Builder{})
	resp, err := handler.handle(context.Background(), []tools.Question{
		{
			Question: "Bạn muốn phong cách gì?",
			Header:   "Phong cách",
			Options: []tools.Option{
				{Label: "Nhiệt huyết", Description: "Thiên về tiến cấp"},
				{Label: "Huyền bí", Description: "Thiên về bí ẩn"},
			},
		},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := resp.Answers["Bạn muốn phong cách gì?"]; got != "Huyền bí" {
		t.Fatalf("unexpected answer: %q", got)
	}
}

func TestTerminalAskUserCustomInput(t *testing.T) {
	handler := newTerminalAskUser(strings.NewReader("0\nkhông có tuyến tình cảm\n"), &strings.Builder{})
	resp, err := handler.handle(context.Background(), []tools.Question{
		{
			Question: "Còn giới hạn gì thêm không?",
			Header:   "Giới hạn",
			Options: []tools.Option{
				{Label: "U ám", Description: "Tông tổng thể u tối"},
				{Label: "Nhẹ nhàng", Description: "Tông cơ bản tươi sáng"},
			},
		},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := resp.Answers["Còn giới hạn gì thêm không?"]; got != "Tùy chỉnh" {
		t.Fatalf("unexpected answer: %q", got)
	}
	if got := resp.Notes["Còn giới hạn gì thêm không?"]; got != "không có tuyến tình cảm" {
		t.Fatalf("unexpected note: %q", got)
	}
}
