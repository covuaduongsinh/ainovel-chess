package host

import (
	"testing"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/models"
)

// makeUsageMsg tao mot tin nhan ma callback OnMessage co the chap nhan (co Usage).
// Role phai duoc dat tuong minh thanh assistant: UsageTracker.Record hien tai loc theo role,
// chi tin nhan assistant moi duoc tich luy (cac role khac tu nhien khong co usage).
func makeUsageMsg(input, cacheRead, cacheWrite, output int) agentcore.AgentMessage {
	return agentcore.Message{
		Role: agentcore.RoleAssistant,
		Usage: &agentcore.Usage{
			Input: input, Output: output, CacheRead: cacheRead, CacheWrite: cacheWrite,
		},
	}
}

// Test_pushSample_RingBuffer kiem tra ngu nghia xoay vong cua cua so truot:
// N lan dau them truc tiep; sau do ghi de vao muc cu nhat theo sampleIdx. recentSums luon phan anh "N lan gan nhat".
func Test_pushSample_RingBuffer(t *testing.T) {
	var tot agentTotals

	for i := 1; i <= recentSampleCap; i++ {
		pushSample(&tot, i, i*100)
	}
	if got := len(tot.samples); got != recentSampleCap {
		t.Fatalf("after %d pushes, samples len=%d want %d", recentSampleCap, got, recentSampleCap)
	}

	pushSample(&tot, 999, 99900)
	if got := len(tot.samples); got != recentSampleCap {
		t.Fatalf("after overflow, samples len=%d want %d (no growth)", got, recentSampleCap)
	}
	cacheRead, input := recentSums(&tot)
	expectedCacheRead := 999
	expectedInput := 99900
	for i := 2; i <= recentSampleCap; i++ {
		expectedCacheRead += i
		expectedInput += i * 100
	}
	if cacheRead != expectedCacheRead || input != expectedInput {
		t.Fatalf("recentSums after overflow = (%d, %d), want (%d, %d)",
			cacheRead, input, expectedCacheRead, expectedInput)
	}
}

// Test_UsageTracker_RecordAccumulates kiem tra Record tich luy nhieu role dung,
// tong hop = tong cua tat ca role; moi role doc lap nhau.
func Test_UsageTracker_RecordAccumulates(t *testing.T) {
	tk := NewUsageTracker(nil, nil) // modelSet=nil -> di qua du phong provider Cost, khong anh huong logic tich luy

	tk.Record("writer", makeUsageMsg(1000, 800, 0, 200))
	tk.Record("writer", makeUsageMsg(1500, 1200, 100, 300))
	tk.Record("editor", makeUsageMsg(500, 0, 0, 100))

	cost, in, out, cr, cw := tk.Totals()
	if in != 3000 || out != 600 || cr != 2000 || cw != 100 {
		t.Fatalf("totals = (in=%d out=%d cr=%d cw=%d), want (3000 600 2000 100)", in, out, cr, cw)
	}
	if cost != 0 {
		t.Errorf("cost should be 0 when modelSet=nil and no provider Cost, got %v", cost)
	}

	per := tk.PerAgent()
	if len(per) != 2 {
		t.Fatalf("per-agent len=%d want 2", len(per))
	}
	// PerAgent giam dan theo CacheRead: writer (2000) phai xep truoc editor (0)
	if per[0].Role != "writer" || per[1].Role != "editor" {
		t.Fatalf("per-agent order = %s,%s want writer,editor", per[0].Role, per[1].Role)
	}
	if per[0].Input != 2500 || per[0].CacheRead != 2000 {
		t.Errorf("writer totals = (in=%d cr=%d), want (2500 2000)", per[0].Input, per[0].CacheRead)
	}
}

// Test_UsageTracker_ArchitectAliasNormalized kiem tra architect_short/mid/long
// deu quy ve cung mot key "architect" (tranh bi /model cat thanh nhieu dong khi chuyen sub-role).
func Test_UsageTracker_ArchitectAliasNormalized(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	tk.Record("architect_short", makeUsageMsg(100, 50, 0, 20))
	tk.Record("architect_mid", makeUsageMsg(200, 100, 0, 40))
	tk.Record("architect_long", makeUsageMsg(300, 150, 0, 60))

	per := tk.PerAgent()
	if len(per) != 1 {
		t.Fatalf("aliases must merge to single role, got %d entries: %+v", len(per), per)
	}
	if per[0].Role != "architect" {
		t.Fatalf("merged role name = %q, want architect", per[0].Role)
	}
	if per[0].Input != 600 || per[0].CacheRead != 300 {
		t.Errorf("merged totals = (in=%d cr=%d), want (600 300)", per[0].Input, per[0].CacheRead)
	}
}

func Test_UsageTracker_PerModelAccumulates(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	tk.accumulate("writer", "openrouter", "model-a", agentcore.Usage{Input: 1000, Output: 200, CacheRead: 700})
	tk.accumulate("editor", "openrouter", "model-b", agentcore.Usage{Input: 500, Output: 100})
	tk.accumulate("writer", "openrouter", "model-a", agentcore.Usage{Input: 300, Output: 80, CacheRead: 200})

	perModel := tk.PerModel()
	if len(perModel) != 2 {
		t.Fatalf("per-model len=%d want 2", len(perModel))
	}
	seen := map[string]AgentUsage{}
	for _, m := range perModel {
		seen[m.Model] = m
	}
	if seen["openrouter/model-a"].Input != 1300 || seen["openrouter/model-a"].CacheRead != 900 {
		t.Errorf("model-a totals = %+v", seen["openrouter/model-a"])
	}
	if seen["openrouter/model-b"].Output != 100 {
		t.Errorf("model-b totals = %+v", seen["openrouter/model-b"])
	}

	snap := tk.Snapshot()
	restored := NewUsageTracker(nil, nil)
	restored.applyState(snap)
	if got := restored.PerModel(); len(got) != 2 {
		t.Fatalf("restored per-model len=%d want 2: %+v", len(got), got)
	}
}

func Test_UsageTracker_RecordUsesActualUsageModel(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	tk.Record("writer", agentcore.Message{
		Role: agentcore.RoleAssistant,
		Usage: &agentcore.Usage{
			Provider: "openrouter",
			Model:    "google/gemini-2.5-pro",
			Input:    1000,
			Output:   200,
		},
	})

	perModel := tk.PerModel()
	if len(perModel) != 1 {
		t.Fatalf("per-model len=%d want 1: %+v", len(perModel), perModel)
	}
	if perModel[0].Model != "openrouter/google/gemini-2.5-pro" {
		t.Fatalf("model key = %q, want openrouter/google/gemini-2.5-pro", perModel[0].Model)
	}
	if perModel[0].Input != 1000 || perModel[0].Output != 200 {
		t.Fatalf("model totals = %+v", perModel[0])
	}
}

func Test_UsageTracker_ProviderOnlyDoesNotInventModelKey(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	tk.Record("writer", agentcore.Message{
		Role: agentcore.RoleAssistant,
		Usage: &agentcore.Usage{
			Provider: "openrouter",
			Input:    1000,
			Output:   200,
		},
	})

	if got := tk.PerModel(); len(got) != 0 {
		t.Fatalf("provider-only usage must not create model stats without a model, got %+v", got)
	}
}

// Test_UsageTracker_RecentWindowReflectsLatest kiem tra cua so truot phan anh "N lan gan nhat",
// khong bi kem hieu qua suc hit dau ky -- day chinh la van de "keo lui giai doan dau vs hit thap o trang thai on dinh" ma P1 giai quyet.
func Test_UsageTracker_RecentWindowReflectsLatest(t *testing.T) {
	tk := NewUsageTracker(nil, nil)

	// 5 lan dau hit rat thap (kich ban chuong dau)
	for i := 0; i < 5; i++ {
		tk.Record("writer", makeUsageMsg(1000, 0, 0, 200))
	}
	// 8 lan sau (>5) hit cao (kich ban trang thai on dinh)
	for i := 0; i < 8; i++ {
		tk.Record("writer", makeUsageMsg(1000, 900, 0, 200))
	}

	per := tk.PerAgent()
	if len(per) != 1 {
		t.Fatalf("len=%d want 1", len(per))
	}
	w := per[0]

	// Tich luy: 13 lan trong do 8 lan co hit -> 7200/13000 ≈ 55.4%
	cumulativeRate := float64(w.CacheRead) / float64(w.Input) * 100
	if cumulativeRate < 50 || cumulativeRate > 60 {
		t.Errorf("cumulative hit rate = %.1f%%, want ~55%%", cumulativeRate)
	}

	// Cua so truot: 10 lan gan nhat trong do 8 lan hit cao + 2 lan khong hit -> 7200/10000 = 72%
	if w.RecentSamples != recentSampleCap {
		t.Errorf("recent samples = %d, want %d (window full)", w.RecentSamples, recentSampleCap)
	}
	recentRate := float64(w.RecentCacheRead) / float64(w.RecentInput) * 100
	if recentRate < 70 || recentRate > 75 {
		t.Errorf("recent hit rate = %.1f%%, want ~72%% (proves window dropped early misses)", recentRate)
	}
	// Quan trong: N lan gan nhat ro rang cao hon tich luy, chung to so 0 dau ky da bi day ra khoi cua so
	if recentRate <= cumulativeRate {
		t.Errorf("recent (%.1f%%) must exceed cumulative (%.1f%%) once window slides past early misses",
			recentRate, cumulativeRate)
	}
}

// Test_computeSaved kiem tra thuat toan saved: CacheRead x (gia Input - gia CacheRead);
// khi chenh lech gia <= 0 hoac InputCost <= 0 thi tra ve 0 (phi ton kem CacheWrite khong khau tru).
func Test_computeSaved(t *testing.T) {
	cases := []struct {
		name  string
		usage agentcore.Usage
		entry models.ModelEntry
		want  float64
	}{
		{
			name:  "anthropic 5m hit tiet kiem 90%",
			usage: agentcore.Usage{Input: 100_000, CacheRead: 80_000},
			entry: models.ModelEntry{InputCostPer1M: 3.0, CacheReadCostPer1M: 0.3},
			want:  80_000 * (3.0 - 0.3) / 1_000_000, // 0.216
		},
		{
			name:  "khong hit saved=0",
			usage: agentcore.Usage{Input: 100_000, CacheRead: 0},
			entry: models.ModelEntry{InputCostPer1M: 3.0, CacheReadCostPer1M: 0.3},
			want:  0,
		},
		{
			name:  "mo hinh chua niem gia saved=0",
			usage: agentcore.Usage{Input: 100_000, CacheRead: 50_000},
			entry: models.ModelEntry{InputCostPer1M: 0, CacheReadCostPer1M: 0},
			want:  0,
		},
		{
			name:  "chenh lech gia bat thuong saved=0",
			usage: agentcore.Usage{Input: 100_000, CacheRead: 50_000},
			entry: models.ModelEntry{InputCostPer1M: 1.0, CacheReadCostPer1M: 2.0}, // cache lai dat hon
			want:  0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeSaved(tc.usage, tc.entry)
			if got != tc.want {
				t.Errorf("computeSaved=%v want %v", got, tc.want)
			}
		})
	}
}

// Test_UsageTracker_CacheCapableSticky kiem tra CacheCapable khi da dat true thi khong lui ve false.
// Tung chay mo hinh ho tro cache -> du lieu tich luy hit hop le; chuyen sang mo hinh khong ho tro khong nen lam lui co nay.
//
// Mo phong bang cach xay dung perAgent dat gia tri truc tiep (duong resolveCost can ModelSet+Registry, lop tich hop da kiem tra).
func Test_UsageTracker_CacheCapableSticky(t *testing.T) {
	tk := NewUsageTracker(nil, nil)

	// Mo phong "tung chay mo hinh ho tro cache + da hit"
	tk.perAgent["writer"] = &agentTotals{
		Input: 1000, CacheRead: 500, Output: 200, CacheCapable: true,
	}
	// Them mot lan goi "mo hinh khong ho tro cache"
	tk.Record("writer", makeUsageMsg(500, 0, 0, 100))

	per := tk.PerAgent()
	if len(per) != 1 || per[0].Role != "writer" {
		t.Fatalf("expected single writer entry, got %+v", per)
	}
	if !per[0].CacheCapable {
		t.Errorf("CacheCapable must remain true after switching to non-capable model")
	}
	if per[0].CacheRead != 500 || per[0].Input != 1500 {
		t.Errorf("totals after merge = (in=%d cr=%d), want (1500 500)",
			per[0].Input, per[0].CacheRead)
	}
}

// Test_UsageTracker_PerAgentSkipsZero kiem tra role chua tieu thu token khong xuat hien trong PerAgent.
func Test_UsageTracker_PerAgentSkipsZero(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	// Tao mot role nhung khong tieu thu token (truong hop cuc doan)
	tk.perAgent["ghost"] = &agentTotals{}
	tk.Record("writer", makeUsageMsg(100, 50, 0, 20))

	per := tk.PerAgent()
	if len(per) != 1 || per[0].Role != "writer" {
		t.Fatalf("ghost role with zero tokens must be skipped, got %+v", per)
	}
}

// Test_UsageTracker_MissingAssistantUsageCounted kiem tra ranh gioi phan dinh cua missingAssistantUsage:
//   - Duong tich luy chi xem Usage != nil (khong rang buoc Role)
//   - Duong chan doan yeu cau Role=Assistant va Content khong rong -- day moi giong "mot phan hoi LLM that nhung
//     khong nhan duoc usage", tuong ung bieu hien dien hinh cua upstream streaming khong gui final chunk include_usage cua OpenAI.
//     Cac truong hop khac (tin nhan user/tool, assistant co content rong) khong tinh la missing.
func Test_UsageTracker_MissingAssistantUsageCounted(t *testing.T) {
	tk := NewUsageTracker(nil, nil)

	withContent := func(text string) agentcore.Message {
		return agentcore.Message{
			Role:    agentcore.RoleAssistant,
			Content: []agentcore.ContentBlock{agentcore.TextBlock(text)},
		}
	}

	// assistant + co Content + nil Usage -> trong giong phan hoi that nhung thieu usage, tinh vao chan doan
	tk.Record("writer", withContent("hi"))
	tk.Record("writer", withContent("again"))
	// assistant nhung Content rong -> duong khoi phuc bat thuong hoac tin nhan placeholder, khong tinh la missing
	tk.Record("writer", agentcore.Message{Role: agentcore.RoleAssistant})
	// Tin nhan user/tool tu nhien khong mang usage, du Content co rong hay khong cung khong tinh la missing
	tk.Record("writer", agentcore.Message{Role: agentcore.RoleUser, Content: []agentcore.ContentBlock{agentcore.TextBlock("u")}})
	tk.Record("writer", agentcore.Message{Role: agentcore.RoleTool, Content: []agentcore.ContentBlock{agentcore.TextBlock("t")}})
	// Co usage binh thuong -> di qua duong tich luy, khong tinh vao chan doan
	tk.Record("writer", makeUsageMsg(100, 50, 0, 20))

	if got := tk.MissingAssistantUsage(); got != 2 {
		t.Errorf("MissingAssistantUsage=%d, want 2", got)
	}
	_, in, _, _, _ := tk.Totals()
	if in != 100 {
		t.Errorf("Duong binh thuong bi pha hong, input=%d want 100", in)
	}
}

// Test_UsageTracker_CacheCapableFromFacts kiem tra CacheCapable van co the duoc danh dau true dua vao "thuc te"
// khi khong tim thay mo hinh trong registry: mo hinh cua backend tu xay / proxy noi dia thuong khong co trong
// pricing index cua BerriAI/litellm, resolveCost tra ve capable=false; nhung chi can backend that su tra ve
// CacheRead hoac CacheWrite > 0, la bang chung mo hinh khach quan ho tro prompt cache, dong per-role
// khong nen hien thi "chua bat".
func Test_UsageTracker_CacheCapableFromFacts(t *testing.T) {
	tk := NewUsageTracker(nil, nil) // modelSet=nil -> resolveCost luon capable=false

	// Mot lan co CacheWrite (mo phong lan dau ghi vao cache, registry khong danh dau capable, nhung thuc te chung minh ho tro)
	tk.Record("writer", makeUsageMsg(1000, 0, 200, 100))
	per := tk.PerAgent()
	if len(per) != 1 || !per[0].CacheCapable {
		t.Fatalf("CacheWrite > 0 nen danh dau CacheCapable=true ngay, got %+v", per)
	}
	if !tk.OverallCacheCapable() {
		t.Errorf("overall CacheCapable cung nen dong bo dat true")
	}

	// Nguoc lai: role hoan toan khong co hoat dong cache, CacheCapable phai giu false
	tk.Record("editor", makeUsageMsg(500, 0, 0, 100))
	per = tk.PerAgent()
	for _, a := range per {
		if a.Role == "editor" && a.CacheCapable {
			t.Errorf("editor khong co CacheRead/Write trong toan bo qua trinh, CacheCapable khong nen duoc danh dau nham la true")
		}
	}
}

// Test_UsageTracker_AccumulatesAnyRoleWithUsage kiem tra duong tich luy doc lap khoi Role:
// ke ca khi mot adapter nao do trong tuong lai lap rap usage vao tin nhan role khong phai assistant,
// van tich luy dung. Giu vung hop dong "quy tac lap rap va quy tac tich luy doc lap nhau".
func Test_UsageTracker_AccumulatesAnyRoleWithUsage(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	// Tao mot tin nhan theo ly thuyet it gap, co Usage nhung khong phai assistant
	hypothetical := agentcore.Message{
		Role:  agentcore.RoleSystem,
		Usage: &agentcore.Usage{Input: 200, Output: 50, CacheRead: 100},
	}
	tk.Record("writer", hypothetical)

	_, in, out, cr, _ := tk.Totals()
	if in != 200 || out != 50 || cr != 100 {
		t.Errorf("Khong tich luy theo truong Usage, got (in=%d out=%d cr=%d) want (200 50 100)", in, out, cr)
	}
	if tk.MissingAssistantUsage() != 0 {
		t.Errorf("Co Usage khong nen tinh vao missing")
	}
}

// Test_UsageTracker_OnCostCallback kiem tra diem noi cua budget sentinel: sau moi lan ghi tai
// callback ngoai lock mang theo chi phi tich luy moi nhat (bao gom duong provider tu bao cost).
func Test_UsageTracker_OnCostCallback(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	var got []float64
	tk.SetOnCost(func(total float64) { got = append(got, total) })

	msg := func(cost float64) agentcore.AgentMessage {
		return agentcore.Message{
			Role:  agentcore.RoleAssistant,
			Usage: &agentcore.Usage{Input: 100, Output: 10, Cost: &agentcore.Cost{Total: cost}},
		}
	}
	tk.Record("writer", msg(0.5))
	tk.Record("writer", msg(0.25))

	if len(got) != 2 || got[0] != 0.5 || got[1] != 0.75 {
		t.Fatalf("onCost should carry growing totals, got %v", got)
	}
}

// Test_UsageTracker_OnMissingUsageOnce kiem tra callback vung mu chi kich hoat lan dau tien.
func Test_UsageTracker_OnMissingUsageOnce(t *testing.T) {
	tk := NewUsageTracker(nil, nil)
	fired := 0
	tk.SetOnMissingUsage(func() { fired++ })

	noUsage := agentcore.Message{Role: agentcore.RoleAssistant, Content: []agentcore.ContentBlock{agentcore.TextBlock("van ban")}}
	tk.Record("writer", noUsage)
	tk.Record("writer", noUsage)
	tk.Record("editor", noUsage)

	if fired != 1 {
		t.Fatalf("onMissingUsage should fire exactly once, got %d", fired)
	}
}
