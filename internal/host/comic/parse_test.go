package comic

import (
	"strings"
	"testing"
)

// TestRepairTrailingComma kiểm tra bộ sửa JSON.
//
// Ca "dấu phẩy trong lời thoại" là quan trọng nhất: câu tiếng Việt đầy dấu phẩy, cắt nhầm
// một dấu bên trong chuỗi là làm hỏng nội dung chứ không phải sửa cú pháp.
func TestRepairTrailingComma(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"phẩy thừa trong object", `{"a":1,}`, `{"a":1}`},
		{"phẩy thừa trong mảng", `{"a":[1,2,]}`, `{"a":[1,2]}`},
		{"phẩy thừa có xuống dòng", "{\"a\":1,\n  }", "{\"a\":1\n  }"},
		{"phẩy thừa lồng nhau", `{"a":{"b":[1,],},}`, `{"a":{"b":[1]}}`},
		{"JSON hợp lệ giữ nguyên", `{"a":1,"b":2}`, `{"a":1,"b":2}`},
		{
			"KHÔNG đụng dấu phẩy trong lời thoại",
			`{"text":"Con nhìn kỹ nước cờ này, rồi hãy đi, nhé,"}`,
			`{"text":"Con nhìn kỹ nước cờ này, rồi hãy đi, nhé,"}`,
		},
		{
			"KHÔNG đụng dấu phẩy trước ngoặc trong chuỗi",
			`{"text":"đóng ngoặc ,] và ,} nằm trong chuỗi"}`,
			`{"text":"đóng ngoặc ,] và ,} nằm trong chuỗi"}`,
		},
		{
			"nháy được escape không làm lệch trạng thái",
			`{"text":"anh ấy nói \"đi thôi,\" rồi quay đi",}`,
			`{"text":"anh ấy nói \"đi thôi,\" rồi quay đi"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := repairJSON(c.in); got != c.want {
				t.Errorf("repairJSON(%q)\n  = %q\n  muốn %q", c.in, got, c.want)
			}
		})
	}
}

// TestParseJSONPayloadRecovers khẳng định đường phân tích thật sự cứu được output có dấu
// phẩy thừa — đúng lỗi đã gặp khi sinh model sheet nhân vật.
func TestParseJSONPayloadRecovers(t *testing.T) {
	raw := `<output>
{
  "characters": [
    {
      "name": "Wolf",
      "role": "nhân vật chính",
      "appearance": "cậu bé gầy, chân khoèo, mắt sáng",
      "canonical_prompt": "a lean boy with a club foot",
    },
  ]
}
</output>`
	var res struct {
		Characters []CharacterSheet `json:"characters"`
	}
	if err := parseJSONPayload(extractOutput(raw), &res); err != nil {
		t.Fatalf("phải cứu được JSON có dấu phẩy thừa, nhận lỗi: %v", err)
	}
	if len(res.Characters) != 1 || res.Characters[0].Name != "Wolf" {
		t.Fatalf("nội dung sai sau khi sửa: %+v", res.Characters)
	}
	if !strings.Contains(res.Characters[0].Appearance, "chân khoèo") {
		t.Errorf("dấu tiếng Việt trong nội dung bị hỏng: %q", res.Characters[0].Appearance)
	}
}

// TestParseJSONPayloadErrorHasContext khẳng định lỗi không cứu được thì kèm đoạn trích —
// không có đoạn trích thì lỗi JSON của LLM gần như không chẩn đoán được từ log.
func TestParseJSONPayloadErrorHasContext(t *testing.T) {
	var v map[string]any
	err := parseJSONPayload(`{"a": @@@ }`, &v)
	if err == nil {
		t.Fatal("phải báo lỗi với JSON hỏng")
	}
	if !strings.Contains(err.Error(), "quanh vị trí") {
		t.Errorf("thông báo lỗi thiếu đoạn trích ngữ cảnh: %v", err)
	}
}
