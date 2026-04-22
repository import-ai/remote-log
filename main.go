package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[chan string]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[chan string]struct{}),
	}
}

func (h *Hub) Register(ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = struct{}{}
}

func (h *Hub) Unregister(ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
}

func (h *Hub) Broadcast(msg string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
			// slow client, drop
		}
	}
}

func writeSSE(w io.Writer, data string) {
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func main() {
	hub := NewHub()

	http.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(htmlPage))
	})

	http.HandleFunc("POST /api/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "bad request"})
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
			return
		}

		text, ok := payload["text"].(string)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": `expected {"text":"..."}`})
			return
		}

		hub.Broadcast(text)
		w.WriteHeader(http.StatusNoContent)
	})

	http.HandleFunc("GET /api/v1/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		ch := make(chan string, 4)
		hub.Register(ch)
		defer hub.Unregister(ch)

		writeSSE(w, "connected")
		flusher.Flush()

		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					return
				}
				writeSSE(w, msg)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	log.Println("Server listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

const htmlPage = `<!DOCTYPE html>
<html>
<head>
    <title>Log Stream</title>
    <style>
        body { font-family: monospace; margin: 20px; background: #1e1e1e; color: #d4d4d4; }
        #logs { white-space: pre-wrap; word-break: break-all; }
        .log-entry { border-bottom: 1px solid #333; padding: 4px 0; }
    </style>
</head>
<body>
    <h1>Realtime Logs</h1>
    <div id="logs"></div>
    <script>
        const logs = document.getElementById('logs');
        const es = new EventSource('/api/v1/stream');
        es.onmessage = (e) => {
            if (e.data === 'connected') return;
            const div = document.createElement('div');
            div.className = 'log-entry';
            div.textContent = e.data;
            logs.appendChild(div);
        };
        es.onerror = (e) => {
            console.error('SSE error', e);
        };
    </script>
</body>
</html>`
