package setup

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	embedded "github.com/Diyarbekoralbaev/emergency_callback_go"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/pbxssh"
	"github.com/staskobzar/goami2"
)

// SSHConfig is how we reach the PBX for the audio copy (shared with the
// audio-upload auto-push in internal/pbxssh).
type SSHConfig = pbxssh.Config

// ProbeAMI dials the AMI port and attempts a login. Returns nil on success.
func ProbeAMI(ctx context.Context, host string, port int, username, secret string) error {
	addr := net.JoinHostPort(host, fmt.Sprint(port))
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("AMI %s ga ulanib bo'lmadi: %w", addr, err)
	}
	defer conn.Close()

	loginCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = goami2.NewClientWithContext(loginCtx, conn, username, secret)
	if err != nil {
		return fmt.Errorf("AMI login rad etildi (username/secret tekshiring): %w", err)
	}
	// Diqqat: goami2 Client.Close() ni darhol chaqirish MUMKIN EMAS —
	// kutubxonaning loop goroutine'i hali ishga tushmagan bo'lsa, Close
	// c.conn=nil qiladi va goroutine nil'ga consume() chaqirib BUTUN
	// jarayonni panic bilan o'ldiradi (lab'da aniqlangan). Faqat raw
	// conn'ni yopamiz (defer yuqorida) — loop toza xato bilan tugaydi.
	return nil
}

// dialplan is the FreePBX-side contract (identical to the docs bundle).
const dialplanConf = `; Append to /etc/asterisk/extensions_custom.conf on the FreePBX server, then:
;   sudo fwconsole reload   (or: sudo asterisk -rx 'dialplan reload')
; Do NOT add a [from-internal] block — FreePBX owns it (outbound routes).

[ambulance-callback]
exten => s,1,NoOp(ANSWERED CALL_ID=${CALL_ID} PHONE=${PHONE_NUMBER})
 same => n,Answer()
 same => n,UserEvent(CallAnswered,CallID: ${CALL_ID},Phone: ${PHONE_NUMBER})
 same => n,Wait(300)
 same => n,Hangup()

[play-audio]
exten => _.,1,NoOp(PLAY ${EXTEN} CALL_ID=${CALL_ID})
 same => n,Playback(${EXTEN})
 same => n,UserEvent(AudioPlayed,CallID: ${CALL_ID},Audio: ${EXTEN})
 same => n,WaitExten(60)
 same => n,Wait(60)
 same => n,Hangup()

[transfer-to-337]
exten => s,1,NoOp(TRANSFER CALL_ID=${CALL_ID})
 same => n,Dial(Local/337@from-internal,30)   ; <-- set your operator extension/queue
 same => n,Hangup()
`

// WriteBundle generates freepbx-bundle/ (AMI user conf, dialplan, README).
// Files are 0600 — manager_custom.conf embeds the AMI secret.
func WriteBundle(appDir, amiUser, amiSecret string) (string, error) {
	dir := filepath.Join(appDir, "freepbx-bundle")
	if err := os.MkdirAll(filepath.Join(dir, "sounds"), 0o700); err != nil {
		return "", err
	}

	manager := fmt.Sprintf(`; Append to /etc/asterisk/manager_custom.conf on the FreePBX server, then:
;   sudo fwconsole reload   (or: sudo asterisk -rx 'manager reload')
[%s]
secret = %s
deny = 0.0.0.0/0.0.0.0
permit = 0.0.0.0/0.0.0.0
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan,originate
write = system,call,agent,user,config,command,reporting,originate,message
`, amiUser, amiSecret)

	if err := os.WriteFile(filepath.Join(dir, "manager_custom.conf"), []byte(manager), 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "extensions_custom.conf"), []byte(dialplanConf), 0o600); err != nil {
		return "", err
	}
	if err := ExtractAudios(filepath.Join(dir, "sounds")); err != nil {
		return "", err
	}
	return dir, nil
}

// ExtractAudios writes the six WAV prompts into dstDir, preferring the disk
// audios/ copy and falling back to the embedded one.
func ExtractAudios(dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	entries, err := embedded.Audios.ReadDir("audios")
	if err != nil {
		return err
	}
	for _, e := range entries {
		dst := filepath.Join(dstDir, e.Name())
		// disk copy wins if the caller already has one there
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		var data []byte
		if d, err := os.ReadFile(filepath.Join("audios", e.Name())); err == nil {
			data = d
		} else if d, err := embedded.Audios.ReadFile("audios/" + e.Name()); err == nil {
			data = d
		} else {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

