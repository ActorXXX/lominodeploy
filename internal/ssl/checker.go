package ssl

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

type PrerequisiteResult struct {
	DNSResolves  bool   `json:"dns_resolves"`
	DNSTarget    string `json:"dns_target"`
	ServerIP     string `json:"server_ip"`
	Port80OK     bool   `json:"port_80_ok"`
	AllOK        bool   `json:"all_ok"`
	DNSDetail    string `json:"dns_detail"`
	Port80Detail string `json:"port_80_detail"`
}

// CheckPrerequisites verifica si el dominio está correctamente configurado
// para obtener un certificado Let's Encrypt.
func CheckPrerequisites(domain string) PrerequisiteResult {
	result := PrerequisiteResult{}

	// Obtener IP pública del servidor
	serverIP := getPublicIP()
	result.ServerIP = serverIP

	// Verificar que el dominio resuelve a esta IP
	addrs, err := net.LookupHost(domain)
	if err != nil || len(addrs) == 0 {
		result.DNSDetail = fmt.Sprintf("El dominio %s no resuelve. Verifica el registro DNS.", domain)
	} else {
		result.DNSTarget = addrs[0]
		if serverIP != "" && addrs[0] == serverIP {
			result.DNSResolves = true
		} else {
			result.DNSDetail = fmt.Sprintf(
				"El dominio resuelve a %s pero la IP del servidor es %s",
				addrs[0], serverIP,
			)
		}
	}

	// Verificar acceso al puerto 80 desde internet (via LominoLLS)
	result.Port80OK, result.Port80Detail = checkPort80External(domain)

	result.AllOK = result.DNSResolves && result.Port80OK
	return result
}

// checkPort80External intenta hacer un HTTP GET al dominio para verificar
// que el puerto 80 es accesible desde internet.
func checkPort80External(domain string) (bool, string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get("http://" + domain + "/.well-known/lominodeploy-check")
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return false, "Puerto 80 no accesible desde internet. Verifica el firewall y el NAT."
		}
		// Si la conexión llegó pero devolvió cualquier HTTP response, el puerto está accesible
		if strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "reset") {
			return true, ""
		}
		return false, fmt.Sprintf("No se pudo verificar el puerto 80: %s", err.Error())
	}
	defer resp.Body.Close()
	return true, ""
}

func getPublicIP() string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://ifconfig.me")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	return strings.TrimSpace(string(buf[:n]))
}
