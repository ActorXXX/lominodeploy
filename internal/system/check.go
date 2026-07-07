package system

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type CheckResult struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Status string `json:"status"` // "ok", "warn", "error"
	Detail string `json:"detail,omitempty"`
}

type SystemInfo struct {
	Overall  string        `json:"overall"`
	Checks   []CheckResult `json:"checks"`
	OS       string        `json:"os"`
	Arch     string        `json:"arch"`
	RAMTotal float64       `json:"ram_total_gb"`
	DiskFree float64       `json:"disk_free_gb"`
}

func RunChecks(minRAMGB, minDiskGB float64, requiredPorts []int) SystemInfo {
	var checks []CheckResult

	// OS
	osName := detectOS()
	checks = append(checks, CheckResult{
		ID:    "os",
		Label: "Sistema operativo",
		Value: osName,
		Status: func() string {
			if runtime.GOOS == "linux" {
				return "ok"
			}
			return "warn"
		}(),
	})

	// RAM
	ramGB := getRamGB()
	ramStatus := "ok"
	if ramGB < minRAMGB {
		ramStatus = "error"
	} else if ramGB < minRAMGB*1.2 {
		ramStatus = "warn"
	}
	checks = append(checks, CheckResult{
		ID:     "ram",
		Label:  fmt.Sprintf("RAM disponible (mínimo %.0f GB)", minRAMGB),
		Value:  fmt.Sprintf("%.1f GB", ramGB),
		Status: ramStatus,
		Detail: func() string {
			if ramStatus == "warn" {
				return "La RAM es justa, el sistema podría ir lento bajo carga"
			}
			if ramStatus == "error" {
				return fmt.Sprintf("Se requieren al menos %.0f GB de RAM", minRAMGB)
			}
			return ""
		}(),
	})

	// Disk
	diskGB := getDiskFreeGB("/opt")
	diskStatus := "ok"
	if diskGB < minDiskGB {
		diskStatus = "error"
	}
	checks = append(checks, CheckResult{
		ID:     "disk",
		Label:  fmt.Sprintf("Espacio en disco en /opt (mínimo %.0f GB)", minDiskGB),
		Value:  fmt.Sprintf("%.1f GB libres", diskGB),
		Status: diskStatus,
		Detail: func() string {
			if diskStatus == "error" {
				return fmt.Sprintf("Se requieren al menos %.0f GB libres en /opt", minDiskGB)
			}
			return ""
		}(),
	})

	// Ports
	usedPorts := checkPorts(requiredPorts)
	portsStatus := "ok"
	portsValue := "Todos disponibles"
	if len(usedPorts) > 0 {
		portsStatus = "warn"
		strs := make([]string, len(usedPorts))
		for i, p := range usedPorts {
			strs[i] = strconv.Itoa(p)
		}
		portsValue = "En uso: " + strings.Join(strs, ", ")
	}
	portLabels := make([]string, len(requiredPorts))
	for i, p := range requiredPorts {
		portLabels[i] = strconv.Itoa(p)
	}
	checks = append(checks, CheckResult{
		ID:     "ports",
		Label:  "Puertos requeridos: " + strings.Join(portLabels, ", "),
		Value:  portsValue,
		Status: portsStatus,
		Detail: func() string {
			if len(usedPorts) > 0 {
				return "Detén los procesos que usan esos puertos antes de continuar"
			}
			return ""
		}(),
	})

	// Connectivity
	ghcrOK := checkConnectivity("ghcr.io", 443)
	checks = append(checks, CheckResult{
		ID:     "ghcr",
		Label:  "Conectividad a ghcr.io",
		Value:  map[bool]string{true: "Accesible", false: "Sin acceso"}[ghcrOK],
		Status: map[bool]string{true: "ok", false: "error"}[ghcrOK],
		Detail: func() string {
			if !ghcrOK {
				return "Necesario para descargar imágenes Docker de los productos"
			}
			return ""
		}(),
	})

	llsOK := checkConnectivity("license.lominodev.com", 443)
	checks = append(checks, CheckResult{
		ID:     "lls",
		Label:  "Conectividad a license.lominodev.com",
		Value:  map[bool]string{true: "Accesible", false: "Sin acceso"}[llsOK],
		Status: map[bool]string{true: "ok", false: "error"}[llsOK],
		Detail: func() string {
			if !llsOK {
				return "Necesario para validar licencias de productos"
			}
			return ""
		}(),
	})

	// Root
	isRoot := os.Geteuid() == 0
	rootStatus := "ok"
	if !isRoot {
		rootStatus = "error"
	}
	checks = append(checks, CheckResult{
		ID:     "root",
		Label:  "Permisos de administrador",
		Value:  map[bool]string{true: "Ejecutando como root", false: "Sin permisos root"}[isRoot],
		Status: rootStatus,
		Detail: func() string {
			if !isRoot {
				return "LominoDeploy necesita permisos root para gestionar Docker y el sistema"
			}
			return ""
		}(),
	})

	overall := "ok"
	for _, c := range checks {
		if c.Status == "error" {
			overall = "error"
			break
		}
		if c.Status == "warn" && overall != "error" {
			overall = "warn"
		}
	}

	return SystemInfo{
		Overall:  overall,
		Checks:   checks,
		OS:       osName,
		Arch:     runtime.GOARCH,
		RAMTotal: ramGB,
		DiskFree: diskGB,
	}
}

func GetStats() map[string]interface{} {
	return map[string]interface{}{
		"ram_total_gb": getRamGB(),
		"ram_used_gb":  getRamUsedGB(),
		"disk_free_gb": getDiskFreeGB("/opt"),
		"disk_total_gb": getDiskTotalGB("/opt"),
		"cpu_percent":  getCPUPercent(),
		"os":           detectOS(),
		"arch":         runtime.GOARCH,
	}
}

func detectOS() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return runtime.GOOS + " " + runtime.GOARCH
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return runtime.GOOS
}

func getRamGB() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseFloat(fields[1], 64)
				return kb / 1024 / 1024
			}
		}
	}
	return 0
}

func getRamUsedGB() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	var total, available float64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		kb, _ := strconv.ParseFloat(fields[1], 64)
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = kb
		case strings.HasPrefix(line, "MemAvailable:"):
			available = kb
		}
	}
	return (total - available) / 1024 / 1024
}

func getDiskFreeGB(path string) float64 {
	out, err := exec.Command("df", "-BG", "--output=avail", path).Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0
	}
	val := strings.TrimSuffix(strings.TrimSpace(lines[1]), "G")
	gb, _ := strconv.ParseFloat(val, 64)
	return gb
}

func getDiskTotalGB(path string) float64 {
	out, err := exec.Command("df", "-BG", "--output=size", path).Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0
	}
	val := strings.TrimSuffix(strings.TrimSpace(lines[1]), "G")
	gb, _ := strconv.ParseFloat(val, 64)
	return gb
}

func getCPUPercent() float64 {
	// Dos lecturas de /proc/stat separadas por 200ms
	read := func() (idle, total uint64) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0
		}
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(line, "cpu ") {
				continue
			}
			fields := strings.Fields(line)
			for i, f := range fields[1:] {
				v, _ := strconv.ParseUint(f, 10, 64)
				total += v
				if i == 3 {
					idle = v
				}
			}
			break
		}
		return
	}

	idle1, total1 := read()
	time.Sleep(200 * time.Millisecond)
	idle2, total2 := read()

	idleDiff := float64(idle2 - idle1)
	totalDiff := float64(total2 - total1)
	if totalDiff == 0 {
		return 0
	}
	return (1 - idleDiff/totalDiff) * 100
}

func checkPorts(ports []int) []int {
	var used []int
	for _, p := range ports {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err != nil {
			used = append(used, p)
			continue
		}
		ln.Close()
	}
	return used
}

func checkConnectivity(host string, port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
