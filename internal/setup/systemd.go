package setup

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const unitTemplate = `[Unit]
Description=Emergency Callback (%s)
After=network.target postgresql.service

[Service]
Type=simple
User=%s
WorkingDirectory=%s
ExecStart=%s/emergency-callback %s
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
`

// InstallUnits writes, enables and (re)starts both systemd services.
func InstallUnits(appDir, serviceUser string) error {
	for _, mode := range []string{"web", "worker"} {
		unit := fmt.Sprintf(unitTemplate, mode, serviceUser, appDir, appDir, mode)
		path := fmt.Sprintf("/etc/systemd/system/emergency-callback-%s.service", mode)
		if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	steps := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "emergency-callback-web", "emergency-callback-worker"},
		{"systemctl", "restart", "emergency-callback-web", "emergency-callback-worker"},
	}
	for _, s := range steps {
		if out, err := exec.Command(s[0], s[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s: %w", strings.Join(s, " "), strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}

// UnitActive reports systemd's view of one service.
func UnitActive(mode string) bool {
	err := exec.Command("systemctl", "is-active", "--quiet", "emergency-callback-"+mode).Run()
	return err == nil
}

// StopUnits stops both services if systemd is present (ignore errors — they
// may not exist yet).
func StopUnits() {
	_ = exec.Command("systemctl", "stop", "emergency-callback-web", "emergency-callback-worker").Run()
}

// WriteRunScripts is the no-systemd fallback: helper scripts + instructions.
func WriteRunScripts(appDir string) error {
	for _, mode := range []string{"web", "worker"} {
		script := fmt.Sprintf("#!/bin/sh\n# systemd yo'q — qo'lda ishga tushirish / manual start\ncd %s && exec ./emergency-callback %s\n", appDir, mode)
		path := fmt.Sprintf("%s/run-%s.sh", appDir, mode)
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			return err
		}
	}
	return nil
}
