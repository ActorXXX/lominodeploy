package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lominodev/lominodeploy/internal/config"
	"github.com/lominodev/lominodeploy/internal/products"
	"github.com/lominodev/lominodeploy/internal/ssl"
	"github.com/lominodev/lominodeploy/internal/system"
	"github.com/lominodev/lominodeploy/internal/updater"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func parseJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// ─── Páginas ─────────────────────────────────────────────────────────────────

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Server.SetupDone {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Server.SetupDone {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	s.render(w, "setup.html", map[string]interface{}{
		"Version": s.version,
	})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Server.SetupDone {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}

	type ProductCard struct {
		config.InstalledProduct
		Status string
		URL    string
	}

	var cards []ProductCard
	for _, p := range s.cfg.Products {
		status, _ := products.Status(p.BasePath)
		protocol := "http"
		if p.SSLEnabled {
			protocol = "https"
		}
		url := fmt.Sprintf("%s://%s", protocol, p.Domain)
		if p.Domain == "" {
			url = ""
		}
		cards = append(cards, ProductCard{
			InstalledProduct: p,
			Status:           status,
			URL:              url,
		})
	}

	stats := system.GetStats()

	s.render(w, "dashboard.html", map[string]interface{}{
		"Version":  s.version,
		"Products": cards,
		"Stats":    stats,
		"Catalog":  products.BuiltinCatalog,
	})
}

func (s *Server) handleProductCatalog(w http.ResponseWriter, r *http.Request) {
	s.render(w, "catalog.html", map[string]interface{}{
		"Version": s.version,
		"Catalog": products.BuiltinCatalog,
	})
}

func (s *Server) handleProductInstallPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	entry := products.FindInCatalog(id)
	if entry == nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "product_install.html", map[string]interface{}{
		"Version": s.version,
		"Product": entry,
	})
}

func (s *Server) handleProductDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	installed := s.cfg.FindProduct(id)
	if installed == nil {
		http.NotFound(w, r)
		return
	}

	status, _ := products.Status(installed.BasePath)
	sslPrereq := ssl.PrerequisiteResult{}
	if installed.Domain != "" {
		sslPrereq = ssl.CheckPrerequisites(installed.Domain)
	}

	s.render(w, "product_detail.html", map[string]interface{}{
		"Version":  s.version,
		"Product":  installed,
		"Status":   status,
		"SSLCheck": sslPrereq,
	})
}

// ─── API — Sistema ────────────────────────────────────────────────────────────

func (s *Server) handleSystemCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MinRAMGB      float64 `json:"min_ram_gb"`
		MinDiskGB     float64 `json:"min_disk_gb"`
		RequiredPorts []int   `json:"required_ports"`
	}
	req.MinRAMGB = 2
	req.MinDiskGB = 20
	parseJSON(r, &req)

	result := system.RunChecks(req.MinRAMGB, req.MinDiskGB, req.RequiredPorts)
	jsonOK(w, result)
}

func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, system.GetStats())
}

// ─── API — Docker ─────────────────────────────────────────────────────────────

func (s *Server) handleDockerCheck(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, system.CheckDocker())
}

func (s *Server) handleDockerInstall(w http.ResponseWriter, r *http.Request) {
	// Streaming via SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming no soportado", 500)
		return
	}

	sendEvent := func(line, level string) {
		data, _ := json.Marshal(map[string]string{"line": line, "level": level})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	err := system.InstallDocker(sendEvent)
	if err != nil {
		sendEvent("Error: "+err.Error(), "error")
	} else {
		sendEvent("__done__", "success")
	}
}

// ─── API — Setup ──────────────────────────────────────────────────────────────

func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Port int `json:"port"`
	}
	req.Port = 8888
	parseJSON(r, &req)

	s.cfg.Server.SetupDone = true
	if req.Port > 0 {
		s.cfg.Server.Port = req.Port
	}

	if err := config.Save(s.cfg); err != nil {
		jsonErr(w, 500, "No se pudo guardar la configuración: "+err.Error())
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

// ─── API — Catálogo ───────────────────────────────────────────────────────────

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, products.BuiltinCatalog)
}

// ─── API — Licencia ───────────────────────────────────────────────────────────

func (s *Server) handleLicenseValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LicenseKey string `json:"license_key"`
		ProductID  string `json:"product_id"`
		ServerIP   string `json:"server_ip"`
		Hostname   string `json:"hostname"`
	}
	if err := parseJSON(r, &req); err != nil {
		jsonErr(w, 400, "Request inválido")
		return
	}

	result, err := products.ValidateLicense(req.LicenseKey, req.ProductID, req.ServerIP, req.Hostname)
	if err != nil {
		jsonErr(w, 502, err.Error())
		return
	}
	jsonOK(w, result)
}

// ─── API — Control de productos ───────────────────────────────────────────────

func (s *Server) handleProductStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p := s.cfg.FindProduct(id)
	if p == nil {
		jsonErr(w, 404, "Producto no encontrado")
		return
	}
	if err := products.Start(p.BasePath); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "starting"})
}

func (s *Server) handleProductStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p := s.cfg.FindProduct(id)
	if p == nil {
		jsonErr(w, 404, "Producto no encontrado")
		return
	}
	if err := products.Stop(p.BasePath); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "stopping"})
}

func (s *Server) handleProductRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p := s.cfg.FindProduct(id)
	if p == nil {
		jsonErr(w, 404, "Producto no encontrado")
		return
	}
	products.Remove(p.BasePath)
	s.cfg.RemoveProduct(id)
	config.Save(s.cfg)
	jsonOK(w, map[string]bool{"ok": true})
}

// ─── API — SSL ────────────────────────────────────────────────────────────────

func (s *Server) handleSSLCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p := s.cfg.FindProduct(id)
	if p == nil {
		jsonErr(w, 404, "Producto no encontrado")
		return
	}
	result := ssl.CheckPrerequisites(p.Domain)
	jsonOK(w, result)
}

func (s *Server) handleSSLEnable(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p := s.cfg.FindProduct(id)
	if p == nil {
		jsonErr(w, 404, "Producto no encontrado")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)

	sendEvent := func(line, level string) {
		data, _ := json.Marshal(map[string]string{"line": line, "level": level})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	certsDir := filepath.Join(p.BasePath, "certs")
	_, err := ssl.EnableLetsEncrypt(p.Domain, certsDir, sendEvent)
	if err != nil {
		sendEvent("Error: "+err.Error(), "error")
		return
	}

	p.SSLEnabled = true
	p.SSLMode = string(ssl.ModeLetsEncrypt)
	config.Save(s.cfg)
	sendEvent("__done__", "success")
}

func (s *Server) handleSSLUpload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p := s.cfg.FindProduct(id)
	if p == nil {
		jsonErr(w, 404, "Producto no encontrado")
		return
	}

	r.ParseMultipartForm(10 << 20)
	certFile, _, err := r.FormFile("cert")
	if err != nil {
		jsonErr(w, 400, "Archivo de certificado requerido")
		return
	}
	defer certFile.Close()

	keyFile, _, err := r.FormFile("key")
	if err != nil {
		jsonErr(w, 400, "Archivo de clave requerido")
		return
	}
	defer keyFile.Close()

	certsDir := filepath.Join(p.BasePath, "certs")
	_, err = ssl.SaveCustomCert(certsDir, p.Domain, certFile, keyFile)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	p.SSLEnabled = true
	p.SSLMode = string(ssl.ModeCustom)
	config.Save(s.cfg)
	jsonOK(w, map[string]bool{"ok": true})
}

// ─── API — Actualizador ───────────────────────────────────────────────────────

func (s *Server) handleUpdaterCheck(w http.ResponseWriter, r *http.Request) {
	status := updater.Check(s.version)
	jsonOK(w, status)
}

func (s *Server) handleUpdaterApply(w http.ResponseWriter, r *http.Request) {
	status := updater.Check(s.version)
	if !status.UpdateAvail || status.Info == nil {
		jsonErr(w, 400, "No hay actualización disponible")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)

	sendEvent := func(line, level string) {
		data, _ := json.Marshal(map[string]string{"line": line, "level": level})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	if err := updater.Apply(status.Info, sendEvent); err != nil {
		sendEvent("Error: "+err.Error(), "error")
		return
	}
	sendEvent("__done__", "success")
}

// ─── Helpers de productos ─────────────────────────────────────────────────────

func getDockerLogs(basePath string, lines int) string {
	out, err := exec.Command("docker", "compose",
		"-f", filepath.Join(basePath, "docker-compose.yml"),
		"logs", "--tail", fmt.Sprintf("%d", lines), "--no-color",
	).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func productURL(p config.InstalledProduct) string {
	if p.Domain == "" {
		return ""
	}
	proto := "http"
	if p.SSLEnabled {
		proto = "https"
	}
	return proto + "://" + p.Domain
}

var _ = time.Now // evitar unused import
