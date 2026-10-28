FROM docker.io/library/golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS go-toolchain
FROM docker.io/library/node:24.17.0-bookworm-slim@sha256:862263c612aa437e3037674b85419622a9d93bff80aa1eee5398dfe686375532 AS node-toolchain
FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04@sha256:d94c97dd9cacf183d0a6fd12a8e87b526e9e928307674ae9c94139139c0c6eae

USER root
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

COPY ubuntu.sources /etc/apt/sources.list.d/ubuntu.sources
RUN apt-get update -o APT::Update::Error-Mode=any \
    && DEBIAN_FRONTEND=noninteractive apt-get install --yes --no-install-recommends \
        gcc \
        pkg-config \
        libgtk-4-dev \
        libwebkitgtk-6.0-dev \
        libfuse2 \
        xvfb \
        xauth \
    && rm -r /var/lib/apt/lists

COPY --from=go-toolchain /usr/local/go/ /usr/local/go/
COPY --from=node-toolchain /usr/local/bin/node /usr/local/bin/node
COPY --from=node-toolchain /usr/local/lib/node_modules/npm/ /usr/local/lib/node_modules/npm/

RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && install -d -o vscode -g vscode \
        /home/vscode/go \
        /home/vscode/.cache/go-build \
        /home/vscode/.npm \
        /workspaces/synchub-for-agents

ENV GOTOOLCHAIN=local \
    GOPATH=/home/vscode/go \
    PATH="/usr/local/go/bin:/home/vscode/go/bin:${PATH}"

USER vscode
WORKDIR /workspaces/synchub-for-agents

RUN test "$(id -u)" -ne 0 \
    && test "$(go env GOVERSION)" = "go1.26.6" \
    && test "$(node -p process.versions.node)" = "24.17.0" \
    && go version \
    && node --version \
    && npm --version \
    && npx --version \
    && gcc --version \
    && pkg-config --modversion gtk4 webkitgtk-6.0 \
    && ldconfig -p | grep -F libfuse.so.2 \
    && command -v xvfb-run \
    && command -v xauth
