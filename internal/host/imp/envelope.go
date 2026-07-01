package imp

import (
	"fmt"
	"regexp"
	"strings"
)

// envelopeTagRe Khop dong === TAG === (co the co khoang trang truoc sau), khong phan biet hoa thuong.
var envelopeTagRe = regexp.MustCompile(`(?m)^\s*===\s*([A-Z_]+)\s*===\s*$`)

// parseTaggedEnvelope Phan tich dau ra nhieu doan dang `=== TAG ===\nbody...` thanh map.
// key la ten the viet hoa, value la doan tuong ung (da trim khoang trang dau cuoi).
// Khi xuat hien the trung lap, the sau ghi de the truoc.
func parseTaggedEnvelope(text string) map[string]string {
	matches := envelopeTagRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make(map[string]string, len(matches))
	for i, m := range matches {
		tag := strings.ToUpper(text[m[2]:m[3]])
		bodyStart := m[1]
		bodyEnd := len(text)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		}
		out[tag] = strings.TrimSpace(text[bodyStart:bodyEnd])
	}
	return out
}

// requireTags Kiem tra envelope phai chua cac the cho truoc va khong rong.
func requireTags(env map[string]string, tags ...string) error {
	var missing []string
	for _, t := range tags {
		if strings.TrimSpace(env[t]) == "" {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required tags: %s", strings.Join(missing, ", "))
	}
	return nil
}
