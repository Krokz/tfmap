package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/Krokz/tfmap/internal/model"
	"github.com/gorilla/websocket"
)

type Server struct {
	mu      sync.RWMutex
	project *model.Project
	clients map[*websocket.Conn]bool
	clMu    sync.Mutex
	distFS  fs.FS
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return isLocalhostOrigin(origin)
	},
}

func isLocalhostOrigin(origin string) bool {
	return strings.HasPrefix(origin, "http://127.0.0.1:") ||
		strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://[::1]:")
}

func New(project *model.Project, distFS fs.FS) *Server {
	return &Server{
		project: project,
		clients: make(map[*websocket.Conn]bool),
		distFS:  distFS,
	}
}

func (s *Server) UpdateProject(project *model.Project) {
	s.mu.Lock()
	s.project = project
	s.mu.Unlock()
	s.broadcast(map[string]string{"type": "reload"})
}

func (s *Server) broadcast(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	s.clMu.Lock()
	defer s.clMu.Unlock()
	for conn := range s.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			delete(s.clients, conn)
		}
	}
}

func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	project := s.project
	s.mu.RUnlock()

	origin := r.Header.Get("Origin")
	if isLocalhostOrigin(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	s.clMu.Lock()
	s.clients[conn] = true
	s.clMu.Unlock()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			s.clMu.Lock()
			delete(s.clients, conn)
			s.clMu.Unlock()
			conn.Close()
			return
		}
	}
}

func (s *Server) Serve(listener net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/project", s.handleProject)
	mux.HandleFunc("/ws", s.handleWS)

	if distFS := s.getDistFS(); distFS != nil {
		fileServer := http.FileServer(http.FS(distFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path != "/" {
				cleanPath := strings.TrimPrefix(path, "/")
				if _, err := fs.Stat(distFS, cleanPath); err == nil {
					fileServer.ServeHTTP(w, r)
					return
				}
			}
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})
	}

	return http.Serve(listener, mux)
}

func (s *Server) getDistFS() fs.FS {
	if s.distFS != nil {
		if _, err := fs.Stat(s.distFS, "index.html"); err == nil {
			return s.distFS
		}
	}
	if info, err := os.Stat("web/dist"); err == nil && info.IsDir() {
		return os.DirFS("web/dist")
	}
	return nil
}
