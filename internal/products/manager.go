package products

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/lominodev/lominodeploy/internal/config"
)

const LLSBaseURL = "https://license.lominodev.com"

type InstallRequest struct {
	ProductID  string            `json:"product_id"`
	LicenseKey string            `json:"license_key"`
	BasePath   string            `json:"base_path"`
	Params     map[string]string `json:"params"`
}

type LLSTokenResponse struct {
	Valid      bool   `json:"valid"`
	GHCRToken  string `json:"ghcr_token"`
	Image      string `json:"image"`
	Tag        string `json:"tag"`
	ClientName string `json:"client_name"`
	Error      string `json:"error,omitempty"`
	Message    string `json:"message,omitempty"`
}

// ValidateLicense llama a LominoLLS para validar la licencia del producto
// y obtener el token de acceso a ghcr.io.
func ValidateLicense(licenseKey, productID, serverIP, serverHostname string) (*LLSTokenResponse, error) {
	payload := map[string]string{
		"license_key":     licenseKey,
		"product":         productID,
		"server_ip":       serverIP,
		"server_hostname": serverHostname,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		LLSBaseURL+"/api/install/token",
		"application/json",
		strings.NewReader(string(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar al servidor de licencias: %w", err)
	}
	defer resp.Body.Close()

	var result LLSTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("respuesta inválida del servidor de licencias")
	}
	return &result, nil
}

// CreateFileStructure crea los directorios base del producto.
func CreateFileStructure(basePath string) error {
	dirs := []struct {
		path string
		mode os.FileMode
	}{
		{basePath, 0755},
		{filepath.Join(basePath, "data/postgres"), 0700},
		{filepath.Join(basePath, "data/redis"), 0700},
		{filepath.Join(basePath, "media"), 0755},
		{filepath.Join(basePath, "static"), 0755},
		{filepath.Join(basePath, "logs"), 0755},
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d.path, d.mode); err != nil {
			return fmt.Errorf("no se pudo crear %s: %w", d.path, err)
		}
	}

	envFile := filepath.Join(basePath, ".env")
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		f, err := os.OpenFile(envFile, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		f.Close()
	}
	return nil
}

// WriteEnv genera el archivo .env desde una plantilla simple.
func WriteEnv(basePath string, entry *CatalogEntry, params map[string]string) error {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s — generado por LominoDeploy el %s\n\n",
		entry.Name, time.Now().Format("2006-01-02 15:04:05")))

	for _, p := range entry.Parameters {
		val := params[p.ID]
		if val == "" && p.Generated {
			val = GenerateSecret(24)
		}
		sb.WriteString(fmt.Sprintf("%s=%s\n", p.ID, val))
	}

	envPath := filepath.Join(basePath, ".env")
	if err := os.WriteFile(envPath, []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("no se pudo escribir .env: %w", err)
	}
	return nil
}

// WriteCompose genera el docker-compose.yml desde la plantilla del catálogo.
func WriteCompose(basePath string, entry *CatalogEntry, image, tag string, params map[string]string) error {
	tmplStr := composeTemplate(entry.ID)
	if tmplStr == "" {
		return fmt.Errorf("no hay template de compose para el producto %s", entry.ID)
	}

	tmpl, err := template.New("compose").Parse(tmplStr)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"Image":  image,
		"Tag":    tag,
		"Params": params,
		"ID":     entry.ID,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	composePath := filepath.Join(basePath, "docker-compose.yml")
	return os.WriteFile(composePath, []byte(buf.String()), 0644)
}

// GenerateSecret genera un string aleatorio seguro.
func GenerateSecret(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:n]
}

// RegisterInstalled registra un producto como instalado en la configuración.
func RegisterInstalled(cfg *config.Config, req *InstallRequest, entry *CatalogEntry, image, tag string) error {
	domain := req.Params["DOMAIN"]
	ssl := req.Params["ENABLE_SSL"] == "true"
	sslMode := "none"
	if ssl {
		sslMode = "letsencrypt"
	}

	cfg.AddProduct(config.InstalledProduct{
		ID:          entry.ID,
		Name:        entry.Name,
		Version:     tag,
		BasePath:    req.BasePath,
		Domain:      domain,
		Port:        80,
		SSLMode:     sslMode,
		SSLEnabled:  false,
		LicenseKey:  req.LicenseKey,
		Image:       image,
		InstalledAt: time.Now(),
	})

	return config.Save(cfg)
}

// composeTemplate devuelve la plantilla docker-compose para cada producto.
func composeTemplate(productID string) string {
	templates := map[string]string{
		"lominoisp": `services:
  web:
    image: {{.Image}}:{{.Tag}}
    restart: unless-stopped
    env_file: .env
    volumes:
      - ./media:/app/media
      - ./static:/app/static
      - ./logs:/app/logs
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    networks:
      - {{.ID}}

  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: {{.ID}}
      POSTGRES_USER: {{.ID}}
      POSTGRES_PASSWORD: ${ + `{DB_PASSWORD}` + `}
    volumes:
      - ./data/postgres:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U {{.ID}}"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - {{.ID}}

  redis:
    image: redis:7-alpine
    restart: unless-stopped
    volumes:
      - ./data/redis:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 3
    networks:
      - {{.ID}}

  nginx:
    image: nginx:alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./static:/app/static:ro
      - ./media:/app/media:ro
    depends_on:
      - web
    networks:
      - {{.ID}}

networks:
  {{.ID}}:
    driver: bridge
`,
	}

	if t, ok := templates[productID]; ok {
		return t
	}
	// Template genérico para productos sin template específico
	return templates["lominoisp"]
}
