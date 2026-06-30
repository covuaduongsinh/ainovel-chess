package domain

import "testing"

func TestCanTransitionPhase(t *testing.T) {
	tests := []struct {
		from Phase
		to   Phase
		want bool
	}{
		{from: "", to: PhaseInit, want: true},
		{from: PhaseInit, to: PhasePremise, want: true},
		{from: PhaseInit, to: PhaseOutline, want: true},
		{from: PhaseOutline, to: PhaseWriting, want: true},
		{from: PhaseWriting, to: PhaseComplete, want: true},
		{from: PhaseOutline, to: PhasePremise, want: false},
		{from: PhaseComplete, to: PhaseWriting, want: false},
	}
	for _, tt := range tests {
		if got := CanTransitionPhase(tt.from, tt.to); got != tt.want {
			t.Fatalf("CanTransitionPhase(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestCanTransitionFlow(t *testing.T) {
	tests := []struct {
		from FlowState
		to   FlowState
		want bool
	}{
		{from: "", to: FlowRewriting, want: true},
		{from: FlowWriting, to: FlowReviewing, want: true},
		{from: FlowReviewing, to: FlowPolishing, want: true},
		{from: FlowRewriting, to: FlowWriting, want: true},
		{from: FlowSteering, to: FlowRewriting, want: true},
		{from: FlowRewriting, to: FlowReviewing, want: false},
		{from: FlowPolishing, to: FlowReviewing, want: false},
	}
	for _, tt := range tests {
		if got := CanTransitionFlow(tt.from, tt.to); got != tt.want {
			t.Fatalf("CanTransitionFlow(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestExtractNovelNameFromPremise_Placeholder(t *testing.T) {
	cases := []struct {
		name    string
		premise string
		want    string
	}{
		{"tên thật", "# Đêm Dài Sắp Rạng\n\n## Đề tài", "Đêm Dài Sắp Rạng"},
		{"có dấu nháy kép", "# «Bờ Kia Ngân Hà»\n## Đề tài", "Bờ Kia Ngân Hà"},
		{"placeholder-Tên truyện", "# Tên truyện\n## Đề tài", ""},
		{"placeholder-Tên ví dụ", "# «Tên ví dụ»\n## Đề tài", ""},
		{"placeholder-Tên truyện thật", "# Tên truyện thật\n## Đề tài", ""},
		{"dòng đầu không phải tiêu đề", "Văn bản thuần dòng đầu\n# Tên truyện", ""},
	}
	for _, c := range cases {
		if got := ExtractNovelNameFromPremise(c.premise); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}
