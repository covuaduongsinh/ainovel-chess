package web

import (
	"encoding/json"
	"net/http"
	"time"
)

// handleStream là endpoint SSE: đăng ký một client, liên tục ghi các tin nhắn từ hub
// thành text/event-stream. Khi client ngắt kết nối (ctx.Done) thì hủy đăng ký.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	client := s.hub.addClient()
	defer s.hub.removeClient(client)

	ctx := r.Context()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	flusher.Flush()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-client.ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if _, err := w.Write([]byte("data: ")); err != nil {
				return
			}
			if _, err := w.Write(payload); err != nil {
				return
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-ping.C:
			// Dòng chú thích làm heartbeat, duy trì kết nối proxy/trình duyệt.
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
