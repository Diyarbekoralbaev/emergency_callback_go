// Package pbxssh copies audio prompts to the PBX over SSH. Used by the
// setup wizard (initial copy) and the admin audio-upload page (auto-push
// after a prompt is replaced).
package pbxssh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Config is how we reach the PBX.
type Config struct {
	Host     string // host[:port], port defaults to 22
	User     string
	Password string // used when set
	KeyPath  string // used when set (unencrypted private key)
	Timeout  time.Duration
}

// Configured reports whether enough fields are set to attempt a push.
func (c Config) Configured() bool {
	return c.Host != "" && c.User != "" && (c.Password != "" || c.KeyPath != "")
}

func (c Config) clientConfig() (*ssh.ClientConfig, error) {
	var auths []ssh.AuthMethod
	if c.KeyPath != "" {
		key, err := os.ReadFile(c.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("ssh key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("ssh key parse: %w", err)
		}
		auths = append(auths, ssh.PublicKeys(signer))
	}
	if c.Password != "" {
		auths = append(auths, ssh.Password(c.Password))
	}
	if len(auths) == 0 {
		return nil, fmt.Errorf("SSH parol yoki kalit berilmagan")
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &ssh.ClientConfig{
		User: c.User,
		Auth: auths,
		// The PBX host is operator-supplied at setup time; pinning host keys
		// would block every first install. Accepted risk for a LAN PBX.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}, nil
}

func (c Config) addr() string {
	if strings.Contains(c.Host, ":") {
		return c.Host
	}
	return c.Host + ":22"
}

// PushFile copies one local file to remotePath (cat > path), then chowns it
// to asterisk. No sftp dependency needed.
func (c Config) PushFile(localPath, remotePath string) error {
	cfg, err := c.clientConfig()
	if err != nil {
		return err
	}
	client, err := ssh.Dial("tcp", c.addr(), cfg)
	if err != nil {
		return fmt.Errorf("ssh %s: %w", c.addr(), err)
	}
	defer client.Close()

	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdin = strings.NewReader(string(data))
	// remotePath is app-controlled (sounds dir + whitelisted basename).
	cmd := fmt.Sprintf("cat > %q && chown asterisk:asterisk %q && chmod 644 %q", remotePath, remotePath, remotePath)
	if out, err := session.CombinedOutput(cmd); err != nil {
		return fmt.Errorf("remote write %s: %s: %w", remotePath, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// PushAudios copies every ambulance-*.wav in localDir to the PBX sounds dir.
func (c Config) PushAudios(localDir, remoteDir string) error {
	matches, err := filepath.Glob(filepath.Join(localDir, "ambulance-*.wav"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("%s ichida ambulance-*.wav topilmadi", localDir)
	}
	for _, m := range matches {
		if err := c.PushFile(m, filepath.Join(remoteDir, filepath.Base(m))); err != nil {
			return err
		}
	}
	return nil
}
