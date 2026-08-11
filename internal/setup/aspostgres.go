package setup

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
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

// DumpDatabase writes a gzip'ed pg_dump of the database to outPath (0600).
// peer=true: pg_dump postgres uid bilan (lokal klaster); aks holda pg_dump
// to'g'ridan-to'g'ri DATABASE_URL bilan chaqiriladi.
func DumpDatabase(ctx context.Context, peer bool, dbNameOrURL, outPath string) error {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return fmt.Errorf("pg_dump topilmadi")
	}
	var cmd *exec.Cmd
	if peer && os.Geteuid() == 0 {
		u, err := user.Lookup("postgres")
		if err != nil {
			return fmt.Errorf("postgres OS user topilmadi: %w", err)
		}
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		cmd = exec.CommandContext(ctx, "pg_dump", dbNameOrURL)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
		}
		cmd.Env = append(os.Environ(), "HOME="+u.HomeDir)
		cmd.Dir = "/tmp"
	} else {
		cmd = exec.CommandContext(ctx, "pg_dump", dbNameOrURL)
	}

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := io.Copy(gz, stdout); err != nil {
		_ = cmd.Wait()
		return err
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("pg_dump: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return gz.Close()
}
