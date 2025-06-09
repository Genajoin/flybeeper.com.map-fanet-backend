FROM mcr.microsoft.com/devcontainers/go:1.23-bookworm

# Install additional tools and dependencies
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends \
        # Protocol Buffers compiler
        protobuf-compiler \
        # Build tools
        build-essential \
        # Database clients
        default-mysql-client \
        redis-tools \
        # MQTT tools
        mosquitto-clients \
        # Networking tools
        curl \
        wget \
        jq \
        # Process monitoring
        htop \
        # Text processing
        ripgrep \
        # Git tools
        git-flow \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install Go tools
RUN go install -v golang.org/x/tools/gopls@latest \
    && go install -v github.com/ramya-rao-a/go-outline@latest \
    && go install -v github.com/stamblerre/gocode@latest \
    && go install -v github.com/rogpeppe/godef@latest \
    && go install -v github.com/sqs/goreturns@latest \
    && go install -v github.com/uudashr/gopkgs/v2/cmd/gopkgs@latest \
    && go install -v github.com/cweill/gotests/gotests@latest \
    && go install -v github.com/fatih/gomodifytags@latest \
    && go install -v github.com/josharian/impl@latest \
    && go install -v github.com/haya14busa/goplay/cmd/goplay@latest \
    && go install -v github.com/go-delve/delve/cmd/dlv@latest \
    && go install -v honnef.co/go/tools/cmd/staticcheck@latest \
    && go install -v golang.org/x/tools/cmd/goimports@latest \
    && go install -v github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install Protocol Buffers Go plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Install air for hot reload during development (compatible version)
RUN go install github.com/cosmtrek/air@v1.49.0

# Install useful development tools
RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest \
    && go install github.com/pressly/goose/v3/cmd/goose@latest

# Set up workspace
WORKDIR /workspace

# Remove postCreate script copy (using inline command instead)

# Configure Git (will be overridden by user settings)
RUN git config --global init.defaultBranch main \
    && git config --global pull.rebase false

# Create directories for Go modules cache
RUN mkdir -p /go/pkg/mod && chown -R vscode:vscode /go

# Switch to vscode user
USER vscode

# Set up shell aliases and environment
RUN echo 'alias ll="ls -la"' >> ~/.bashrc \
    && echo 'alias la="ls -A"' >> ~/.bashrc \
    && echo 'alias l="ls -CF"' >> ~/.bashrc \
    && echo 'export PATH=$PATH:/go/bin' >> ~/.bashrc \
    && echo 'export GOPATH=/go' >> ~/.bashrc \
    && echo 'export GO111MODULE=on' >> ~/.bashrc

# Ensure Go tools are available
ENV PATH="${PATH}:/go/bin"