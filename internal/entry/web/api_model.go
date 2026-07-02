package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

// roleList là danh sách các vai có thể cấu hình mô hình/mức độ suy luận riêng (nhất quán với nội bộ host).
// "" đại diện cho mặc định (default).
var roleList = []string{"default", "coordinator", "architect", "writer", "editor"}

type providerInfo struct {
	Name   string   `json:"name"`
	Models []string `json:"models"`
}

type roleSelection struct {
	Role     string `json:"role"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type modelsResponse struct {
	Providers []providerInfo  `json:"providers"`
	Roles     []roleSelection `json:"roles"`
}

func (s *Server) handleModels(w http.ResponseWriter, _ *http.Request) {
	var resp modelsResponse
	for _, p := range s.eng.ConfiguredProviders() {
		resp.Providers = append(resp.Providers, providerInfo{
			Name:   p,
			Models: s.eng.ConfiguredModels(p),
		})
	}
	for _, role := range roleList {
		lookup := role
		if role == "default" {
			lookup = ""
		}
		provider, model, _ := s.eng.CurrentModelSelection(lookup)
		resp.Roles = append(resp.Roles, roleSelection{Role: role, Provider: provider, Model: model})
	}
	writeOK(w, resp)
}

func (s *Server) handleSwitchModel(w http.ResponseWriter, r *http.Request) {
	var req modelRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	role := normalizeRole(req.Role)
	if err := s.eng.SwitchModel(role, req.Provider, req.Model); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeOK(w, nil)
}

type modelAutoRequest struct {
	Preset string `json:"preset"` // standard (mặc định) | economy
}

// handleModelAuto áp một preset Claude (theo body {preset}) cho cả bốn vai qua provider
// claude-code. Body rỗng = preset "Chuẩn". Yêu cầu provider "claude-code" đã được cấu hình.
func (s *Server) handleModelAuto(w http.ResponseWriter, r *http.Request) {
	var req modelAutoRequest
	_ = decodeBody(r, &req) // body tùy chọn: rỗng → preset mặc định
	preset, ok := bootstrap.ClaudePresetByKey(req.Preset)
	if !ok {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("preset không hợp lệ %q (chọn: %s)", req.Preset, bootstrap.PresetKeysHint()))
		return
	}
	for _, rp := range preset.Assignments() {
		if err := s.eng.SwitchModel(rp.Role, bootstrap.ClaudeCodeProvider, rp.Model); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := s.eng.SetRoleThinking(rp.Role, rp.Effort); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	writeOK(w, map[string]any{"preset": preset.Key, "label": preset.Label})
}

type thinkingResponse struct {
	Roles []roleThinking `json:"roles"`
}

type roleThinking struct {
	Role      string   `json:"role"`
	Current   string   `json:"current"`
	Available []string `json:"available"`
}

func (s *Server) handleThinking(w http.ResponseWriter, _ *http.Request) {
	var resp thinkingResponse
	for _, role := range roleList {
		lookup := role
		if role == "default" {
			lookup = ""
		}
		avail := make([]string, 0)
		for _, lv := range s.eng.AvailableThinking(lookup) {
			avail = append(avail, string(lv))
		}
		resp.Roles = append(resp.Roles, roleThinking{
			Role:      role,
			Current:   s.eng.CurrentThinking(lookup),
			Available: avail,
		})
	}
	writeOK(w, resp)
}

func (s *Server) handleSetThinking(w http.ResponseWriter, r *http.Request) {
	var req thinkingRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.eng.SetRoleThinking(normalizeRole(req.Role), req.Level); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeOK(w, nil)
}

// normalizeRole chuẩn hóa "default" từ frontend thành "" mà host mong đợi (mặc định).
func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "default" {
		return ""
	}
	return role
}
