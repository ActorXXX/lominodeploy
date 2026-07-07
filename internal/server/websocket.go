package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lominodev/lominodeploy/internal/config"
	"github.com/lominodev/lominodeploy/internal/products"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsMsg struct {
	Type    string `json:"type"`
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`
	Step    int    `json:"step,omitempty"`
	Total   int    `json:"total,omitempty"`
	Label   string `json:"label,omitempty"`
	Success bool   `json:"success,omitempty"`
}

// handleDeployWS gestiona el WebSocket de despliegue de un producto.
func (s *Server) handleDeployWS(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	entry := products.FindInCatalog(productID)
	if entry == nil {
		http.NotFound(w, r)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	send := func(msg wsMsg) {
		conn.WriteJSON(msg)
	}

	// Leer configuración del deploy desde el primer mensaje
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}

	var req products.InstallRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		send(wsMsg{Type: "error", Message: "Request inválido"})
		return
	}
	req.ProductID = productID

	// Crear estructura de archivos
	send(wsMsg{Type: "log", Level: "info", Message: "Creando estructura de directorios..."})
	if err := products.CreateFileStructure(req.BasePath); err != nil {
		send(wsMsg{Type: "error", Message: "No se pudo crear la estructura: " + err.Error()})
		return
	}
	send(wsMsg{Type: "log", Level: "success", Message: "✓ Directorios creados"})

	// Validar licencia en LominoLLS
	send(wsMsg{Type: "log", Level: "info", Message: "Validando licencia con LominoLLS..."})
	tokenResp, err := products.ValidateLicense(
		req.LicenseKey, productID,
		req.Params["SERVER_IP"], req.Params["SERVER_HOSTNAME"],
	)
	if err != nil {
		send(wsMsg{Type: "error", Message: err.Error()})
		return
	}
	if !tokenResp.Valid {
		send(wsMsg{Type: "error", Message: tokenResp.Message})
		return
	}
	send(wsMsg{Type: "log", Level: "success",
		Message: fmt.Sprintf("✓ Licencia válida — Cliente: %s", tokenResp.ClientName)})

	// Escribir .env
	send(wsMsg{Type: "log", Level: "info", Message: "Generando archivo .env..."})
	if err := products.WriteEnv(req.BasePath, entry, req.Params); err != nil {
		send(wsMsg{Type: "error", Message: err.Error()})
		return
	}
	send(wsMsg{Type: "log", Level: "success", Message: "✓ .env generado con permisos 600"})

	// Escribir docker-compose.yml
	if err := products.WriteCompose(req.BasePath, entry, tokenResp.Image, tokenResp.Tag, req.Params); err != nil {
		send(wsMsg{Type: "error", Message: err.Error()})
		return
	}
	send(wsMsg{Type: "log", Level: "success", Message: "✓ docker-compose.yml generado"})

	// Ejecutar deploy
	logFn := func(line, level string) {
		ts := time.Now().Format("15:04:05")
		send(wsMsg{Type: "log", Level: level, Message: fmt.Sprintf("[%s] %s", ts, line)})
	}
	progressFn := func(step, total int, label string) {
		send(wsMsg{Type: "progress", Step: step, Total: total, Label: label})
	}

	if err := products.Deploy(req.BasePath, tokenResp.GHCRToken, entry, logFn, progressFn); err != nil {
		send(wsMsg{Type: "done", Success: false, Message: err.Error()})
		return
	}

	// Registrar en configuración
	products.RegisterInstalled(s.cfg, &req, entry, tokenResp.Image, tokenResp.Tag)

	send(wsMsg{Type: "done", Success: true,
		Message: fmt.Sprintf("%s instalado correctamente", entry.Name)})
}

// handleLogsWS transmite los logs en vivo de un producto instalado.
func (s *Server) handleLogsWS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p := s.cfg.FindProduct(id)
	if p == nil {
		http.NotFound(w, r)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	composePath := filepath.Join(p.BasePath, "docker-compose.yml")
	cmd := exec.Command("docker", "compose", "-f", composePath,
		"logs", "--follow", "--tail", "50", "--no-color")

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		conn.WriteJSON(wsMsg{Type: "error", Message: "No se pudo iniciar el streaming de logs"})
		return
	}

	var wg sync.WaitGroup
	streamToWS := func(reader interface{ Read([]byte) (int, error) }) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				conn.WriteJSON(wsMsg{Type: "log", Level: "info", Message: string(buf[:n])})
			}
			if err != nil {
				break
			}
		}
	}

	wg.Add(2)
	go streamToWS(stdout)
	go streamToWS(stderr)

	// Cerrar cuando el cliente desconecta
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cmd.Process.Kill()
				return
			}
		}
	}()

	wg.Wait()
	cmd.Wait()
}

// handleProductUpdate actualiza un producto via WebSocket.
func (s *Server) handleProductUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p := s.cfg.FindProduct(id)
	if p == nil {
		http.NotFound(w, r)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	logFn := func(line, level string) {
		conn.WriteJSON(wsMsg{Type: "log", Level: level, Message: line})
	}

	if err := products.Update(p.BasePath, logFn); err != nil {
		conn.WriteJSON(wsMsg{Type: "done", Success: false, Message: err.Error()})
		return
	}

	conn.WriteJSON(wsMsg{Type: "done", Success: true})
}

var _ = config.Config{} // evitar unused import
