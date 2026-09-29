package dashboard

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/pingo/sidecar/internal/operationlog"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	port    int
	server  *http.Server
	apiBase string
}

func NewServer(port int, apiBase string) *Server {
	return &Server{
		port:    port,
		apiBase: apiBase,
	}
}

func (s *Server) Start() error {
	// 去掉 static 前缀，让根路径直接对应 static/ 内容
	subFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("fs.Sub: %w", err)
	}

	mux := http.NewServeMux()
	files := http.FileServer(http.FS(subFS))
	logger := operationlog.New(log.Writer())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		files.ServeHTTP(w, r)
		logger.Event("dashboard", r.Method+" "+r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler: mux,
	}

	go func() {
		log.Printf("INFO: Dashboard server listening on 127.0.0.1:%d", s.port)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("ERROR: Dashboard server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}
