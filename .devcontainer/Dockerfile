FROM mcr.microsoft.com/devcontainers/base:ubuntu

RUN apt-get update && apt-get install -y \
    curl \
    wget \
    gnupg \
    ca-certificates \
    python3 \
    python3-pip \
    pipx \
    libgdiplus \
    && rm -rf /var/lib/apt/lists/*

COPY certs/ /tmp/certs/
RUN if [ "$(ls -A /tmp/certs/*.crt 2>/dev/null)" ]; then \
        cp /tmp/certs/*.crt /usr/local/share/ca-certificates/ && \
        update-ca-certificates; \
    fi && \
    rm -rf /tmp/certs

RUN wget https://packages.microsoft.com/config/ubuntu/24.04/packages-microsoft-prod.deb -O packages-microsoft-prod.deb \
    && dpkg -i packages-microsoft-prod.deb \
    && rm packages-microsoft-prod.deb \
    && apt-get update \
    && apt-get install -y dotnet-sdk-10.0 \
    && rm -rf /var/lib/apt/lists/

RUN curl -fsSL https://deb.nodesource.com/setup_24.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN corepack enable && corepack prepare pnpm@latest --activate
RUN corepack enable && corepack install --global pnpm@latest
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0

RUN curl -fsSL https://get.docker.com -o get-docker.sh \
    && sh get-docker.sh \
    && rm get-docker.sh \
    && usermod -aG docker vscode

RUN apt-get update --quiet && \
  apt-get install --quiet --yes --no-install-recommends software-properties-common gnupg openssh-client git ca-certificates && \
  add-apt-repository --yes ppa:fish-shell/release-4 && \
  apt-get install --quiet --yes --no-install-recommends fish && \
  apt-get clean && \
  rm -rf /var/lib/apt/lists/*

RUN su - vscode -c "dotnet tool install --global dotnet-ef" && \
    su - vscode -c "dotnet tool install --global dotnet-aspnet-codegenerator"

RUN su - "vscode" -c "curl -fsSL https://opencode.ai/install | bash" && \
    mkdir -p /home/vscode/.local/share/opencode /home/vscode/.local/state/opencode  && \
    chown -R vscode:vscode /home/vscode/.local/share

RUN mkdir -p /app/data && chown vscode:vscode /app/data

RUN echo fs.inotify.max_user_instances=524288 | sudo tee -a /etc/sysctl.conf && sudo sysctl -p

COPY --from=ghcr.io/astral-sh/uv:latest /uv /uvx /bin/

RUN chsh -s /usr/bin/fish

USER vscode

# Install fisher & pure-fish prompt & fish git plugin
RUN fish -c 'curl -sL https://raw.githubusercontent.com/jorgebucaran/fisher/main/functions/fisher.fish | source && fisher install jorgebucaran/fisher' && \
    fish -c 'fisher install pure-fish/pure' && \
    fish -c 'fisher install jhillyerd/plugin-git' && \
    fish -c 'set --universal pure_color_primary green' && \
    fish -c 'set --universal pure_enable_single_line_prompt true' && \
    fish -c 'fish_add_path -U /home/vscode/.dotnet/tools' && \
    fish -c 'fish_add_path -U /home/vscode/.opencode/bin/'

RUN uv tool install specify-cli --from git+https://github.com/github/spec-kit.git

ENV SHELL=/usr/bin/fish

ENV NODE_EXTRA_CA_CERTS=/usr/local/share/ca-certificates/home.crt

RUN sudo apt update && sudo apt install -y libgtk-4-dev

# Install Go 1.26.1
RUN wget https://go.dev/dl/go1.26.1.linux-amd64.tar.gz -O /tmp/go.tar.gz && \
    sudo tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz

# Add Go to PATH for all users
ENV PATH=/usr/local/go/bin:${PATH}

# Install GoReleaser
RUN curl -fsSL https://github.com/goreleaser/goreleaser/releases/download/v2.16.0/goreleaser_2.16.0_amd64.deb -o /tmp/goreleaser.deb && \
    sudo dpkg -i /tmp/goreleaser.deb && \
    rm /tmp/goreleaser.deb
