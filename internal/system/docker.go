package system

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type DockerStatus struct {
	Installed        bool   `json:"installed"`
	Version          string `json:"version"`
	VersionOK        bool   `json:"version_ok"`
	DaemonRunning    bool   `json:"daemon_running"`
	ComposeAvailable bool   `json:"compose_available"`
	ComposeVersion   string `json:"compose_version"`
	Overall          string `json:"overall"` // "ok", "warn", "not_installed"
}

func CheckDocker() DockerStatus {
	path, err := exec.LookPath("docker")
	if err != nil || path == "" {
		return DockerStatus{Installed: false, Overall: "not_installed"}
	}

	out, err := exec.Command("docker", "--version").Output()
	version := strings.TrimSpace(string(out))
	if err != nil {
		return DockerStatus{Installed: true, Overall: "error"}
	}

	major, minor := parseDockerVersion(version)
	versionOK := major > 24 || (major == 24)

	daemonOut := exec.Command("docker", "info")
	daemonRunning := daemonOut.Run() == nil

	composeOut, composeErr := exec.Command("docker", "compose", "version").Output()
	composeAvailable := composeErr == nil
	composeVersion := strings.TrimSpace(string(composeOut))

	overall := "ok"
	if !daemonRunning || !composeAvailable {
		overall = "error"
	} else if !versionOK {
		overall = "warn"
	}

	return DockerStatus{
		Installed:        true,
		Version:          version,
		VersionOK:        versionOK,
		DaemonRunning:    daemonRunning,
		ComposeAvailable: composeAvailable,
		ComposeVersion:   composeVersion,
		Overall:          overall,
	}
}

// InstallDocker instala Docker Engine vía el script oficial.
// logFn recibe cada línea de output en tiempo real.
func InstallDocker(logFn func(line, level string)) error {
	logFn("Descargando script de instalación de Docker...", "info")

	script := `#!/bin/sh
set -e
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
`
	cmd := exec.Command("sh", "-c", script)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("no se pudo iniciar la instalación: %w", err)
	}

	go streamLines(stdout, logFn, "info")
	go streamLines(stderr, logFn, "warn")

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("la instalación de Docker falló: %w", err)
	}

	logFn("✓ Docker instalado correctamente", "success")
	return nil
}

func parseDockerVersion(s string) (major, minor int) {
	re := regexp.MustCompile(`(\d+)\.(\d+)`)
	m := re.FindStringSubmatch(s)
	if len(m) < 3 {
		return 0, 0
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	return
}
