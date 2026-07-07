package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/lominodev/lominodeploy/internal/config"
)

type Server struct {
	cfg     *config.Config
	webFS   embed.FS
	subFS   fs.FS
	tmpls   map[string]*template.Template
	funcMap template.FuncMap
	version string
	llsURL  string
	mux     *http.ServeMux
}

func New(cfg *config.Config, webFS embed.FS, version, llsURL string) *Server {
	s := &Server{
		cfg:     cfg,
		webFS:   webFS,
		version: version,
		llsURL:  llsURL,
		mux:     http.NewServeMux(),
	}
	s.loadTemplates()
	s.registerRoutes()
	return s
}

func (s *Server) Start(port int) error {
	addr := fmt.Sprintf(":%d", port)
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) loadTemplates() {
	sub, err := fs.Sub(s.webFS, "web/templates")
	if err != nil {
		panic("no se pudo acceder a web/templates: " + err.Error())
	}
	s.subFS = sub

	s.funcMap = template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"pct": func(used, total float64) int {
			if total == 0 {
				return 0
			}
			return int(used / total * 100)
		},
		"dict": func(pairs ...interface{}) map[string]interface{} {
			m := make(map[string]interface{})
			for i := 0; i+1 < len(pairs); i += 2 {
				m[fmt.Sprint(pairs[i])] = pairs[i+1]
			}
			return m
		},
		"list": func(items ...interface{}) []interface{} { return items },
		"jsonify": func(v interface{}) (template.JS, error) {
			b, err := json.Marshal(v)
			return template.JS(b), err
		},
	}

	// Cada página se parsea junto con layout.html para evitar conflictos
	// entre los bloques {{define "body"}} de distintas páginas.
	pages := []string{
		"setup.html",
		"dashboard.html",
		"catalog.html",
		"product_install.html",
		"product_detail.html",
	}
	s.tmpls = make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		t := template.Must(
			template.New("").Funcs(s.funcMap).ParseFS(sub, "layout.html", page),
		)
		s.tmpls[page] = t
	}
}

func (s *Server) render(w http.ResponseWriter, name string, data interface{}) {
	t, ok := s.tmpls[name]
	if !ok {
		http.Error(w, "template no encontrado: "+name, 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Error rendering: "+err.Error(), 500)
	}
}

func (s *Server) registerRoutes() {
	// Archivos estáticos embebidos
	staticFS, _ := fs.Sub(s.webFS, "web/static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Páginas
	s.mux.HandleFunc("GET /", s.handleRoot)
	s.mux.HandleFunc("GET /setup", s.handleSetup)
	s.mux.HandleFunc("GET /dashboard", s.handleDashboard)
	s.mux.HandleFunc("GET /products/install", s.handleProductCatalog)
	s.mux.HandleFunc("GET /products/install/{id}", s.handleProductInstallPage)
	s.mux.HandleFunc("GET /products/{id}", s.handleProductDetail)

	// API — sistema
	s.mux.HandleFunc("POST /api/system/check", s.handleSystemCheck)
	s.mux.HandleFunc("GET /api/system/stats", s.handleSystemStats)

	// API — Docker
	s.mux.HandleFunc("POST /api/docker/check", s.handleDockerCheck)
	s.mux.HandleFunc("GET /api/docker/install", s.handleDockerInstall)

	// API — setup
	s.mux.HandleFunc("POST /api/setup/complete", s.handleSetupComplete)

	// API — productos
	s.mux.HandleFunc("GET /api/catalog", s.handleCatalog)
	s.mux.HandleFunc("POST /api/license/validate", s.handleLicenseValidate)
	s.mux.HandleFunc("POST /api/products/{id}/start", s.handleProductStart)
	s.mux.HandleFunc("POST /api/products/{id}/stop", s.handleProductStop)
	s.mux.HandleFunc("DELETE /api/products/{id}", s.handleProductRemove)

	// API — SSL
	s.mux.HandleFunc("POST /api/products/{id}/ssl/check", s.handleSSLCheck)
	s.mux.HandleFunc("GET /api/products/{id}/ssl/enable", s.handleSSLEnable)
	s.mux.HandleFunc("POST /api/products/{id}/ssl/upload", s.handleSSLUpload)

	// API — actualizador
	s.mux.HandleFunc("GET /api/updater/check", s.handleUpdaterCheck)
	s.mux.HandleFunc("POST /api/updater/apply", s.handleUpdaterApply)

	// WebSocket — deploy en vivo
	s.mux.HandleFunc("/ws/deploy/{id}", s.handleDeployWS)

	// WebSocket — logs de producto
	s.mux.HandleFunc("/ws/logs/{id}", s.handleLogsWS)

	// WebSocket — actualización de producto
	s.mux.HandleFunc("/ws/update/{id}", s.handleProductUpdate)
}
