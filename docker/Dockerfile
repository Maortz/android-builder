# Debian slim + Go baked in (official Go image is Debian-based)
FROM golang:1.24-bookworm

# --- Node 20 (for Claude Code CLI) + dev CLI tools ---
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y --no-install-recommends \
        nodejs \
        gh \
        ripgrep \
        fd-find \
        jq \
        ca-certificates \
        openssh-server \
        tmux \
        git \
    # fd ships as `fdfind` on Debian/Ubuntu; expose the usual `fd` name
    && ln -sf "$(command -v fdfind)" /usr/local/bin/fd \
    # yq (mikefarah) — not packaged reliably, grab the static binary
    && curl -fsSL https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 \
        -o /usr/local/bin/yq \
    && chmod +x /usr/local/bin/yq \
    && rm -rf /var/lib/apt/lists/*

# --- supercronic (non-root cron for Docker) ---
RUN curl -fsSL https://github.com/aptible/supercronic/releases/latest/download/supercronic-linux-amd64 \
    -o /usr/local/bin/supercronic \
    && chmod +x /usr/local/bin/supercronic

# --- Claude Code CLI ---
RUN npm install -g @anthropic-ai/claude-code

# --- non-root user (Claude refuses --dangerously-skip-permissions as root) ---
ARG UID=1001
ARG GID=1001
RUN groupadd -g ${GID} dev 2>/dev/null || true \
    && useradd -m -u ${UID} -g ${GID} -s /bin/bash dev \
    && mkdir -p /workspace /home/dev/.claude /home/dev/.local/bin \
    && chown -R dev:dev /workspace /home/dev \
    && git config --system --add safe.directory /workspace

COPY --chown=dev:dev claude-settings.seed.json /opt/claude/settings.seed.json
COPY --chown=dev:dev orchestrators.crontab /opt/claude/orchestrators.crontab
COPY --chown=dev:dev entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

USER dev
WORKDIR /workspace
ENV PATH="/home/dev/.local/bin:/usr/local/go/bin:${PATH}"
ENV GOPATH="/home/dev/go"

# --- rtk (Rust Token Killer) — installs to ~/.local/bin ---
RUN curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["bash"]
