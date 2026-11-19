package main

import (
	"fmt"
	"log"
	"os"
	"time"
)

var logger = log.New(os.Stderr, "", log.Ldate|log.Ltime)
var colorLogs = func() bool {
	info, err := os.Stderr.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0 && os.Getenv("TERM") != "dumb" && os.Getenv("NO_COLOR") == ""
}()

func label(text, color string) string {
	if colorLogs {
		return "\x1b[" + color + "m" + text + "\x1b[0m"
	}
	return text
}

func logRequest(out *responseLog, complete bool, method, ip, host, path string, elapsed time.Duration) {
	result, color := "OK", "32"
	switch {
	case !complete || out.status < 200 || out.status >= 500:
		result, color = "ERROR", "31"
	case out.status >= 400:
		result, color = "FAIL", "33"
	case out.status == 204 && host == hosts["sessionserver"] && path == "/session/minecraft/hasJoined":
		result, color = "NO SESSION", "33"
	}
	detail := ""
	if out.err != nil {
		detail = fmt.Sprintf("  %q", out.err.Error())
	}
	logger.Printf("%s  %7s  %s  %s %s%s%s",
		label(fmt.Sprintf("%3d %-10s", out.status, result), color),
		elapsed.Round(time.Millisecond), ip, method, host, path, detail)
}
