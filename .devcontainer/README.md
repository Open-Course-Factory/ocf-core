# OCF Core Dev Container

This dev container comes pre-configured with everything you need for OCF Core development, including AI-powered development tools.

## 🚀 What's Included

### Development Tools
- **Go 1.24** - Main language
- **Node.js 21** - For MCPs and frontend tools
- **PostgreSQL Client** - Database CLI and MCP prerequisites
- **Docker-in-Docker** - Access to sibling containers
- **Git** - With bash completion
- **Swag** - API documentation generator
- **Delve** - Go debugger
- **gopls** - Go language server

### AI-Powered Development
- **Claude Code Extension** - AI pair programmer
- **20 Custom Commands** - Development workflows (`.claude/commands/`)
- **7 Review Agents** - Automated code quality checks
- **5 MCP Servers** - Direct integrations with databases, Docker, Git

## 🔌 MCP (Model Context Protocol) Servers

The dev container automatically configures these MCPs for Claude Code:

### PostgreSQL MCP
Query your databases directly through Claude:
```
"Show me all users with role administrator"
"Find organizations with more than 10 members"
"Show the schema for groups table"
```

**Configured databases:**
- `postgres` - Main database (ocf)
- `postgres-test` - Test database (ocf_test)

### Docker MCP
Manage containers without leaving the chat:
```
"Show all running containers"
"Check postgres logs from last 10 minutes"
"Is the casdoor service healthy?"
```

### Git MCP
Analyze repository history:
```
"Who wrote the payment service?"
"Find all commits mentioning security"
"Show changes to entity management system"
```

### Filesystem MCP
Enhanced file operations:
```
"Find all files with TODO comments"
"List all DTO files"
"Show files modified in last 24 hours"
```

### Fetch MCP
Test APIs directly:
```
"Test the organizations endpoint with auth"
"Make a GET request to /api/v1/users"
```

## 📦 Container Setup Process

When you open the project in VS Code:

1. **Container Build**
   - Installs Go 1.24 + Node.js 21
   - Sets up user `ocf` with sudo access
   - Installs Go tools (swag, dlv, gopls)
   - Downloads Go dependencies

2. **Post-Create Setup** (`.devcontainer/setup-mcps.sh`)
   - Configures npm for user-level packages
   - Installs/prepares MCP servers
   - Sets up PATH for npm global binaries
   - Displays setup summary

3. **Extension Setup**
   - Installs Claude Code extension
   - Configures MCP servers automatically
   - Loads custom commands from `.claude/commands/`
   - Activates Go extension with proper settings

## 🎯 First-Time Setup

### Prerequisites: Shared Docker Network

OCF projects (ocf-core, ocf-front, tt-backend) use a shared Docker network for cross-project communication. Create it once on your host:

```bash
docker network create ocf-shared
```

Services exposed on this network:
| Service | Address | Description |
|---------|---------|-------------|
| ocf-core | `ocf-core:8080` | Core API |
| postgres | `postgres:5432` | PostgreSQL database |

Internal services (casdoor, pgadmin, casdoor_db) remain on the internal `devcontainer-network` only.

### Open in Dev Container
```bash
# Option 1: VS Code Command Palette
Ctrl+Shift+P → "Dev Containers: Reopen in Container"

# Option 2: GitHub Codespaces
Just open the repository in Codespaces
```

### Verify Everything Works
```bash
# Check Node.js
node --version  # Should show v21.x

# Check npm
npm --version   # Should show 10.x+

# Check Go
go version      # Should show 1.24

# Check Docker access
docker ps       # Should show postgres, casdoor, etc.

# Check Claude Code
# Ask in Claude Code: "What MCPs do you have access to?"
```

## 📚 Available AI Agents

Once the container is running, you can use these commands in Claude Code:

### Development Commands
- `/new-entity` - Create complete entity with DTOs, registration, tests
- `/migrate` - Handle database migrations
- `/update-docs` - Regenerate Swagger documentation

### Testing Commands
- `/test` - Smart test runner
- `/debug-test` - Debug failing tests
- `/api-test` - Quick API endpoint testing

### Code Quality Commands
- `/review-pr` - Comprehensive PR review
- `/enforce-patterns` - Check pattern compliance
- `/security-scan` - Security vulnerability scan
- `/performance-audit` - Performance analysis
- `/architecture-review` - Architecture validation
- `/pre-commit` - Pre-commit quality gate

### Analysis Commands
- `/find-pattern` - Find implementation examples
- `/explain` - Explain how systems work
- `/check-permissions` - Debug permissions

**Or just ask naturally!** You don't need to use slash commands:
- "Review my code"
- "Run pre-commit checks"
- "Show all organizations from the database"

## 🔧 Configuration Files

### `.devcontainer/devcontainer.json`
Main configuration:
- Container setup
- Extensions to install
- MCP server configuration
- Port forwarding
- Mounts (SSH keys, bash history)

### `.devcontainer/Dockerfile`
Container image definition:
- Base image (golang:1.24-bookworm)
- System packages (Node.js, vim, etc.)
- User configuration
- Go tools installation

### `.devcontainer/setup-mcps.sh`
Post-create setup script:
- npm configuration
- MCP installation
- PATH setup
- Verification

### `.devcontainer/docker-compose.yml`
Container overrides for dev environment

## 🐛 Troubleshooting

### MCPs Not Working
```bash
# Check if Node.js is available
node --version

# Check npm global packages
npm list -g --depth=0

# Check PATH includes npm global
echo $PATH | grep npm-global

# Re-run MCP setup
.devcontainer/setup-mcps.sh
```

### Docker Access Issues
```bash
# Check if docker is accessible
docker ps

# If permission denied, check user groups
groups

# You should see 'docker' in the list
# If not, rebuild the container
```

### Database Connection Issues
```bash
# Check if postgres container is running
docker ps | grep postgres

# Test connection manually
psql postgresql://ocf:root@postgres:5432/ocf

# Check from Claude Code
# Ask: "Test the postgres connection"
```

### Claude Code Extension Issues
```bash
# Check if extension is installed
code --list-extensions | grep anthropic

# Reload window
Ctrl+Shift+P → "Developer: Reload Window"

# Check settings
Ctrl+Shift+P → "Preferences: Open Settings (JSON)"
# Look for claude-code.mcpServers
```

## 📖 Documentation

### Quick Start
- `.claude/QUICK_REFERENCE.md` - Command quick reference
- `.claude/MCP_QUICKSTART.md` - MCP 5-minute setup
- `.claude/AGENTS_GUIDE.md` - Complete agent guide

### Detailed Guides
- `.claude/commands/REVIEW_AGENTS_README.md` - All agents explained
- `.claude/MCP_SETUP_GUIDE.md` - Complete MCP documentation
- `.claude/COMPLETE_SETUP_SUMMARY.md` - Everything in one place

### Project Documentation
- `CLAUDE.md` - Main project guide
- `.claude/docs/` - Feature-specific documentation

## 🔒 Security Notes

### Database Credentials
The dev container includes database credentials for local development:
- User: `ocf`
- Password: `root`

**These are for development only!** Never use in production.

### MCP Configuration
MCPs are configured to access:
- **Database**: Read/write access to local PostgreSQL
- **Docker**: Read-only access to container info and logs
- **Filesystem**: Read/write access to `/workspaces/ocf-core`
- **Git**: Read-only access to repository history

All access is limited to the dev container environment.

## 🚀 Performance Tips

### First Container Build
The first build takes 5-10 minutes:
- Downloads base images
- Installs system packages
- Downloads Go dependencies
- Sets up MCP servers

### Subsequent Rebuilds
Rebuilds are much faster (1-2 minutes) thanks to:
- Docker layer caching
- Go module caching
- npm cache

### Speed Up Builds
```bash
# Use GitHub Codespaces for cloud-based containers
# Pre-built images available

# Or use VS Code Dev Containers locally
# Cached layers persist across rebuilds
```

## 🎓 Getting Started Guide

### Day 1: Setup & Exploration
1. Open project in dev container
2. Wait for post-create setup
3. Ask Claude: "What can you help me with?"
4. Read `.claude/QUICK_REFERENCE.md`

### Day 2: First Commands
1. Try: "Review the organization entity"
2. Try: "Show me all users from database"
3. Try: "Check if all containers are running"
4. Try: "Run pre-commit checks"

### Day 3: Daily Workflow
1. Use `/pre-commit` before every commit
2. Use natural language queries with MCPs
3. Ask Claude to explain complex code
4. Use agents for code reviews

### Week 1: Mastery
1. Read full agent documentation
2. Create custom commands if needed
3. Experiment with all MCPs
4. Share findings with team

## 💡 Pro Tips

1. **Natural Language First**: Don't memorize commands, just ask naturally
2. **Context-Aware**: Describe what you're doing, Claude will suggest the right agents
3. **Parallel Work**: Claude can run multiple agents/MCPs at once
4. **Database Queries**: Skip writing SQL, just describe what you need
5. **Container Debugging**: Ask Claude to check logs instead of manual `docker logs`

## 🤝 Team Onboarding

When onboarding new developers:

1. **Clone repo** → Open in VS Code
2. **VS Code prompts** to open in dev container → Click "Reopen in Container"
3. **Wait for setup** → Coffee break (5-10 min first time)
4. **Verify setup** → Ask Claude "What MCPs do you have?"
5. **Start coding** → Everything just works!

No manual setup required! ✨

## 📞 Getting Help

### Issues with Dev Container
- Check `.devcontainer/README.md` (this file)
- Run troubleshooting commands above
- Ask Claude: "Help me debug my dev container setup"

### Issues with MCPs
- Read `.claude/MCP_QUICKSTART.md`
- Run `.devcontainer/setup-mcps.sh` manually
- Ask Claude: "Why aren't my MCPs working?"

### Issues with Agents
- Read `.claude/AGENTS_GUIDE.md`
- Type `/` to see available commands
- Ask naturally: "Review my code" works without slash commands

## 🎉 You're Ready!

Your dev container is a complete, AI-powered development environment:
- ✅ All tools installed
- ✅ MCPs configured
- ✅ Agents ready
- ✅ Database connected
- ✅ Docker access
- ✅ Git integrated

**Start developing with superpowers!** 🚀
