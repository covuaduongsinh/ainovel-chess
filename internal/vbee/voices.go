package vbee

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// maxVoicePages chặn số trang khi lặp con trỏ, phòng máy chủ trả con trỏ lặp vô hạn.
const maxVoicePages = 20

// Voices lấy MỘT trang giọng đọc.
//
// CHÚ Ý: endpoint này nằm ở host KHÁC với endpoint TTS (vbee.vn vs api.vbee.vn).
// Nếu token bị giới hạn theo host thì lời gọi này có thể 401 trong khi TTS vẫn chạy —
// người gọi nên coi lỗi ở đây là không nghiêm trọng và cho phép nhập mã giọng tay.
func (c *Client) Voices(ctx context.Context, q VoiceQuery) (VoicePage, error) {
	u := c.voicesURL + "/api/public/v1/voices"
	vals := url.Values{}
	if s := strings.TrimSpace(q.Ownership); s != "" {
		vals.Set("voiceOwnership", s)
	}
	if s := strings.TrimSpace(q.LanguageCode); s != "" {
		vals.Set("languageCode", s)
	}
	if s := strings.TrimSpace(q.Gender); s != "" {
		vals.Set("gender", s)
	}
	if q.Limit > 0 {
		vals.Set("limit", strconv.Itoa(q.Limit))
	}
	if s := strings.TrimSpace(q.Cursor); s != "" {
		vals.Set("cursor", s)
	}
	if len(vals) > 0 {
		u += "?" + vals.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return VoicePage{}, err
	}
	c.setAuth(req)

	resp, err := c.do(c.http, req)
	if err != nil {
		return VoicePage{}, err
	}
	defer resp.Body.Close()

	var env voicesEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return VoicePage{}, fmt.Errorf("đọc danh sách giọng thất bại: %w", err)
	}
	return VoicePage{
		Voices:      env.Result.Voices,
		HasNextPage: env.Result.Pagination.HasNextPage,
		NextCursor:  env.Result.Pagination.NextCursor,
	}, nil
}

// ListAllVoices lặp con trỏ cho tới hết (chặn ở maxVoicePages trang) và trả về danh
// sách gộp. Trang cuối cùng lấy được vẫn trả về ngay cả khi trang sau lỗi.
func (c *Client) ListAllVoices(ctx context.Context, q VoiceQuery) ([]Voice, error) {
	if q.Limit <= 0 {
		q.Limit = 100
	}
	var all []Voice
	seen := make(map[string]bool)
	for page := 0; page < maxVoicePages; page++ {
		p, err := c.Voices(ctx, q)
		if err != nil {
			if len(all) > 0 {
				return all, nil // đã lấy được ít nhất một trang thì dùng tạm
			}
			return nil, err
		}
		for _, v := range p.Voices {
			if v.Code == "" || seen[v.Code] {
				continue
			}
			seen[v.Code] = true
			all = append(all, v)
		}
		if !p.HasNextPage || p.NextCursor == "" || p.NextCursor == q.Cursor {
			break
		}
		q.Cursor = p.NextCursor
	}
	return all, nil
}
