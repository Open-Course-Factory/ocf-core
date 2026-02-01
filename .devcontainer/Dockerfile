FROM golang:1.25-bookworm

# Installation minimale des outils système + Node.js + PostgreSQL client + glab
RUN apt-get update && apt-get install -y \
    vim \
    sudo \
    less \
    bash-completion \
    curl \
    ca-certificates \
    gnupg \
    postgresql-client \
    libpq-dev \
    wget \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && NODE_MAJOR=21 \
    && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_$NODE_MAJOR.x nodistro main" | tee /etc/apt/sources.list.d/nodesource.list \
    && apt-get update \
    && apt-get install -y nodejs \
    && curl https://raw.githubusercontent.com/docker/docker-ce/master/components/cli/contrib/completion/bash/docker -o /etc/bash_completion.d/docker.sh \
    && wget -O /tmp/glab.deb "https://gitlab.com/api/v4/projects/gitlab-org%2Fcli/packages/generic/glab/1.82.0/glab_1.82.0_linux_amd64.deb" \
    && dpkg -i /tmp/glab.deb \
    && rm /tmp/glab.deb \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Configuration de l'utilisateur
RUN useradd -ms /bin/bash ocf \
    && usermod -aG sudo ocf \
    && echo '%sudo ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers

ENV EDITOR=vim
SHELL ["/bin/bash", "-c"]
USER ocf
WORKDIR /workspaces/ocf-core

COPY --chown=ocf:ocf go.mod go.sum ./

# Installation des outils Go
RUN go install -v github.com/swaggo/swag/cmd/swag@latest \
    && go install -v github.com/go-delve/delve/cmd/dlv@latest \
    && go install -v golang.org/x/tools/gopls@latest
    
RUN go mod download && go mod verify && go clean -cache
RUN touch .bash_history .env

RUN git config --global core.pager 'less -F'

RUN curl -fsSL https://claude.ai/install.sh | bash