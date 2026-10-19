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

# The opencode CLI must exist even with an external OPENCODE_HOST: `openchamber
# serve` resolves the binary unconditionally (bin/cli.js checkOpenCodeCLI
# throws when missing). @opencode/cli is the v2 CLI that @openchamber/web 2.x
# pairs with (same version the official image installs); the v1-era
# `opencode-ai` package (latest 1.19.3) also works but makes the compatibility
# page report a v1 CLI.
RUN npm install -g --no-audit --no-fund \
        @opencode/cli@2.0.32 \
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
