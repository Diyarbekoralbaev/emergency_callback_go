package setup

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/config"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/migrations"
)

type CheckStatus string

const (
	Pass CheckStatus = "PASS"
	Warn CheckStatus = "WARN"
	Fail CheckStatus = "FAIL"
)

type CheckResult struct {
	Name   string
	Status CheckStatus
	Detail string
}

// RunDoctor performs read-only health checks and returns results in order.
func RunDoctor(ctx context.Context) []CheckResult {
	var results []CheckResult
	add := func(name string, st CheckStatus, detail string) {
		results = append(results, CheckResult{name, st, detail})
	}

	// 1. .env: mavjudligi, O'QILISHI (root yozgan bo'lsa oddiy user o'qiy
	// olmaydi — godotenv buni jimgina yutadi va "vars not set" degan
	// adashtiruvchi xato chiqadi) va rejimi.
	if st, err := os.Stat(".env"); err != nil {
		add(".env", Fail, "topilmadi (CWD: shu katalogda bo'lishi kerak)")
	} else if _, rerr := os.ReadFile(".env"); rerr != nil {
		if os.IsPermission(rerr) {
			add(".env", Fail, "O'QIB BO'LMAYAPTI (egasi boshqa user) — sudo bilan ishga tushiring yoki: sudo chown <servis-user> .env")
			return results
		}
		add(".env", Fail, rerr.Error())
		return results
	} else if st.Mode().Perm()&0o077 != 0 {
		add(".env", Warn, fmt.Sprintf("ruxsat %o — chmod 600 tavsiya etiladi", st.Mode().Perm()))
	} else {
		add(".env", Pass, "mavjud, 0600, o'qiladi")
	}

	// 2. Config completeness
	cfg, err := config.Load()
	if err != nil {
		add("config", Fail, err.Error())
		return results // qolgan tekshiruvlar config'siz ma'nosiz
	}
	add("config", Pass, "barcha majburiy o'zgaruvchilar bor")

	if len(bytesTrim(cfg.SessionSecret)) < 32 || len(bytesTrim(cfg.CSRFKey)) < 32 {
		add("secrets", Warn, "SESSION_SECRET/CSRF_KEY 32 baytdan qisqa — nol bilan to'ldirilgan (zaif)")
	} else {
		add("secrets", Pass, "uzunlik yetarli")
	}

	// 3. DB + migrations
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		add("database", Fail, err.Error())
	} else {
		defer pool.Close()
		add("database", Pass, "ulanish OK")
		if applied, total, err := migrations.RiverStatus(ctx, pool); err != nil {
			add("river-migratsiya", Fail, err.Error())
		} else if applied < total {
			add("river-migratsiya", Warn, fmt.Sprintf("%d/%d — 'migrate up' kerak", applied, total))
		} else {
			add("river-migratsiya", Pass, fmt.Sprintf("%d/%d", applied, total))
		}
		var n int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&n); err != nil {
			add("app-jadvallar", Fail, "users jadvali yo'q — 'migrate up' kerak")
		} else if n == 0 {
			add("app-jadvallar", Warn, "users bo'sh — 'createuser' kerak")
		} else {
			add("app-jadvallar", Pass, fmt.Sprintf("%d foydalanuvchi", n))
		}
	}

	// 4. Telefoniya — faqat tanlangan backend tekshiriladi
	if cfg.TelephonyBackend == "ari" {
		if err := ProbeARI(ctx, cfg.ARI.URL, cfg.ARI.Username, cfg.ARI.Password); err != nil {
			add("ARI", Fail, err.Error())
		} else {
			add("ARI", Pass, cfg.ARI.URL+" auth OK")
			mediaCache, soundsExist := probeARIAudio(ctx, cfg.ARI.URL, cfg.ARI.Username, cfg.ARI.Password)
			switch {
			case cfg.AudioMediaBaseURL != "" && mediaCache:
				add("ARI-audio", Pass, "HTTP rejim (res_http_media_cache) — PBX'da fayl kerak emas")
			case soundsExist:
				add("ARI-audio", Pass, "promptlar PBX sounds indeksida bor")
			default:
				add("ARI-audio", Fail, "PBX'da prompt ham, res_http_media_cache ham yo'q — qo'ng'iroqda audio chalinmaydi")
			}
		}
	} else {
		if err := ProbeAMI(ctx, cfg.AMI.Host, cfg.AMI.Port, cfg.AMI.Username, cfg.AMI.Secret); err != nil {
			add("AMI", Fail, err.Error())
		} else {
			add("AMI", Pass, fmt.Sprintf("%s:%d login OK", cfg.AMI.Host, cfg.AMI.Port))
		}
	}

	// 5. Eskiz
	if cfg.Eskiz.DryRun || cfg.Eskiz.Email == "" {
		add("Eskiz SMS", Warn, "dry-run rejimi (SMS yuborilmaydi)")
	} else if err := probeEskiz(ctx, cfg.Eskiz.BaseURL, cfg.Eskiz.Email, cfg.Eskiz.Password); err != nil {
		add("Eskiz SMS", Fail, err.Error())
	} else {
		add("Eskiz SMS", Pass, "auth OK")
	}

	// 6. sox + audio dir
	if commandExists("sox") {
		add("sox", Pass, "o'rnatilgan")
	} else {
		add("sox", Warn, "yo'q — audio yuklash sahifasi konvert qila olmaydi")
	}
	wavs, _ := filepath.Glob(filepath.Join(cfg.AudioDir, "ambulance-*.wav"))
	if len(wavs) >= 6 {
		add("audio-fayllar", Pass, fmt.Sprintf("%d ta WAV (%s)", len(wavs), cfg.AudioDir))
	} else {
		add("audio-fayllar", Warn, fmt.Sprintf("%d/6 WAV %s ichida", len(wavs), cfg.AudioDir))
	}

	// 7. Web health
	port := cfg.HTTPAddr
	if i := strings.LastIndex(port, ":"); i >= 0 {
		port = port[i+1:]
	}
	url := fmt.Sprintf("http://127.0.0.1:%s/users/login/", port)
	client := http.Client{Timeout: 5 * time.Second}
	if resp, err := client.Get(url); err != nil {
		add("web-server", Fail, fmt.Sprintf("%s javob bermayapti: %v", url, err))
	} else {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			add("web-server", Pass, url+" → 200")
		} else {
			add("web-server", Fail, fmt.Sprintf("%s → %d", url, resp.StatusCode))
		}
	}

	// 8. systemd units
	if st, err := os.Stat("/run/systemd/system"); err == nil && st.IsDir() {
		for _, m := range []string{"web", "worker"} {
			if UnitActive(m) {
				add("service-"+m, Pass, "active")
			} else {
				add("service-"+m, Warn, "active emas — journalctl -u emergency-callback-"+m)
			}
		}
	}

	return results
}

// bytesTrim drops the zero-padding padKey adds so we can see the real length.
func bytesTrim(b []byte) []byte { return bytes.TrimRight(b, "\x00") }

// probeEskiz does POST /auth/login without sending any SMS.
func probeEskiz(ctx context.Context, baseURL, email, password string) error {
	form := &bytes.Buffer{}
	mw := multipart.NewWriter(form)
	_ = mw.WriteField("email", email)
	_ = mw.WriteField("password", password)
	_ = mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/auth/login", form)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("eskiz login: HTTP %d (email/parol tekshiring)", resp.StatusCode)
	}
	return nil
}

// PrintResults renders the doctor table and returns true if nothing FAILed.
func PrintResults(results []CheckResult) bool {
	ok := true
	for _, r := range results {
		icon := map[CheckStatus]string{Pass: "✓", Warn: "!", Fail: "✗"}[r.Status]
		fmt.Printf(" %s %-18s %s\n", icon, r.Name, r.Detail)
		if r.Status == Fail {
			ok = false
		}
	}
	return ok
}
