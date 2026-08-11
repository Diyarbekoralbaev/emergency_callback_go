package setup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// runPsql executes psql as the postgres OS user (peer auth over the local
// socket). Running as root we drop to the postgres uid directly via
// SysProcAttr.Credential — works in containers where sudo isn't installed.
// Non-root falls back to `sudo -n`.
func runPsql(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	base := append([]string{"-v", "ON_ERROR_STOP=1"}, args...)

	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		u, err := user.Lookup("postgres")
		if err != nil {
			return "", fmt.Errorf("postgres OS user topilmadi: %w", err)
		}
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		cmd = exec.CommandContext(ctx, "psql", base...)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
		}
		// psql peer auth socket katalogiga HOME/dir kerak emas, lekin ba'zi
		// distrolarda ~/.psqlrc o'qishga urinadi — xatoni oldini olamiz.
		cmd.Env = append(os.Environ(), "HOME="+u.HomeDir, "PSQLRC=/dev/null")
		cmd.Dir = "/tmp"
	} else {
		cmd = exec.CommandContext(ctx, "sudo", append([]string{"-n", "-u", "postgres", "psql"}, base...)...)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("psql: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}
