package setup

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/term"
)

// Facts is everything detect() learns about the host before the wizard asks
// a single question. All probes are read-only.
type Facts struct {
	OSID        string // "ubuntu", "debian", ...
	OSVersion   string
	Arch        string
	IsRoot      bool
	IsTTY       bool
	HasApt      bool
	HasSystemd  bool
	HasSox      bool
	PrimaryIP   string
	ExistingEnv bool // ./.env present
	OldCreds    bool // INSTALL_CREDENTIALS.txt → old install.sh deployment
	UnitsExist  bool // emergency-callback-web.service installed

	PG PGFacts
}

type PGFacts struct {
	PeerAuthOK bool   // `sudo -u postgres psql` works (local socket, peer)
	TCPOpen    bool   // something listens on 127.0.0.1:5432
	Version    string // from SELECT version() via peer auth, if available
}

// Detect gathers Facts. Never modifies the system.
func Detect() Facts {
	f := Facts{
		IsRoot: os.Geteuid() == 0,
		IsTTY:  term.IsTerminal(int(os.Stdin.Fd())),
		Arch:   runtimeArch(),
	}

	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if v, ok := strings.CutPrefix(line, "ID="); ok {
				f.OSID = strings.Trim(v, `"`)
			}
			if v, ok := strings.CutPrefix(line, "VERSION_ID="); ok {
				f.OSVersion = strings.Trim(v, `"`)
			}
		}
	}

	f.HasApt = commandExists("apt-get")
	f.HasSox = commandExists("sox")
	if st, err := os.Stat("/run/systemd/system"); err == nil && st.IsDir() {
		f.HasSystemd = true
	}
	if _, err := os.Stat(".env"); err == nil {
		f.ExistingEnv = true
	}
	if _, err := os.Stat("INSTALL_CREDENTIALS.txt"); err == nil {
		f.OldCreds = true
	}
	if _, err := os.Stat("/etc/systemd/system/emergency-callback-web.service"); err == nil {
		f.UnitsExist = true
	}

	f.PrimaryIP = primaryIP()
	f.PG = detectPG()
	return f
}

func runtimeArch() string {
	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// primaryIP finds the host's outbound IP without sending packets.
func primaryIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

func detectPG() PGFacts {
	var pg PGFacts

	c, err := net.DialTimeout("tcp", "127.0.0.1:5432", 2*time.Second)
	if err == nil {
		pg.TCPOpen = true
		c.Close()
	}

	// Peer auth via the postgres OS user: root drops uid directly (sudo
	// shart emas — konteynerlarda sudo bo'lmasligi mumkin).
	if out, err := runPsql(context.Background(), "-tAc", "SELECT version()"); err == nil {
		pg.PeerAuthOK = true
		pg.Version = out
	}
	return pg
}

// PortBusy reports whether TCP port is already bound on any interface.
// tcp4 va tcp6 alohida tekshiriladi: faqat IPv4'da (0.0.0.0) o'tirgan servis
// bilan Go'ning dual-stack "::" bind'i to'qnashmasligi mumkin (lab'da
// aniqlangan false-negative).
func PortBusy(port string) bool {
	for _, network := range []string{"tcp4", "tcp6"} {
		l, err := net.Listen(network, ":"+port)
		if err != nil {
			return true
		}
		l.Close()
	}
	return false
}

// NextFreePort returns the first free port at or after start (bounded).
func NextFreePort(start int) int {
	for p := start; p < start+100 && p <= 65535; p++ {
		if !PortBusy(fmt.Sprint(p)) {
			return p
		}
	}
	return start
}

// VerifyDatabaseURL connects and checks the CREATE privilege migrations need.
func VerifyDatabaseURL(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	var ok bool
	if err := conn.QueryRow(ctx, "SELECT has_schema_privilege(current_user, 'public', 'CREATE')").Scan(&ok); err != nil {
		return fmt.Errorf("privilege probe: %w", err)
	}
	if !ok {
		return fmt.Errorf("user has no CREATE privilege on schema public (PG15+: GRANT ALL ON SCHEMA public TO <user>)")
	}
	return nil
}
