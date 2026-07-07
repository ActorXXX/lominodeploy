package system

import (
	"bufio"
	"io"
	"strings"
)

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
