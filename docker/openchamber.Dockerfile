# syntax=docker/dockerfile:1
# OpenChamber runtime image.
#
# Installs @openchamber/web from npm (2.x declares engines node >= 24.14.0),
# following the official install.sh flow. OPENCHAMBER_VERSION pins the npm
# version; CI passes the released tag, "latest" tracks the npm dist-tag.

FROM node:24-slim

ARG OPENCHAMBER_VERSION=latest

RUN apt-get update && apt-get install -y --no-install-recommends \
        bash curl git ca-certificates openssh-client python3 make g++ \
    && rm -rf /var/lib/apt/lists/*

# Rename the base image's 'node' user to 'openchamber' so mounted volumes
# owned by UID/GID 1000 stay writable.
RUN usermod -l openchamber -d /home/openchamber -m node \
    && groupmod -n openchamber node \
    && mkdir -p /home/openchamber && chown -R openchamber:openchamber /home/openchamber

USER openchamber
ENV PATH=/home/openchamber/.local/bin:/home/openchamber/.npm-global/bin:$PATH
ENV NPM_CONFIG_PREFIX=/home/openchamber/.npm-global
# UTF-8 locale so the built-in terminal handles multibyte input correctly
# (parity with the official image).
ENV LANG=C.UTF-8
RUN mkdir -p /home/openchamber/.npm-global /home/openchamber/.local/bin

# 外部 OpenCode server 形态（OPENCODE_HOST + OPENCODE_SKIP_START）：不安装
# opencode CLI——v2 平台二进制解压约 212MB，且在此形态下从不真正运行。
# 但 `openchamber serve` 启动时无条件检查 opencode 二进制是否存在
# （bin/cli.js 的 checkOpenCodeCLI，找不到直接退出，外部形态也一样），
# 所以用 OPENCODE_BINARY 把检查指到任意可执行文件即可通过。
# 已在 2.4.0 上 A/B 验证：无 CLI 且无 OPENCODE_BINARY 时 serve 报
# "Unable to locate the opencode CLI on PATH" 退出；OPENCODE_BINARY=/bin/true
# 时正常监听、/health 200。若改回容器内自管 OpenCode：删除下面这行 ENV，
# 并在 npm install 里加回 CLI（@opencode/cli）。
ENV OPENCODE_BINARY=/bin/true

RUN npm install -g --no-audit --no-fund \
        @openchamber/web@${OPENCHAMBER_VERSION} \
    && npm cache clean --force \
    && openchamber --version || true

RUN mkdir -p /home/openchamber/.config/openchamber \
        /home/openchamber/.local/share/opencode \
        /home/openchamber/.local/state/opencode \
        /home/openchamber/.config/opencode \
        /home/openchamber/.ssh

EXPOSE 3000

CMD ["openchamber", "serve", "--lan", "--foreground"]
