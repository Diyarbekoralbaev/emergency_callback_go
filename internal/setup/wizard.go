// Package setup is the interactive install wizard (`emergency-callback
// setup`) and health checker (`doctor`). Design: detect first (read-only),
// ask with options wherever reality deviates, show a summary, then apply.
// Re-running is always safe: existing .env values become defaults, secrets
// are reused, provisioning is idempotent.
package setup

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/auth"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/config"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	NonInteractive bool
	Version        string
}

// State collects every decision before anything is applied.
type State struct {
	Facts Facts
	Env   *EnvFile

	AppDir      string
	ServiceUser string

	// Postgres
	PGMode      string // peer | password | remote | install
	DBName      string
	DBUser      string
	DBPassword  string
	DBHost      string
	DBPort      string
	DBSuperURL  string // password mode only
	DatabaseURL string
	NewDBPass   bool

	HTTPPort   string
	SiteDomain string

	Backend string // ami | ari

	AdminUser    string
	AdminPass    string
	AdminCreated bool

	InstallPG  bool
	InstallSox bool

	// PBX audio push over SSH
	SSHPush bool
	SSH     SSHConfig
	SoundsDir string

	// ARI probe natijalari (backend=ari, probe o'tganda to'ldiriladi) —
	// audio qadamida keraksiz savollarni o'tkazib yuborish uchun.
	ARIProbeOK     bool
	ARIMediaCache  bool // res_http_media_cache yuklangan
	ARISoundsExist bool // ambulance-* promptlar PBX sounds indeksida bor

	newSecrets []string // names of freshly generated secrets, for the creds file
	warnings   []string
}

func info(format string, a ...any)  { fmt.Printf("==> "+format+"\n", a...) }
func ok(format string, a ...any)    { fmt.Printf(" ✓ "+format+"\n", a...) }
func warnf(format string, a ...any) { fmt.Printf(" ! "+format+"\n", a...) }

func genSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b) // 43 chars, no padding issues
}

// Run drives the whole wizard.
func Run(ctx context.Context, opts Options) error {
	p := NewPrompter(opts.NonInteractive)
	st := &State{Facts: Detect()}

	fmt.Printf("Emergency Callback setup (%s)\n\n", opts.Version)

	// ---- 0. Preflight -------------------------------------------------
	if !st.Facts.IsRoot {
		return fmt.Errorf("root kerak: sudo ./emergency-callback setup")
	}
	if !st.Facts.IsTTY && !opts.NonInteractive {
		return fmt.Errorf("terminal yo'q: --non-interactive va SETUP_* env o'zgaruvchilar bilan ishga tushiring")
	}
	info("Muhit: %s %s / %s, systemd=%v, apt=%v", st.Facts.OSID, st.Facts.OSVersion, st.Facts.Arch, st.Facts.HasSystemd, st.Facts.HasApt)
	if st.Facts.OSID != "ubuntu" && st.Facts.OSID != "debian" {
		warnf("Ubuntu/Debian emas — paket o'rnatish qadamlari qo'lda ko'rsatma sifatida beriladi")
	}

	// ---- 1. Install kind ----------------------------------------------
	if st.Facts.ExistingEnv {
		info("Mavjud .env topildi — qiymatlar saqlanadi, faqat yetishmayotganlari so'raladi")
	}
	env, err := LoadEnvFile(".env")
	if err != nil {
		return fmt.Errorf(".env o'qish: %w", err)
	}
	st.Env = env

	// ---- 2. App dir ----------------------------------------------------
	cwd, _ := os.Getwd()
	st.AppDir = p.Ask("APP_DIR", "Ilova katalogi", cwd)
	if st.AppDir != cwd {
		if err := relocateSelf(st.AppDir); err != nil {
			return err
		}
		if err := os.Chdir(st.AppDir); err != nil {
			return err
		}
		// reload .env from the new location
		if st.Env, err = LoadEnvFile(".env"); err != nil {
			return err
		}
	}
	st.ServiceUser = dirOwner(st.AppDir)
	if st.ServiceUser == "root" {
		warnf("Katalog egasi root — servislar root sifatida ishlaydi")
	}
	st.ServiceUser = p.Ask("SERVICE_USER", "Servis foydalanuvchisi", st.ServiceUser)

	// ---- 3. PostgreSQL ---------------------------------------------------
	if err := stepPostgres(ctx, p, st); err != nil {
		return err
	}

	// ---- 4. Web ---------------------------------------------------------
	stepWeb(p, st)

	// ---- 5. Secrets -------------------------------------------------------
	stepSecrets(p, st)

	// ---- 6-7. Telephony ---------------------------------------------------
	if err := stepTelephony(ctx, p, st); err != nil {
		return err
	}

	// ---- 8. Audio ----------------------------------------------------------
	stepAudio(ctx, p, st)

	// ---- 9. Eskiz -----------------------------------------------------------
	stepEskiz(ctx, p, st)

	// ---- 10. Admin user -------------------------------------------------------
	st.AdminUser = p.Ask("ADMIN_USER", "Admin username", "admin")

	// ---- Non-interactive gap check -----------------------------------------
	if len(p.Missing) > 0 {
		return fmt.Errorf("non-interactive rejimda quyidagi qiymatlar yetishmayapti:\n  %s", strings.Join(p.Missing, "\n  "))
	}

	// ---- Summary + confirm ---------------------------------------------------
	printSummary(st)
	if !p.AskYN("CONFIRM", "Davom etamizmi?", true) {
		return fmt.Errorf("bekor qilindi")
	}

	// ---- Apply -----------------------------------------------------------------
	return apply(ctx, p, st)
}

func stepPostgres(ctx context.Context, p *Prompter, st *State) error {
	// Prefill from existing DATABASE_URL.
	exURL := st.Env.Get("DATABASE_URL")
	defName, defUser, defPass, defHost, defPort := "emergency_callback", "ecb", "", "127.0.0.1", "5432"
	if exURL != "" {
		if c, err := pgx.ParseConfig(exURL); err == nil {
			defName, defUser, defPass = c.Database, c.User, c.Password
			defHost, defPort = c.Host, fmt.Sprint(c.Port)
		}
	}

	// If the existing URL still works, keep it with zero questions.
	if exURL != "" {
		if err := VerifyDatabaseURL(ctx, exURL); err == nil {
			ok("Mavjud DATABASE_URL ishlayapti — o'zgartirilmaydi")
			st.PGMode, st.DatabaseURL = "keep", exURL
			st.DBName, st.DBUser, st.DBPassword, st.DBHost, st.DBPort = defName, defUser, defPass, defHost, defPort
			return nil
		}
		warnf("Mavjud DATABASE_URL ishlamayapti — qayta sozlaymiz")
	}

	pg := st.Facts.PG
	// Re-run qulayligi: birinchi o'rnatishda SETUP_PG_MODE=install bo'lgan
	// skript ikkinchi marta ishga tushsa, PG allaqachon bor — peer'ga o'tamiz.
	if os.Getenv("SETUP_PG_MODE") == "install" && pg.PeerAuthOK {
		os.Setenv("SETUP_PG_MODE", "peer")
	}
	var choices []Choice
	switch {
	case pg.PeerAuthOK:
		ok("Lokal PostgreSQL topildi (peer auth OK): %s", strings.SplitN(pg.Version, " on ", 2)[0])
		choices = []Choice{
			{"peer", "Lokal PostgreSQL'da yaratish (tavsiya)"},
			{"remote", "Boshqa/masofaviy DB URL kiritish"},
		}
	case pg.TCPOpen:
		warnf("5432-portda PostgreSQL bor, lekin peer auth ishlamadi (parol talab qilinadi yoki docker)")
		choices = []Choice{
			{"password", "Superuser parol bilan ulanib yaratish"},
			{"remote", "Tayyor DATABASE_URL kiritish (DB allaqachon yaratilgan)"},
		}
	case st.Facts.HasApt:
		warnf("PostgreSQL topilmadi")
		choices = []Choice{
			{"install", "PostgreSQL o'rnatish (apt)"},
			{"remote", "Masofaviy DB URL kiritish"},
		}
	default:
		warnf("PostgreSQL topilmadi, apt ham yo'q")
		choices = []Choice{
			{"remote", "Masofaviy DB URL kiritish"},
		}
	}
	st.PGMode = p.AskChoice("PG_MODE", "PostgreSQL sozlash usuli:", choices)

	if st.PGMode == "remote" {
		for {
			url := p.Ask("DATABASE_URL", "To'liq DATABASE_URL (postgres://user:pass@host:port/db?sslmode=...)", exURL)
			if err := VerifyDatabaseURL(ctx, url); err != nil {
				warnf("Ulanib bo'lmadi: %v", err)
				if p.NonInteractive {
					return fmt.Errorf("DATABASE_URL yaroqsiz: %w", err)
				}
				continue
			}
			st.DatabaseURL = url
			ok("Ulanish va CREATE huquqi OK")
			return nil
		}
	}

	st.DBName = p.Ask("DB_NAME", "Baza nomi", defName)
	st.DBUser = p.Ask("DB_USER", "Baza foydalanuvchisi", defUser)
	if !ValidIdent(st.DBName) || !ValidIdent(st.DBUser) {
		return fmt.Errorf("baza nomi/foydalanuvchisi faqat harf-raqam va _ bo'lishi mumkin")
	}
	if defPass != "" {
		st.DBPassword = defPass // reuse — parol almashtirish alohida savol emas
	} else {
		st.DBPassword = genSecret()[:24]
		st.NewDBPass = true
	}

	switch st.PGMode {
	case "password":
		st.DBHost = p.Ask("DB_HOST", "PostgreSQL host", defHost)
		st.DBPort = p.Ask("DB_PORT", "PostgreSQL port", defPort)
		for {
			superPass := p.AskSecret("PG_SUPER_PASSWORD", "postgres superuser paroli", "")
			st.DBSuperURL = fmt.Sprintf("postgres://postgres:%s@%s:%s/postgres", urlEscape(superPass), st.DBHost, st.DBPort)
			conn, err := pgx.Connect(ctx, st.DBSuperURL)
			if err != nil {
				warnf("Superuser bilan ulanib bo'lmadi: %v", err)
				if p.NonInteractive {
					return fmt.Errorf("superuser ulanish: %w", err)
				}
				continue
			}
			conn.Close(ctx)
			break
		}
	case "install":
		st.InstallPG = true
		st.DBHost, st.DBPort = "127.0.0.1", "5432"
	default: // peer
		st.DBHost, st.DBPort = defHost, defPort
	}

	st.DatabaseURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		st.DBUser, urlEscape(st.DBPassword), st.DBHost, st.DBPort, st.DBName)
	return nil
}

func stepWeb(p *Prompter, st *State) {
	defPort := "8000"
	if addr := st.Env.Get("HTTP_ADDR"); addr != "" {
		if i := strings.LastIndex(addr, ":"); i >= 0 {
			defPort = addr[i+1:]
		}
	}
	envPort := defPort // .env'dagi joriy port (bo'lsa)
	for {
		st.HTTPPort = p.Ask("HTTP_PORT", "Web port", defPort)
		if !PortBusy(st.HTTPPort) {
			break
		}
		if st.Facts.UnitsExist && st.HTTPPort == envPort {
			// band port — bizning ishlab turgan eski instansiyamiz
			ok("Port %s band — mavjud emergency-callback servisi restart qilinadi", st.HTTPPort)
			break
		}
		next := NextFreePort(atoi(st.HTTPPort, 8000) + 1)
		choice := p.AskChoice("PORT_ACTION", fmt.Sprintf("Port %s band:", st.HTTPPort), []Choice{
			{"next", fmt.Sprintf("Bo'sh portni olish (%d)", next)},
			{"retry", "Boshqa port kiritish"},
			{"keep", "Baribir shu portni yozish (band qilgan servisni o'zingiz to'xtatasiz)"},
		})
		if choice == "next" {
			st.HTTPPort = fmt.Sprint(next)
			break
		}
		if choice == "keep" {
			st.warnings = append(st.warnings, fmt.Sprintf("port %s band edi — web servis ishga tushmasligi mumkin", st.HTTPPort))
			break
		}
		defPort = ""
	}
	st.SiteDomain = p.Ask("SITE_DOMAIN", "Sayt URL (SMS havolalari uchun)",
		firstNonEmpty(st.Env.Get("SITE_DOMAIN"), fmt.Sprintf("http://%s:%s", st.Facts.PrimaryIP, st.HTTPPort)))
}

func stepSecrets(p *Prompter, st *State) {
	for _, key := range []string{"SESSION_SECRET", "CSRF_KEY"} {
		if st.Env.Has(key) {
			continue // reuse — hech qachon jimgina almashtirmaymiz
		}
		st.Env.Set(key, genSecret())
		st.newSecrets = append(st.newSecrets, key)
	}
	if len(st.newSecrets) == 0 {
		if p.AskYN("REGEN_SECRETS", "Secretlarni qayta yaratish? (BARCHA sessiyalar bekor bo'ladi)", false) {
			st.Env.Set("SESSION_SECRET", genSecret())
			st.Env.Set("CSRF_KEY", genSecret())
			st.newSecrets = append(st.newSecrets, "SESSION_SECRET", "CSRF_KEY")
		}
	}
}

func stepTelephony(ctx context.Context, p *Prompter, st *State) error {
	st.Backend = p.AskChoice("TELEPHONY_BACKEND", "Telefoniya backend:", []Choice{
		{"ami", "AMI + dialplan (klassik, mavjud o'rnatishlarga mos)"},
		{"ari", "ARI (dialplan kerak emas — FreePBX'da faqat ARI user yaratiladi)"},
	})

	if st.Backend == "ari" {
		defURL := st.Env.Get("ARI_URL")
		if defURL == "" && st.Env.Get("AMI_HOST") != "" {
			defURL = "http://" + st.Env.Get("AMI_HOST") + ":8088/ari"
		}
		for {
			st.Env.Set("ARI_URL", p.Ask("ARI_URL", "ARI URL (http://<freepbx-ip>:8088/ari)", defURL))
			st.Env.Set("ARI_USERNAME", p.Ask("ARI_USERNAME", "ARI username", firstNonEmpty(st.Env.Get("ARI_USERNAME"), "ecb")))
			st.Env.Set("ARI_PASSWORD", p.AskSecret("ARI_PASSWORD", "ARI password", st.Env.Get("ARI_PASSWORD")))
			err := ProbeARI(ctx, st.Env.Get("ARI_URL"), st.Env.Get("ARI_USERNAME"), st.Env.Get("ARI_PASSWORD"))
			if err == nil {
				ok("ARI ulanish OK")
				st.ARIProbeOK = true
				st.ARIMediaCache, st.ARISoundsExist = probeARIAudio(ctx,
					st.Env.Get("ARI_URL"), st.Env.Get("ARI_USERNAME"), st.Env.Get("ARI_PASSWORD"))
				break
			}
			// Kiritilgan qiymatlarni ko'rsatamiz (parol — faqat uzunligi):
			// noto'g'ri terilgan username yoki buzilib yetib kelgan parolni
			// operator darhol ko'radi.
			warnf("ARI probe: %v [user=%q, parol=%d belgi]", err,
				st.Env.Get("ARI_USERNAME"), len(st.Env.Get("ARI_PASSWORD")))
			st.Env.Set("ARI_PASSWORD", "") // xato parolni default sifatida taklif qilmaymiz
			fmt.Println("   FreePBX'da: Settings → Asterisk REST Interface Users → Add User;")
			fmt.Println("   Advanced Settings → 'Asterisk Builtin mini-HTTP server' va 'ARI' yoqilgan bo'lsin (port 8088).")
			if p.NonInteractive {
				st.warnings = append(st.warnings, "ARI probe muvaffaqiyatsiz: "+err.Error())
				break
			}
			if !p.Pause("Sozlagach Enter bosing") {
				st.warnings = append(st.warnings, "ARI tekshirilmadi (skip)")
				break
			}
		}
		// AMI ham .env'da qolishi mumkin — tegmaymiz.
		return nil
	}

	// --- AMI path ---
	amiHost := p.Ask("AMI_HOST", "FreePBX AMI host (IP)", firstNonEmpty(st.Env.Get("AMI_HOST"), ""))
	amiPort := p.Ask("AMI_PORT", "AMI port", firstNonEmpty(st.Env.Get("AMI_PORT"), "5038"))
	st.Env.Set("AMI_HOST", amiHost)
	st.Env.Set("AMI_PORT", amiPort)
	if !st.Env.Has("AMI_CALLER_ID") {
		st.Env.Set("AMI_CALLER_ID", p.Ask("AMI_CALLER_ID", "Caller ID raqami", "103"))
	}

	// Secret defaulti: SETUP_ override → mavjud .env → yangi generatsiya.
	// Mavjud AMI user bilan ulanish uchun operator O'ZINIKINI kiritadi;
	// Enter bossa yangi yaratilgani qoladi (keyin FreePBX'da shu bilan
	// user ochiladi). Probe yiqilsa creds QAYTA so'raladi — noto'g'ri
	// parol bilan qotib qolinmaydi.
	defUser := firstNonEmpty(st.Env.Get("AMI_USERNAME"), "ecb")
	defSecret := firstNonEmpty(os.Getenv("SETUP_AMI_SECRET"), st.Env.Get("AMI_SECRET"))
	generated := ""
	if defSecret == "" {
		generated = genSecret()[:24]
		defSecret = generated
	}
	markedNew := false

	for {
		amiUser := p.Ask("AMI_USERNAME", "AMI username (mavjud AMI user bo'lsa o'shani yozing)", defUser)
		label := "AMI secret"
		if generated != "" && defSecret == generated {
			label = "AMI secret (Enter = yangi yaratilganini ishlatish)"
		}
		amiSecret := p.AskSecret("AMI_SECRET", label, defSecret)
		st.Env.Set("AMI_USERNAME", amiUser)
		st.Env.Set("AMI_SECRET", amiSecret)
		if amiSecret == generated && generated != "" && !markedNew {
			st.newSecrets = append(st.newSecrets, "AMI_SECRET")
			markedNew = true
			info("Yangi AMI secret ishlatiladi — FreePBX'da shu bilan user ochasiz (bundle'da tayyor)")
		}

		err := ProbeAMI(ctx, amiHost, atoi(amiPort, 5038), amiUser, amiSecret)
		if err == nil {
			ok("AMI login OK")
			return nil
		}
		warnf("AMI probe: %v [user=%q, parol=%d belgi]", err, amiUser, len(amiSecret))
		bundleDir, berr := WriteBundle(".", amiUser, amiSecret)
		if berr == nil {
			fmt.Printf("   FreePBX bundle tayyor: %s/ (manager_custom.conf, extensions_custom.conf, sounds/)\n", bundleDir)
			fmt.Println("   Mavjud user ishlatsangiz — yuqorida creds'ni qayta kiriting;")
			fmt.Println("   yangi user bo'lsa — bundle'ni FreePBX'da qo'llang: sudo fwconsole reload")
		}
		if p.NonInteractive {
			st.warnings = append(st.warnings, "AMI probe muvaffaqiyatsiz: "+err.Error())
			return nil
		}
		if !p.Pause("Qayta urinish uchun Enter bosing") {
			st.warnings = append(st.warnings, "AMI tekshirilmadi (skip)")
			return nil
		}
		defUser, defSecret = amiUser, amiSecret
	}
}

func stepAudio(ctx context.Context, p *Prompter, st *State) {
	audioDir := firstNonEmpty(st.Env.Get("AUDIO_DIR"), filepath.Join(st.AppDir, "audios"))
	st.Env.Set("AUDIO_DIR", audioDir)
	if err := ExtractAudios(audioDir); err != nil {
		warnf("Audio fayllarni chiqarib bo'lmadi: %v", err)
	} else {
		ok("6 ta WAV %s ichida tayyor", audioDir)
	}

	if !st.Facts.HasSox {
		if st.Facts.HasApt && p.AskYN("INSTALL_SOX", "sox o'rnatilsinmi? (admin panelda audio yuklash uchun kerak)", true) {
			st.InstallSox = true
		} else {
			st.warnings = append(st.warnings, "sox yo'q — audio yuklash sahifasi ishlamaydi")
		}
	}

	// PBX'ga audio yetkazish. ARI probe o'tgan bo'lsa avval PBX'ning o'zini
	// tekshiramiz — keraksiz savol bermaslik uchun:
	//   1) res_http_media_cache bor → audio HTTP orqali beriladi, PBX'ga
	//      fayl umuman kerak emas;
	//   2) promptlar PBX'da allaqachon bor → ko'chirish shart emas.
	st.SoundsDir = firstNonEmpty(st.Env.Get("PBX_SOUNDS_DIR"), "/var/lib/asterisk/sounds/en")
	if st.Backend == "ari" && st.ARIProbeOK {
		if st.ARIMediaCache {
			base := firstNonEmpty(st.Env.Get("AUDIO_MEDIA_BASE_URL"), st.SiteDomain)
			st.Env.Set("AUDIO_MEDIA_BASE_URL", p.Ask("AUDIO_MEDIA_BASE_URL",
				"res_http_media_cache topildi — audio HTTP orqali beriladi. Bazaviy URL (PBX shu manzilga yeta olishi kerak)", base))
			ok("PBX'ga audio fayl ko'chirish KERAK EMAS (HTTP rejim)")
			return
		}
		if st.ARISoundsExist {
			ok("Promptlar PBX'da allaqachon mavjud — ko'chirish kerak emas")
			return
		}
		warnf("PBX'da promptlar topilmadi va res_http_media_cache ham yo'q — ko'chirish kerak")
	}
	if p.AskYN("SSH_PUSH", "Audio fayllarni FreePBX'ga SSH orqali ko'chiraymi? (yo'q bo'lsa qo'lda ko'chirish ko'rsatmasi beriladi)", true) {
		host := firstNonEmpty(st.Env.Get("PBX_SSH_HOST"), st.Env.Get("AMI_HOST"))
		st.SSH.Host = p.Ask("PBX_SSH_HOST", "FreePBX SSH host", host)
		st.SSH.User = p.Ask("PBX_SSH_USER", "FreePBX SSH user", firstNonEmpty(st.Env.Get("PBX_SSH_USER"), "root"))
		st.SSH.KeyPath = st.Env.Get("PBX_SSH_KEY")
		st.SSH.Password = p.AskSecret("PBX_SSH_PASSWORD", "FreePBX SSH parol (key bo'lsa bo'sh qoldiring)", st.Env.Get("PBX_SSH_PASSWORD"))
		st.SoundsDir = p.Ask("PBX_SOUNDS_DIR", "Asterisk sounds katalogi", st.SoundsDir)
		st.SSHPush = true
		// .env'ga yozamiz — audio yuklash sahifasi avto-push uchun ishlatadi
		st.Env.Set("PBX_SSH_HOST", st.SSH.Host)
		st.Env.Set("PBX_SSH_USER", st.SSH.User)
		if st.SSH.Password != "" {
			st.Env.Set("PBX_SSH_PASSWORD", st.SSH.Password)
		}
		st.Env.Set("PBX_SOUNDS_DIR", st.SoundsDir)
	} else {
		fmt.Printf("   Qo'lda: scp %s/ambulance-*.wav root@<freepbx>:%s/ && ssh root@<freepbx> 'chown asterisk:asterisk %s/ambulance-*.wav'\n",
			audioDir, st.SoundsDir, st.SoundsDir)
	}
}

func stepEskiz(ctx context.Context, p *Prompter, st *State) {
	email := p.Ask("ESKIZ_EMAIL", "Eskiz email (bo'sh = SMS o'chiq/dry-run)", firstNonEmpty(st.Env.Get("ESKIZ_EMAIL"), "-"))
	if email == "-" || email == "" {
		st.Env.Set("ESKIZ_EMAIL", "")
		st.Env.Set("ESKIZ_DRY_RUN", "true")
		return
	}
	st.Env.Set("ESKIZ_EMAIL", email)
	st.Env.Set("ESKIZ_DRY_RUN", "false")
	if !st.Env.Has("ESKIZ_BASE_URL") {
		st.Env.Set("ESKIZ_BASE_URL", "https://notify.eskiz.uz/api")
	}
	for {
		pass := p.AskSecret("ESKIZ_PASSWORD", "Eskiz parol", st.Env.Get("ESKIZ_PASSWORD"))
		st.Env.Set("ESKIZ_PASSWORD", pass)
		if err := probeEskiz(ctx, st.Env.Get("ESKIZ_BASE_URL"), email, pass); err != nil {
			warnf("Eskiz auth: %v", err)
			if p.NonInteractive {
				st.warnings = append(st.warnings, "Eskiz auth muvaffaqiyatsiz: "+err.Error())
				return
			}
			if !p.AskYN("ESKIZ_RETRY", "Qayta urinasizmi?", true) {
				st.warnings = append(st.warnings, "Eskiz tekshirilmadi")
				return
			}
			continue
		}
		ok("Eskiz auth OK")
		return
	}
}

func printSummary(st *State) {
	fmt.Println("\n──────── Reja ────────")
	fmt.Printf("  Katalog:      %s (user: %s)\n", st.AppDir, st.ServiceUser)
	if st.PGMode == "keep" {
		fmt.Printf("  PostgreSQL:   mavjud DATABASE_URL saqlanadi\n")
	} else if st.PGMode == "remote" {
		fmt.Printf("  PostgreSQL:   masofaviy (provision qilinmaydi)\n")
	} else {
		fmt.Printf("  PostgreSQL:   %s rejimida provision: db=%s user=%s\n", st.PGMode, st.DBName, st.DBUser)
	}
	if st.InstallPG {
		fmt.Println("  O'rnatiladi:  postgresql (apt)")
	}
	if st.InstallSox {
		fmt.Println("  O'rnatiladi:  sox (apt)")
	}
	fmt.Printf("  Web:          :%s  (%s)\n", st.HTTPPort, st.SiteDomain)
	fmt.Printf("  Backend:      %s\n", st.Backend)
	if st.SSHPush {
		fmt.Printf("  Audio push:   %s@%s → %s\n", st.SSH.User, st.SSH.Host, st.SoundsDir)
	}
	fmt.Printf("  Admin:        %s\n", st.AdminUser)
	if st.Facts.HasSystemd {
		fmt.Println("  Servislar:    systemd (web + worker)")
	} else {
		fmt.Println("  Servislar:    systemd YO'Q → run-web.sh / run-worker.sh")
	}
	fmt.Println("──────────────────────")
}

func apply(ctx context.Context, p *Prompter, st *State) error {
	// apt installs first
	if st.InstallPG {
		info("PostgreSQL o'rnatilmoqda…")
		if err := aptInstall("postgresql"); err != nil {
			return fmt.Errorf("postgresql o'rnatish: %w", err)
		}
		_ = exec.Command("systemctl", "enable", "--now", "postgresql").Run()
		_ = exec.Command("service", "postgresql", "start").Run() // systemd'siz muhit
		// klaster ko'tarilishini kutamiz (15s gacha)
		for i := 0; i < 15; i++ {
			st.Facts.PG = detectPG()
			if st.Facts.PG.PeerAuthOK {
				break
			}
			time.Sleep(time.Second)
		}
		if !st.Facts.PG.PeerAuthOK {
			return fmt.Errorf("postgresql o'rnatildi, lekin peer auth ishlamayapti — qayta ishga tushiring")
		}
		st.PGMode = "peer"
		ok("PostgreSQL o'rnatildi")
	}
	if st.InstallSox {
		info("sox o'rnatilmoqda…")
		if err := aptInstall("sox"); err != nil {
			warnf("sox o'rnatib bo'lmadi: %v", err)
			st.warnings = append(st.warnings, "sox o'rnatilmadi")
		} else {
			ok("sox o'rnatildi")
		}
	}

	// DB provisioning
	if st.PGMode == "peer" || st.PGMode == "password" {
		info("Baza sozlanmoqda (db=%s user=%s)…", st.DBName, st.DBUser)
		admin := &PGAdmin{Peer: st.PGMode == "peer", SuperURL: st.DBSuperURL}
		roleExists, err := admin.RoleExists(ctx, st.DBUser)
		if err != nil {
			return fmt.Errorf("rolni tekshirish: %w", err)
		}
		syncPass := true
		if roleExists && !st.NewDBPass {
			syncPass = false // parol .env'dan qayta ishlatilyapti — tegmaymiz
		}
		if roleExists && st.NewDBPass {
			// mavjud rol, lekin bizda parol yo'q — reset qilishga aniq rozilik
			if !p.AskYN("RESET_DB_PASS", fmt.Sprintf("Rol '%s' mavjud, paroli noma'lum. Parolni yangilaymizmi?", st.DBUser), true) {
				return fmt.Errorf("baza paroli noma'lum — DATABASE_URL'ni remote rejimda kiriting")
			}
		}
		if err := admin.EnsureRole(ctx, st.DBUser, st.DBPassword, roleExists, syncPass); err != nil {
			return fmt.Errorf("rol yaratish: %w", err)
		}
		dbExists, err := admin.DBExists(ctx, st.DBName)
		if err != nil {
			return err
		}
		if err := admin.EnsureDB(ctx, st.DBName, st.DBUser, dbExists); err != nil {
			return fmt.Errorf("baza yaratish: %w", err)
		}
		ok("Baza va huquqlar tayyor (PG15+ safe)")

		if err := VerifyDatabaseURL(ctx, st.DatabaseURL); err != nil {
			return fmt.Errorf("app ulanishi ishlamayapti (pg_hba.conf tekshiring): %w", err)
		}
		ok("App ulanishi tekshirildi")
	}

	// .env write (merge)
	st.Env.Set("DATABASE_URL", st.DatabaseURL)
	st.Env.Set("HTTP_ADDR", ":"+st.HTTPPort)
	st.Env.Set("SITE_DOMAIN", st.SiteDomain)
	st.Env.Set("TELEPHONY_BACKEND", st.Backend)
	if !st.Env.Has("DB_POOL_MAX_CONNS") {
		st.Env.Set("DB_POOL_MAX_CONNS", "10")
		st.Env.Set("DB_POOL_MIN_CONNS", "2")
	}
	if !st.Env.Has("RIVER_MAX_WORKERS") {
		st.Env.Set("RIVER_MAX_WORKERS", "5")
	}
	if changed := st.Env.Diff(); len(changed) > 0 {
		info(".env yozilmoqda (o'zgargan kalitlar: %s)", strings.Join(changed, ", "))
	}
	if err := st.Env.Save(); err != nil {
		return fmt.Errorf(".env yozish: %w", err)
	}
	_ = os.Setenv("DATABASE_URL", st.DatabaseURL) // pastdagi qadamlar uchun

	// Baza konflikti: bazada goose bilmagan app jadvallar bo'lsa (eski
	// Django bazasi, chala o'rnatish) migratsiya baribir yiqiladi. Yechim
	// operatorda: backup + purge (default) yoki to'xtatish.
	if conflicts, err := AppTableConflicts(ctx, st.DatabaseURL); err == nil && len(conflicts) > 0 {
		warnf("Bazada eski/begona jadvallar topildi: %s", strings.Join(conflicts, ", "))
		choice := p.AskChoice("DB_CONFLICT", "Nima qilamiz?", []Choice{
			{"purge", "Backup olib TOZALASH — bazadagi HAMMA jadval o'chadi, keyin toza o'rnatiladi"},
			{"abort", "To'xtatish (boshqa baza nomi bilan qayta ishga tushirasiz)"},
		})
		if choice != "purge" {
			return fmt.Errorf("baza konflikti: setup'ni boshqa baza nomi bilan qayta ishga tushiring, yoki purge'ni tanlang (non-interactive: SETUP_DB_CONFLICT=purge)")
		}
		backup := filepath.Join(st.AppDir, fmt.Sprintf("%s-purge-backup-%s.sql.gz", st.DBName, time.Now().Format("20060102-150405")))
		info("Backup olinmoqda: %s …", backup)
		dumpTarget := st.DBName
		if st.PGMode == "remote" || st.PGMode == "keep" {
			dumpTarget = st.DatabaseURL
		}
		if err := DumpDatabase(ctx, st.PGMode == "peer" || st.PGMode == "install", dumpTarget, backup); err != nil {
			warnf("Backup olinmadi: %v", err)
			if !p.AskYN("PURGE_NO_BACKUP", "BACKUPSIZ o'chirilsinmi? (ma'lumot qaytmaydi!)", false) {
				return fmt.Errorf("purge bekor qilindi (backup yo'q)")
			}
			_ = os.Remove(backup)
		} else {
			ok("Backup tayyor: %s", backup)
		}
		if err := PurgeSchema(ctx, st.DatabaseURL); err != nil {
			return fmt.Errorf("purge: %w", err)
		}
		ok("Baza tozalandi — toza o'rnatish davom etadi")
	}

	// Migrations (in-process)
	info("Migratsiyalar qo'llanmoqda…")
	if err := migrations.GooseRun(ctx, st.DatabaseURL, "up"); err != nil {
		return fmt.Errorf("goose: %w", err)
	}
	cfg := &config.Config{DatabaseURL: st.DatabaseURL, DBPoolMaxConns: 5, DBPoolMinConns: 1}
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrations.RiverUp(ctx, pool); err != nil {
		return err
	}
	ok("Migratsiyalar OK (goose + River)")

	// Admin user
	exists, err := AdminUserExists(ctx, st.DatabaseURL, st.AdminUser)
	if err != nil {
		return fmt.Errorf("admin tekshirish: %w", err)
	}
	if exists {
		ok("Admin '%s' mavjud — parol o'zgartirilmadi", st.AdminUser)
		if !p.NonInteractive && p.AskYN("RESET_ADMIN_PASS", "Admin parolini yangilaymizmi?", false) {
			st.AdminPass = genSecret()[:16]
			if err := resetAdminPassword(ctx, pool, st.AdminUser, st.AdminPass); err != nil {
				return err
			}
			st.AdminCreated = true
			ok("Admin paroli yangilandi")
		}
	} else {
		st.AdminPass = genSecret()[:16]
		hash, err := auth.HashPassword(st.AdminPass)
		if err != nil {
			return err
		}
		q := sqlc.New(pool)
		if _, err := q.CreateUser(ctx, sqlc.CreateUserParams{
			Username: st.AdminUser, Password: hash,
			IsActive: true, IsStaff: true, IsSuperuser: true, Role: "admin",
		}); err != nil {
			return fmt.Errorf("admin yaratish: %w", err)
		}
		st.AdminCreated = true
		ok("Admin '%s' yaratildi", st.AdminUser)
	}

	// Audio SSH push
	if st.SSHPush {
		info("Audio fayllar FreePBX'ga ko'chirilmoqda (%s)…", st.SSH.Host)
		if err := st.SSH.PushAudios(st.Env.Get("AUDIO_DIR"), st.SoundsDir); err != nil {
			warnf("SSH push xato: %v", err)
			st.warnings = append(st.warnings, "audio SSH push muvaffaqiyatsiz — qo'lda ko'chiring")
		} else {
			ok("6 ta WAV ko'chirildi va chown qilindi")
		}
	}

	// Services
	if st.Facts.HasSystemd {
		info("systemd servislar o'rnatilmoqda…")
		if err := InstallUnits(st.AppDir, st.ServiceUser); err != nil {
			return fmt.Errorf("systemd: %w", err)
		}
		time.Sleep(2 * time.Second)
	} else {
		if err := WriteRunScripts(st.AppDir); err != nil {
			return err
		}
		warnf("systemd yo'q — qo'lda ishga tushiring: ./run-web.sh va ./run-worker.sh (yoki supervisor)")
		st.warnings = append(st.warnings, "systemd yo'q: servislar qo'lda boshqariladi")
	}

	// Health check
	healthy := false
	if st.Facts.HasSystemd {
		url := fmt.Sprintf("http://127.0.0.1:%s/users/login/", st.HTTPPort)
		client := http.Client{Timeout: 3 * time.Second}
		for i := 0; i < 5; i++ {
			if resp, err := client.Get(url); err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					healthy = true
					break
				}
			}
			time.Sleep(time.Second)
		}
		if healthy {
			ok("Web server javob beryapti: %s", url)
		} else {
			st.warnings = append(st.warnings, "web server javob bermayapti — journalctl -u emergency-callback-web -n 50")
		}
	}

	writeCredentials(st)

	fmt.Println()
	if len(st.warnings) == 0 && (healthy || !st.Facts.HasSystemd) {
		fmt.Println("════════ O'rnatish tugadi ════════")
	} else {
		fmt.Println("════ O'rnatish tugadi (ogohlantirishlar bilan) ════")
		for _, w := range st.warnings {
			warnf("%s", w)
		}
	}
	fmt.Printf("  Panel:  %s  (local: http://127.0.0.1:%s/users/login/)\n", st.SiteDomain, st.HTTPPort)
	if st.AdminCreated {
		fmt.Printf("  Admin:  %s / %s\n", st.AdminUser, st.AdminPass)
	} else {
		fmt.Printf("  Admin:  %s (parol o'zgartirilmagan)\n", st.AdminUser)
	}
	fmt.Println("  Tekshirish: ./emergency-callback doctor")
	return nil
}

func writeCredentials(st *State) {
	// Faqat YANGI yaratilgan qiymatlar yoziladi — saqlanganlar echo qilinmaydi.
	var b strings.Builder
	if st.AdminCreated {
		fmt.Fprintf(&b, "Admin: %s / %s\n", st.AdminUser, st.AdminPass)
	}
	if st.NewDBPass && (st.PGMode == "peer" || st.PGMode == "password") {
		fmt.Fprintf(&b, "DB:    %s / %s\n", st.DBUser, st.DBPassword)
	}
	for _, k := range st.newSecrets {
		if k == "AMI_SECRET" {
			fmt.Fprintf(&b, "AMI secret (FreePBX'da user oching): %s\n", st.Env.Get("AMI_SECRET"))
		}
	}
	// Yangi hech narsa bo'lmasa faylga TEGMAYMIZ (re-run eski yozuvlarni
	// o'chirib yubormasin); bo'lsa — ustiga emas, OXIRIGA qo'shamiz.
	if b.Len() == 0 {
		return
	}
	f, err := os.OpenFile("INSTALL_CREDENTIALS.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		warnf("INSTALL_CREDENTIALS.txt yozilmadi: %v", err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "─── %s ───\nPanel: %s\n%s\n", time.Now().UTC().Format(time.RFC3339), st.SiteDomain, b.String())
}

func resetAdminPassword(ctx context.Context, pool *pgxpool.Pool, username, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, "UPDATE users SET password=$1 WHERE username=$2", hash, username)
	return err
}

// --- helpers ---

func aptInstall(pkg string) error {
	env := append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	upd := exec.Command("apt-get", "update", "-qq")
	upd.Env = env
	_ = upd.Run()
	cmd := exec.Command("apt-get", "install", "-y", "-qq", pkg)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", pkg, strings.TrimSpace(string(out)))
	}
	return nil
}

func relocateSelf(dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dst, "emergency-callback"), data, 0o755)
}

func dirOwner(dir string) string {
	out, err := exec.Command("stat", "-c", "%U", dir).Output()
	if err != nil {
		return "root"
	}
	u := strings.TrimSpace(string(out))
	if u == "" || u == "UNKNOWN" {
		return "root"
	}
	return u
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func atoi(s string, def int) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
}

func urlEscape(s string) string {
	r := strings.NewReplacer("%", "%25", "@", "%40", ":", "%3A", "/", "%2F", "?", "%3F", "#", "%23", " ", "%20")
	return r.Replace(s)
}

// probeARIAudio checks (read-only) whether the PBX can play prompts over
// HTTP (res_http_media_cache) and whether the prompt files already exist in
// its sounds index. Errors degrade to false — the wizard then just asks.
func probeARIAudio(ctx context.Context, baseURL, user, pass string) (mediaCache, soundsExist bool) {
	base := strings.TrimRight(baseURL, "/")
	client := http.Client{Timeout: 5 * time.Second}
	get := func(path string) int {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return 0
		}
		req.SetBasicAuth(user, pass)
		resp, err := client.Do(req)
		if err != nil {
			return 0
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	// Diqqat: modul nomi ".so" bilan bo'lishi shart (aks holda 409).
	mediaCache = get("/asterisk/modules/res_http_media_cache.so") == 200
	soundsExist = get("/sounds/ambulance-rating-request") == 200
	return
}

// ProbeARI checks the ARI HTTP endpoint with basic auth.
func ProbeARI(ctx context.Context, baseURL, user, pass string) error {
	if baseURL == "" {
		return fmt.Errorf("ARI_URL bo'sh")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/asterisk/info", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, pass)
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ulanib bo'lmadi: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		return nil
	case 401, 403:
		return fmt.Errorf("auth rad etildi (ARI username/password tekshiring)")
	default:
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
}
