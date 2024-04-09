FROM golang:latest

RUN apt-get update

# Install NodeJS
RUN apt-get install -y ca-certificates curl gnupg vim \
 && mkdir -p /etc/apt/keyrings \
 && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg

RUN export EDITOR=vim

RUN NODE_MAJOR=21 \
 && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_$NODE_MAJOR.x nodistro main" | tee /etc/apt/sources.list.d/nodesource.list

RUN apt-get update \
 && apt-get install nodejs -y

# Install NPM dependencies
RUN npm install -g @marp-team/marp-core \
    && npm install -g markdown-it-include \
    && npm install -g markdown-it-container \
    && npm install -g markdown-it-attrs

RUN apt-get install sudo -y

RUN useradd -ms /bin/bash ocf

RUN usermod -aG sudo ocf

RUN echo '%sudo ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers

SHELL ["/bin/bash", "-c"] 

RUN apt-get install bash-completion \
    && curl https://raw.githubusercontent.com/docker/docker-ce/master/components/cli/contrib/completion/bash/docker -o /etc/bash_completion.d/docker.sh \
    && source /etc/bash_completion

USER ocf

WORKDIR /workspaces/ocf-core

RUN go install -v github.com/swaggo/swag/cmd/swag@v1.8.12
RUN go install -v github.com/go-delve/delve/cmd/dlv@latest 
RUN go install -v golang.org/x/tools/gopls@latest

COPY go.mod go.sum ./
RUN go mod download && go mod verify


