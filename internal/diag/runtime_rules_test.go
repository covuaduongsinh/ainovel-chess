package diag

import "testing"

// TestRuntimeFindings_Classify chung minh chu ky lap duoc phan loai theo hinh thai, nguong tang/ha cap dung,
// va tat ca Finding thoi gian chay deu la AutoNone (nguyen tac quan sat: chi chan doan khong tao Action).
func TestRuntimeFindings_Classify(t *testing.T) {
	rc := RuntimeCapture{
		Repeats: []RepeatStat{
			{Sig: "coordinator · err: InputValidationError", Count: 14}, // vong lap loi critical
			{Sig: "coordinator · subagent", Count: 45},                  // cong cu tan so cao binh thuong -> khong tao Finding
			{Sig: "writer · save_plan (args invalid)", Count: 4},        // tham so khong hop le warning
		},
		StuckStep:  "writing.commit_ch07",
		StuckCount: 9, // bi ket critical
		LogKinds:   map[string]int{"stream_idle": 4},
		LogErrors:  270, // tich luy trong luong chay dai, khong nen tao Finding rieng le
	}

	fs := runtimeFindings(&rc)
	sev := map[string]Severity{}
	for _, f := range fs {
		sev[f.Rule] = f.Severity
		if f.AutoLevel != AutoNone {
			t.Errorf("%s phai la AutoNone (nguyen tac quan sat), got %s", f.Rule, f.AutoLevel)
		}
	}

	want := map[string]Severity{
		"RepeatedToolError": SevCritical,
		"ArgsInvalidLoop":   SevWarning,
		"StuckStep":         SevCritical,
		"StreamIdleStorm":   SevWarning,
	}
	for rule, w := range want {
		if sev[rule] != w {
			t.Errorf("%s: got %q want %q", rule, sev[rule], w)
		}
	}
	// Cong cu tan so cao binh thuong / tich luy error log khong nen tao Finding (tranh bao nham trong luong chay dai).
	if _, ok := sev["RepeatedToolCall"]; ok {
		t.Error("Lap cong cu binh thuong khong nen tao Finding")
	}
	if _, ok := sev["LogErrorBurst"]; ok {
		t.Error("Tich luy error log khong nen tao Finding rieng le")
	}
}

// TestRuntimeFindings_Quiet chung minh khi khong co tin hieu bat thuong se khong tao bat ky Finding nao (khong bao nham).
func TestRuntimeFindings_Quiet(t *testing.T) {
	rc := RuntimeCapture{
		LogKinds:  map[string]int{"stream_idle": 1}, // duoi nguong
		LogErrors: 2,
	}
	if fs := runtimeFindings(&rc); len(fs) != 0 {
		t.Errorf("trang thai yen tinh khong nen tao Finding, got %d: %+v", len(fs), fs)
	}
}
