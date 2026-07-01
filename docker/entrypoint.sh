#!/usr/bin/env bash
# Merge the baked seed into the (volume-mounted) settings.json so the rtk hook +
# plugins are configured without clobbering the persisted login.
set -e

CFG_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
SEED=/opt/claude/settings.seed.json
DEST="$CFG_DIR/settings.json"

mkdir -p "$CFG_DIR"

if [ -f "$DEST" ]; then
  # deep-merge: seed values win for the keys it defines, keep everything else
  merged="$(jq -s '.[0] * .[1]' "$DEST" "$SEED")"
  printf '%s\n' "$merged" > "$DEST"
else
  cp "$SEED" "$DEST"
fi

# --- Remote access: sshd + tmux (all rootless, under $HOME/.ssh) ---
SSH_DIR="$HOME/.ssh"
SSHD_CONFIG="$SSH_DIR/sshd_config"
HOST_KEY="$SSH_DIR/ssh_host_ed25519_key"

mkdir -p "$SSH_DIR"
chmod 700 "$SSH_DIR"

# Generate an ed25519 host key on first run (idempotent across restarts via the
# persisted home volume). Private host key must stay owner-only or sshd refuses it.
if [ ! -f "$HOST_KEY" ]; then
  ssh-keygen -t ed25519 -f "$HOST_KEY" -N '' -C "claude-container-host" >/dev/null
fi
chmod 600 "$HOST_KEY"

# Seed authorized_keys from $SSH_PUBKEY (host-provided). Appended only if absent,
# so restarts don't duplicate and manually-added keys are preserved.
AUTH_KEYS="$SSH_DIR/authorized_keys"
touch "$AUTH_KEYS"
chmod 600 "$AUTH_KEYS"
if [ -n "${SSH_PUBKEY:-}" ] && ! grep -qF "$SSH_PUBKEY" "$AUTH_KEYS"; then
  printf '%s\n' "$SSH_PUBKEY" >> "$AUTH_KEYS"
fi

# Key-based auth only — no passwords, no root. Everything lives under $HOME so
# the unprivileged `dev` user owns it and never needs sudo/root.
cat > "$SSHD_CONFIG" <<EOF
Port 2224
HostKey $HOST_KEY
PidFile $SSH_DIR/sshd.pid
AuthorizedKeysFile $SSH_DIR/authorized_keys
PasswordAuthentication no
PermitEmptyPasswords no
ChallengeResponseAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
PermitRootLogin no
UsePAM no
Subsystem sftp internal-sftp
EOF
chmod 600 "$SSHD_CONFIG"

# Start sshd as the dev user (runs unprivileged on port 2224).
if [ ! -f "$SSH_DIR/sshd.pid" ] || ! kill -0 "$(cat "$SSH_DIR/sshd.pid" 2>/dev/null)" 2>/dev/null; then
  /usr/sbin/sshd -f "$SSHD_CONFIG"
fi

# UTF-8 locale for interactive shells. SSH login shells don't inherit Docker's
# ENV, so LANG is empty there -> tmux thinks the client isn't UTF-8 and renders
# every glyph as a "_" placeholder. Setting it in ~/.bashrc fixes all shells.
if ! grep -q "LANG=C.UTF-8" "$HOME/.bashrc" 2>/dev/null; then
  printf '\n# Force UTF-8 so tmux renders glyphs not "_" (SSH shells lack Docker ENV)\nexport LANG=C.UTF-8\nexport LC_ALL=C.UTF-8\n' >> "$HOME/.bashrc"
fi

# tmux truecolor + UTF-8 passthrough.
TMUX_CONF="$HOME/.tmux.conf"
if [ ! -f "$TMUX_CONF" ]; then
  cat > "$TMUX_CONF" <<'EOF'
set  -g  default-terminal "tmux-256color"
set -ga terminal-overrides ",*:Tc"
set -ga terminal-features ",*:RGB"
EOF
fi

# Start a persistent tmux session named 'claude' (idempotent).
if command -v tmux >/dev/null 2>&1 && ! tmux has-session -t claude 2>/dev/null; then
  tmux new-session -d -s claude
fi

# --- Git setup for the Windows-host bind mount ---
git config --global core.autocrlf true

# Authenticate github.com pushes/pulls without prompting.
if [ -n "${GH_TOKEN:-}" ]; then
  git config --global credential.helper store
  printf 'https://x-access-token:%s@github.com\n' "$GH_TOKEN" > "$HOME/.git-credentials"
  chmod 600 "$HOME/.git-credentials"
fi

# --- Autonomous orchestrators (opt-in: set ENABLE_CRON=1) ---
if [ "${ENABLE_CRON:-0}" = "1" ]; then
  supercronic /opt/claude/orchestrators.crontab &
  echo "Orchestrator cron started (PID $!). Logs: /tmp/agent.log"
fi

exec "$@"
