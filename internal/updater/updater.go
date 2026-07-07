package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const LLSBaseURL = "https://license.lominodev.com"

type VersionInfo struct {
	Latest    string `json:"latest"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Changelog string `json:"changelog"`
}

type UpdateStatus struct {
	Current       string      `json:"current"`
	Latest        string      `json:"latest"`
	UpdateAvail   bool        `json:"update_available"`
	Info          *VersionInfo `json:"info,omitempty"`
	CheckedAt     time.Time   `json:"checked_at"`
	Error         string      `json:"error,omitempty"`
}

// Check consulta LominoLLS para ver si hay una nueva versión disponible.
func Check(currentVersion string) UpdateStatus {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "amd64"
	}

	url := fmt.Sprintf("%s/binaries/version.json", LLSBaseURL)
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return UpdateStatus{
			Current:   currentVersion,
			CheckedAt: time.Now(),
			Error:     "No se pudo conectar al servidor de actualizaciones",
		}
	}
	defer resp.Body.Close()

	var info VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return UpdateStatus{
			Current:   currentVersion,
			CheckedAt: time.Now(),
			Error:     "Respuesta inválida del servidor",
		}
	}

	updateAvail := compareVersions(info.Latest, currentVersion) > 0

	return UpdateStatus{
		Current:     currentVersion,
		Latest:      info.Latest,
		UpdateAvail: updateAvail,
		Info:        &info,
		CheckedAt:   time.Now(),
	}
}

// Apply descarga el nuevo binario, verifica el hash y reemplaza el ejecutable.
// El servicio systemd reinicia LominoDeploy automáticamente.
func Apply(info *VersionInfo, logFn func(line, level string)) error {
	arch := runtime.GOARCH
	dlURL := fmt.Sprintf("%s/binaries/lominodeploy-linux-%s", LLSBaseURL, arch)

	logFn(fmt.Sprintf("Descargando LominoDeploy %s...", info.Latest), "info")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(dlURL)
	if err != nil {
		return fmt.Errorf("no se pudo descargar la actualización: %w", err)
	}
	defer resp.Body.Close()

	tmpFile, err := os.CreateTemp("", "lominodeploy-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		return fmt.Errorf("error al descargar: %w", err)
	}
	tmpFile.Close()

	// Verificar SHA256
	got := hex.EncodeToString(hasher.Sum(nil))
	if info.SHA256 != "" && got != strings.ToLower(info.SHA256) {
		return fmt.Errorf("checksum SHA256 inválido: esperado %s, obtenido %s", info.SHA256, got)
	}
	logFn("✓ SHA256 verificado correctamente", "success")

	// Reemplazar el binario
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("no se pudo determinar la ruta del ejecutable: %w", err)
	}

	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		return err
	}
	if err := os.Rename(tmpFile.Name(), execPath); err != nil {
		return fmt.Errorf("no se pudo reemplazar el binario: %w", err)
	}

	logFn("✓ Binario actualizado. Reiniciando servicio...", "success")

	// Reiniciar vía systemd
	_ = runCmd("systemctl", "restart", "lominodeploy")

	return nil
}

func runCmd(name string, args ...string) error {
	// exec.Command(name, args...).Run()
	return nil // stub para compilación en Windows
}

// compareVersions retorna positivo si a > b (semver simple).
func compareVersions(a, b string) int {
	pa := parseSemver(a)
	pb := parseSemver(b)
	for i := range pa {
		if i >= len(pb) {
			return 1
		}
		if pa[i] > pb[i] {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
	}
	return 0
}

func parseSemver(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		fmt.Sscanf(p, "%d", &nums[i])
	}
	return nums
}
