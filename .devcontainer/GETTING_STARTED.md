# 🚀 Getting Started with OCF Core Dev Container

## For New Developers

Welcome! This project uses a **pre-configured dev container** with AI-powered development tools. Everything is set up automatically when you open the project.

## ⚡ Quick Start (5 Minutes)

### Step 1: Open in Dev Container

**Option A: VS Code**
```bash
1. Clone the repository
2. Open in VS Code
3. Click "Reopen in Container" when prompted
   (or Ctrl+Shift+P → "Dev Containers: Reopen in Container")
```

**Option B: GitHub Codespaces**
```bash
1. Go to the repository on GitHub
2. Click "Code" → "Codespaces" → "Create codespace"
3. Wait for setup (coffee break!)
```

### Step 2: Wait for Setup (First Time: 5-10 minutes)

You'll see:
```
🔌 Setting up MCPs for OCF Core Development
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✅ Node.js v21.x.x
✅ npm 10.x.x

📥 Installing MCP servers...
...
✅ MCP Setup Complete!
```

### Step 3: Verify Everything Works

**Check Node.js & npm:**
```bash
node --version  # v21.x.x
npm --version   # 10.x.x
```

**Check Docker access:**
```bash
docker ps  # Should show postgres, casdoor, etc.
```

**Check Claude Code with MCPs:**
```
Ask in Claude Code chat: "What MCPs do you have access to?"

Expected response:
- postgres (database queries)
- docker (container management)
- git (repository analysis)
- filesystem (file operations)
- fetch (HTTP requests)
```

### Step 4: Try Your First AI Command

**Ask Claude Code naturally:**
```
"Show me all organizations from the database"
"What containers are running?"
"Review the organization entity"
"Run pre-commit checks"
```

**Or use slash commands:**
```
/review-pr
/test
/security-scan
```

## 🎯 Your First Day

### Morning: Setup & Exploration (30 minutes)

1. **Read the quick reference:**
   ```bash
   code .claude/QUICK_REFERENCE.md
   ```

2. **Try database queries with MCP:**
   ```
   Ask Claude: "Show me all users"
   Ask Claude: "Find organizations with more than 5 members"
   ```

3. **Check running services:**
   ```
   Ask Claude: "Are all containers healthy?"
   Ask Claude: "Show postgres logs from last 5 minutes"
   ```

### Afternoon: First Feature (1-2 hours)

1. **Understand existing code:**
   ```
   Ask Claude: "Explain how the organization system works"
   Ask Claude: "Show me the entity registration pattern"
   ```

2. **Make a small change:**
   ```
   Edit a file, then ask:
   "Review my changes"
   "Run pre-commit checks"
   "Test the organizations endpoint"
   ```

3. **Commit your work:**
   ```bash
   # Claude will guide you through:
   # 1. Pre-commit validation
   # 2. Test execution
   # 3. Commit message suggestion
   ```

## 📚 Essential Reading (1 Hour Total)

### Must Read First (20 minutes)
1. `.claude/QUICK_REFERENCE.md` (5 min) - Command cheat sheet
2. `.devcontainer/README.md` (10 min) - Dev container overview
3. `.claude/MCP_QUICKSTART.md` (5 min) - MCP basics

### Read This Week (40 minutes)
4. `.claude/AGENTS_GUIDE.md` (15 min) - Complete agent guide
5. `.claude/commands/REVIEW_AGENTS_README.md` (15 min) - Agent details
6. `CLAUDE.md` (10 min) - Project overview

## 🎓 Learning Path

### Week 1: Bronze Level
**Goal: Build muscle memory with basic commands**

Daily routine:
```bash
# Before every commit
Ask: "Run pre-commit checks"

# When stuck
Ask: "Explain how [system] works"

# For testing
Ask: "Test the [endpoint] endpoint with auth"

# Database queries
Ask: "Show me [data] from database"
```

### Week 2: Silver Level
**Goal: Use agents proactively**

Start using:
- `/review-pr` before pushing
- `/security-scan` for security-related changes
- `/new-entity` for creating new entities
- Natural language queries with database MCP

### Month 2: Gold Level
**Goal: Autonomous development**

Master:
- Weekly `/enforce-patterns` runs
- Monthly `/improve` sessions
- Custom command creation
- All MCP integrations

## 🔧 What's Pre-Configured

### Development Tools
✅ Go 1.24
✅ Node.js 21
✅ Docker-in-Docker
✅ Git with bash completion
✅ Swag (API docs)
✅ Delve (debugger)
✅ gopls (language server)

### AI Tools
✅ Claude Code extension
✅ 20 custom commands
✅ 7 review agents
✅ 5 MCP servers

### Database Access
✅ PostgreSQL (main)
✅ PostgreSQL (test)
✅ Direct queries via MCP

### Container Access
✅ View all containers
✅ Check logs
✅ Monitor health

## 💡 Pro Tips

### 1. Don't Memorize Commands
Just ask naturally:
- ✅ "Review my code"
- ❌ "I need to run /review-pr --check-patterns --security"

### 2. Use Database MCP Constantly
Skip writing SQL:
- ✅ "Show users with expired subscriptions"
- ❌ "Let me write a SELECT query..."

### 3. Let Claude Check Containers
Skip docker commands:
- ✅ "Is postgres healthy?"
- ❌ "docker ps && docker logs postgres"

### 4. Ask for Explanations
Don't guess:
- ✅ "How does terminal sharing work?"
- ❌ *Reads code for 30 minutes*

### 5. Pre-Commit Always
Before every commit:
```
Ask: "Run pre-commit checks"
```

## 🐛 Common Issues

### "MCPs not working"
```bash
# Re-run setup script
.devcontainer/setup-mcps.sh

# Check Node.js
node --version

# Ask Claude
"Why aren't my MCPs working?"
```

### "Can't connect to database"
```bash
# Check if postgres is running
docker ps | grep postgres

# Test connection
psql postgresql://ocf:root@postgres:5432/ocf

# Ask Claude
"Test the postgres connection"
```

### "Docker permission denied"
```bash
# Check user groups
groups  # Should include 'docker'

# If not, rebuild container
Ctrl+Shift+P → "Dev Containers: Rebuild Container"
```

### "Commands not found"
```bash
# Reload VS Code window
Ctrl+Shift+P → "Developer: Reload Window"

# Check if Claude Code is installed
code --list-extensions | grep anthropic
```

## 🚀 Next Steps

### Today
1. ✅ Dev container is running
2. ✅ MCPs are configured
3. ✅ Try your first AI command
4. ✅ Read QUICK_REFERENCE.md

### This Week
1. Use Claude Code on every commit
2. Ask Claude to explain unfamiliar code
3. Use database MCP for queries
4. Read full documentation

### This Month
1. Master all 7 review agents
2. Create a custom command
3. Contribute to documentation
4. Help onboard next developer

## 🤝 Getting Help

### Questions About Project
- Read: `CLAUDE.md`
- Ask Claude: "Explain [feature]"
- Check: `.claude/docs/`

### Questions About Dev Container
- Read: `.devcontainer/README.md`
- Run troubleshooting commands
- Ask Claude: "Help me debug my setup"

### Questions About Agents
- Read: `.claude/AGENTS_GUIDE.md`
- Type: `/` to see commands
- Ask naturally: Claude understands

### Questions About MCPs
- Read: `.claude/MCP_QUICKSTART.md`
- Check: `.claude/MCP_SETUP_GUIDE.md`
- Ask: "What MCPs do you have?"

## 🎉 You're Ready!

**Your setup includes:**
- ✅ Complete development environment
- ✅ AI-powered code review
- ✅ Direct database access
- ✅ Container management
- ✅ Git integration
- ✅ Security scanning
- ✅ Performance analysis
- ✅ Architecture validation

**Just open the project and start coding!**

Everything works out of the box. No manual setup needed. 🚀

---

**First command to try:**

Ask Claude Code: "What can you help me with?"

Then: "Show me all organizations from the database"

**Welcome to the team!** 🎉
