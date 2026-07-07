package products

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type DeployStep struct {
	Label   string
	Command string
}

// Deploy ejecuta el proceso de despliegue completo de un producto.
// logFn recibe cada línea del output en tiempo real.
// progressFn recibe el avance (step actual, total steps, label).
func Deploy(
	basePath string,
	ghcrToken string,
	entry *CatalogEntry,
	logFn func(line, level string),
	progressFn func(step, total int, label string),
) error {
	steps := buildSteps(entry, ghcrToken)
	total := len(steps)

	for i, step := range steps {
		progressFn(i+1, total, step.Label)
		logFn(fmt.Sprintf("▶ %s", step.Label), "info")

		if step.Command == "__login__" {
			if err := dockerLogin(ghcrToken, logFn); err != nil {
				return err
			}
			continue
		}

		cmd := exec.Command("sh", "-c", step.Command)
		cmd.Dir = basePath

		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("no se pudo iniciar '%s': %w", step.Command, err)
		}

		go streamLines(stdout, logFn, "info")
		go streamLines(stderr, logFn, "warn")

		if err := cmd.Wait(); err != nil {
			return fmt.Errorf("falló '%s': %w", step.Command, err)
		}

		ts := time.Now().Format("15:04:05")
		logFn(fmt.Sprintf("[%s] ✓ %s completado", ts, step.Label), "success")
	}

	return nil
}

func buildSteps(entry *CatalogEntry, ghcrToken string) []DeployStep {
	var steps []DeployStep

	if ghcrToken != "" {
		steps = append(steps, DeployStep{
			Label:   "Autenticando en ghcr.io",
			Command: "__login__",
		})
	}

	for _, cmd := range entry.DeployCommands {
		label := cmd
		label = strings.TrimPrefix(label, "docker compose ")
		label = strings.ReplaceAll(label, " --noinput", "")
		label = strings.ReplaceAll(label, " -T", "")

		steps = append(steps, DeployStep{
			Label:   label,
			Command: cmd,
		})
	}

	steps = append(steps, DeployStep{
		Label:   "Verificando servicios",
		Command: "docker compose ps",
	})

	return steps
}

func dockerLogin(token string, logFn func(line, level string)) error {
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("echo '%s' | docker login ghcr.io -u lominodev-installer --password-stdin", token),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logFn("Error al autenticarse en ghcr.io: "+string(out), "error")
		return fmt.Errorf("docker login falló: %w", err)
	}
	logFn("✓ Autenticado en ghcr.io", "success")
	return nil
}

// Stop detiene un producto.
func Stop(basePath string) error {
	return runCompose(basePath, "docker compose stop")
}

// Start inicia un producto detenido.
func Start(basePath string) error {
	return runCompose(basePath, "docker compose up -d")
}

// Update actualiza las imágenes y reinicia.
func Update(basePath string, logFn func(line, level string)) error {
	for _, cmd := range []string{"docker compose pull", "docker compose up -d"} {
		if err := runComposeWithLog(basePath, cmd, logFn); err != nil {
			return err
		}
	}
	return nil
}

// Remove detiene y elimina un producto.
func Remove(basePath string) error {
	return runCompose(basePath, "docker compose down -v")
}

// Status retorna el estado de los contenedores de un producto.
func Status(basePath string) (string, error) {
	out, err := exec.Command("docker", "compose", "-f",
		basePath+"/docker-compose.yml", "ps", "--format", "json").Output()
	if err != nil {
		return "unknown", err
	}
	output := strings.TrimSpace(string(out))
	if output == "" || output == "[]" {
		return "stopped", nil
	}
	if strings.Contains(output, `"running"`) || strings.Contains(output, `"Up"`) {
		return "running", nil
	}
	return "stopped", nil
}

func runCompose(basePath, cmd string) error {
	c := exec.Command("sh", "-c", cmd)
	c.Dir = basePath
	return c.Run()
}

func runComposeWithLog(basePath, cmd string, logFn func(line, level string)) error {
	c := exec.Command("sh", "-c", cmd)
	c.Dir = basePath
	stdout, _ := c.StdoutPipe()
	stderr, _ := c.StderrPipe()
	if err := c.Start(); err != nil {
		return err
	}
	go streamLines(stdout, logFn, "info")
	go streamLines(stderr, logFn, "warn")
	return c.Wait()
}

func streamLines(r io.Reader, logFn func(line, level string), defaultLevel string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		level := defaultLevel
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") {
			level = "error"
		} else if strings.Contains(lower, "warn") {
			level = "warn"
		}
		logFn(line, level)
	}
}
