# Prerequisites

What you need before installing.

## Server

- A Linux server (Ubuntu 22.04 / 24.04 recommended; any modern distro works).
- Root or `sudo` access.
- Outbound internet access (to reach the Eskiz SMS API and, during build, the
  Go module proxy).

## Software

| Software | Version | Why |
|----------|---------|-----|
| Go | 1.23+ | Build the binary. The `go.mod` toolchain directive auto-downloads a newer Go toolchain if needed, so a slightly older Go still works. |
| PostgreSQL | 14+ | Application data, the job queue, and sessions. |
| `git`, `curl`, `openssl` | any | Fetch code, generate secrets. |

Install on Ubuntu/Debian:

```bash
sudo apt update
sudo apt install -y postgresql git curl ca-certificates
# Go (adjust version/arch):
curl -fsSL https://go.dev/dl/go1.23.0.linux-amd64.tar.gz -o /tmp/go.tgz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tgz
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' | sudo tee /etc/profile.d/go.sh
source /etc/profile.d/go.sh
go version
```

## Telephony

You need an **existing FreePBX / Asterisk** server that:

- Runs Asterisk 18+ / 20+ with the **PJSIP** channel driver (`chan_pjsip`).
- Already has a **working outbound SIP trunk** that can place calls to the
  public phone network.
- Lets you reach its **AMI** port (default `5038`) from where the worker runs
  (commonly the worker runs **on the same host** as FreePBX, so AMI stays on
  `127.0.0.1`).

You will:

- Create an **AMI user** for this application (see
  [FreePBX Integration](../telephony/freepbx-integration.md)).
- Add a small **custom dialplan** (4 contexts).
- Install **6 audio prompt files**.

!!! info "No FreePBX yet?"
    For local testing you can stand up a bare Asterisk instead — see
    [Standalone Asterisk](../telephony/standalone-asterisk.md). Production is
    expected to use your existing FreePBX.

## SMS (optional but recommended)

An **Eskiz.uz** account with credit. Without it, the phone-rating flow still
works; only the SMS fallback for un-rated calls is unavailable. You can also set
`ESKIZ_DRY_RUN=true` to exercise the flow without sending real messages.

## Network ports summary

| Port | Used by | Notes |
|------|---------|-------|
| `8000` (configurable) | App web server | Put a TLS reverse proxy in front for production. |
| `5432` | PostgreSQL | |
| `5038` | Asterisk AMI | Keep bound to localhost where possible. |
| `5060` (or alternative) | Asterisk SIP (PJSIP transport) | On the PBX host. |

Next: [Installation](installation.md).
