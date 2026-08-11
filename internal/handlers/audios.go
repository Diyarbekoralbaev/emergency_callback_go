package handlers

import (
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/models"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/pbxssh"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/tz"
	"github.com/gin-gonic/gin"
)

// audioPrompt is one editable Asterisk WAV prompt. Key is the base filename
// (no extension); the file on disk is <AudioDir>/<Key>.wav — the same name
// Asterisk plays via the play-audio dialplan context (see ami.AudioMap).
// Titles/descriptions are Russian: this is the admin-facing UI.
type audioPrompt struct {
	Key         string
	Title       string
	Description string
}

// The six prompts the call flow plays. Order = play order in a typical call.
var audioPrompts = []audioPrompt{
	{"ambulance-rating-request", "Запрос оценки", "Главная фраза: просьба оценить работу бригады цифрами 1–5."},
	{"ambulance-rating-thankyou", "Благодарность", "Проигрывается сразу после того, как пациент поставил оценку."},
	{"ambulance-rating-invalid", "Неверный ввод", "Если нажата не 1–5 — просит повторить ввод."},
	{"ambulance-transfer-message", "Перевод оператору", "Сообщение перед соединением с живым оператором (0 или 9)."},
	{"ambulance-transfer-error", "Ошибка перевода", "Если оператор недоступен."},
	{"ambulance-failed-rating", "Оценка не получена", "Резервная фраза, когда оценка так и не собрана."},
}

func audioPromptByKey(key string) (audioPrompt, bool) {
	for _, p := range audioPrompts {
		if p.Key == key {
			return p, true
		}
	}
	return audioPrompt{}, false
}

type audioView struct {
	audioPrompt
	Exists   bool
	SizeKB   int64
	Modified string
}

type audioListData struct {
	models.BaseData
	Prompts  []audioView
	AudioDir string
	SoxOK    bool
}

// AudioList renders the admin audio-management page.
func (s *Server) AudioList(c *gin.Context) {
	bd := s.baseData(c)
	dir := s.Cfg.AudioDir

	items := make([]audioView, 0, len(audioPrompts))
	for _, p := range audioPrompts {
		v := audioView{audioPrompt: p}
		if st, err := os.Stat(filepath.Join(dir, p.Key+".wav")); err == nil {
			v.Exists = true
			v.SizeKB = (st.Size() + 1023) / 1024
			v.Modified = tz.ToTashkent(st.ModTime()).Format("02.01.2006 15:04")
		}
		items = append(items, v)
	}

	_, soxErr := exec.LookPath("sox")
	s.render(c, "audios/list.html", audioListData{
		BaseData: bd,
		Prompts:  items,
		AudioDir: dir,
		SoxOK:    soxErr == nil,
	})
}

// AudioPlay streams the current WAV so the admin can preview it in the browser.
func (s *Server) AudioPlay(c *gin.Context) {
	key := c.Param("key")
	if _, ok := audioPromptByKey(key); !ok {
		c.Status(http.StatusNotFound)
		return
	}
	fp := filepath.Join(s.Cfg.AudioDir, key+".wav")
	if _, err := os.Stat(fp); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	// Pre-set so http.ServeFile doesn't guess audio/wave inconsistently.
	c.Header("Content-Type", "audio/wav")
	c.Header("Cache-Control", "no-store")
	c.File(fp)
}

// CallMedia serves a prompt WAV to Asterisk (res_http_media_cache). Public
// but strictly whitelisted: only the six known prompt files, no traversal
// (the :file param must match "<key>.wav" for a known key).
func (s *Server) CallMedia(c *gin.Context) {
	file := c.Param("file")
	key := strings.TrimSuffix(file, ".wav")
	if key == file { // .wav suffix required
		c.Status(http.StatusNotFound)
		return
	}
	if _, ok := audioPromptByKey(key); !ok {
		c.Status(http.StatusNotFound)
		return
	}
	fp := filepath.Join(s.Cfg.AudioDir, key+".wav")
	if _, err := os.Stat(fp); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", "audio/wav")
	c.File(fp)
}

// AudioUpload accepts an uploaded audio file, converts it to the exact format
// Asterisk requires (8 kHz, mono, 16-bit signed PCM WAV) via sox, and writes it
// into place. Asterisk re-reads the file on the next call, so no reload needed.
func (s *Server) AudioUpload(c *gin.Context) {
	key := c.Param("key")
	prompt, ok := audioPromptByKey(key)
	if !ok {
		c.String(http.StatusNotFound, "unknown prompt")
		return
	}

	file, err := c.FormFile("audio")
	if err != nil {
		s.pushFlash(c, "danger", "Файл не выбран")
		c.Redirect(http.StatusFound, "/audios/")
		return
	}
	if file.Size > 20<<20 {
		s.pushFlash(c, "danger", "Файл слишком большой (максимум 20 МБ)")
		c.Redirect(http.StatusFound, "/audios/")
		return
	}

	// Save the raw upload to a temp file for sox to read.
	tmpIn, err := os.CreateTemp("", "ecb-audio-*")
	if err != nil {
		s.pushFlash(c, "danger", "Внутренняя ошибка (temp)")
		c.Redirect(http.StatusFound, "/audios/")
		return
	}
	tmpIn.Close()
	defer os.Remove(tmpIn.Name())
	if err := c.SaveUploadedFile(file, tmpIn.Name()); err != nil {
		s.pushFlash(c, "danger", "Не удалось сохранить загруженный файл")
		c.Redirect(http.StatusFound, "/audios/")
		return
	}

	if _, err := exec.LookPath("sox"); err != nil {
		s.pushFlash(c, "danger", "На сервере не установлен sox — конвертация невозможна")
		c.Redirect(http.StatusFound, "/audios/")
		return
	}

	// Convert into a temp file IN the target dir so the final rename is atomic
	// and same-filesystem, and so the new file inherits the dir's SELinux type.
	dst := filepath.Join(s.Cfg.AudioDir, key+".wav")
	tmpOut := filepath.Join(s.Cfg.AudioDir, "."+key+".upload.wav")
	// sox picks the input format from the file contents; forces 8 kHz mono 16-bit.
	cmd := exec.Command("sox", tmpIn.Name(),
		"-t", "wav", "-r", "8000", "-c", "1", "-b", "16", "-e", "signed-integer",
		tmpOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmpOut)
		slog.Error("sox convert failed", "key", key, "err", err, "out", string(out))
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		s.pushFlash(c, "danger", "Не удалось обработать аудио: "+msg)
		c.Redirect(http.StatusFound, "/audios/")
		return
	}

	_ = os.Chmod(tmpOut, 0o644)
	if err := os.Rename(tmpOut, dst); err != nil {
		os.Remove(tmpOut)
		slog.Error("audio rename failed", "key", key, "dst", dst, "err", err)
		s.pushFlash(c, "danger", "Не удалось записать файл в "+s.Cfg.AudioDir)
		c.Redirect(http.StatusFound, "/audios/")
		return
	}

	slog.Info("audio replaced", "key", key, "dst", dst)

	// Auto-push to the PBX when it plays local sound files (PBX_SSH_* set).
	// A failed push is a warning, not an error — the local copy is updated.
	sshCfg := pbxssh.Config{
		Host:     s.Cfg.PBXSSH.Host,
		User:     s.Cfg.PBXSSH.User,
		Password: s.Cfg.PBXSSH.Password,
		KeyPath:  s.Cfg.PBXSSH.KeyPath,
	}
	if sshCfg.Configured() {
		remote := filepath.Join(s.Cfg.PBXSSH.SoundsDir, key+".wav")
		if err := sshCfg.PushFile(dst, remote); err != nil {
			slog.Error("audio pbx push", "key", key, "err", err)
			s.pushFlash(c, "warning", "Аудио «"+prompt.Title+"» обновлено локально, но НЕ скопировано на АТС: "+err.Error())
			c.Redirect(http.StatusFound, "/audios/")
			return
		}
		slog.Info("audio pushed to pbx", "key", key, "remote", remote)
		s.pushFlash(c, "success", "Аудио «"+prompt.Title+"» обновлено и скопировано на АТС. Действует со следующего звонка.")
		c.Redirect(http.StatusFound, "/audios/")
		return
	}

	s.pushFlash(c, "success", "Аудио «"+prompt.Title+"» обновлено. Изменения вступят в силу со следующего звонка.")
	c.Redirect(http.StatusFound, "/audios/")
}
