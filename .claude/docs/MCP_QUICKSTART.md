# 🚀 MCP Quick Start - Get MCPs Running in 5 Minutes

## What Are MCPs?

**Model Context Protocol (MCP)** servers give me superpowers by connecting me directly to:
- **Databases** - Query PostgreSQL directly
- **Docker** - Check containers, view logs
- **Git** - Search history, analyze changes
- **APIs** - Test endpoints, make requests

## ⚡ Quick Install

### Option 1: Automatic Installation (Recommended)

```bash
# Run the install script
bash .claude/install-mcps.sh
```

This will:
- Install Node.js (if needed)
- Install all essential MCPs
- Show you what's available

### Option 2: Manual Installation

```bash
# Install Node.js first (if not already installed)
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs

# Install MCPs one by one
npm install -g @modelcontextprotocol/server-postgres
npm install -g @modelcontextprotocol/server-docker
npm install -g @modelcontextprotocol/server-git
```

## 🔧 Configuration

### Your Database Connection
Based on your `.env` file:
```
postgresql://ocf:root@postgres:5432/ocf
```

### Claude Code Configuration

**Location:** Add to your Claude Code settings

You have two options:

#### Option A: VS Code Settings (Recommended if using VS Code)
1. Open VS Code settings (Cmd/Ctrl + ,)
2. Search for "Claude Code MCP"
3. Add server configurations

#### Option B: Configuration File
Create: `~/.config/claude-code/config.json` (or equivalent on your OS)

Use the template in `.claude/mcp-config.json`:
```bash
# Copy the template
cp .claude/mcp-config.json ~/.config/claude-code/config.json
```

## 🎯 What You Can Do With MCPs

### PostgreSQL MCP

**Without MCP:**
```
You: "Show me all organizations"
Me: "Here's a psql command to run..."
You: [Run command, paste output]
Me: [Analyze results]
```

**With MCP:**
```
You: "Show me all organizations"
Me: [Direct query] "Found 15 organizations: ..."
```

**Examples:**
- "Show all users with role administrator"
- "Find organizations with more than 10 members"
- "List all active subscriptions"
- "Show the schema for the groups table"
- "Find expired terminal sessions"

### Docker MCP

**Examples:**
- "Show all running containers"
- "Check postgres logs from last 5 minutes"
- "Is the casdoor service healthy?"
- "Show container resource usage"
- "Restart the database container"

### Git MCP

**Examples:**
- "Who wrote the payment service?"
- "Find all commits mentioning security"
- "Show changes to entity management system"
- "Compare current branch to main"
- "Find when terminal sharing was added"

### Filesystem MCP

**Examples:**
- "Find all files with TODO comments"
- "List all DTO files"
- "Show files modified in last 24 hours"
- "Find large files (> 1MB)"

### Fetch MCP

**Examples:**
- "Test the /api/v1/organizations endpoint"
- "Call Stripe API to list products"
- "Make authenticated request to local API"

## ✅ Verify MCPs Are Working

After configuration, ask me:

```
"What MCPs do you have access to?"
```

I should respond with a list of available MCPs.

## 🎓 Usage Patterns

### Pattern 1: Database Debugging
```
You: "Why is user X not able to access organization Y?"
Me: [Uses postgres MCP]
    - Queries user permissions
    - Checks organization membership
    - Verifies group assignments
    - Provides diagnosis
```

### Pattern 2: Container Troubleshooting
```
You: "The API is returning 500 errors"
Me: [Uses docker MCP]
    - Checks if all containers are running
    - Views recent logs
    - Identifies the issue
```

### Pattern 3: Code History Analysis
```
You: "How did the subscription system evolve?"
Me: [Uses git MCP]
    - Searches commit history
    - Finds relevant changes
    - Shows timeline
```

## 🔒 Security Notes

1. **Database credentials** in MCP config - Keep them secure!
2. **Don't commit** `~/.config/claude-code/config.json` to git
3. **Use environment variables** for production
4. **Limit permissions** on filesystem MCP if needed

## 🐛 Troubleshooting

### "MCP not found"
```bash
# Check if MCP is installed
npm list -g | grep @modelcontextprotocol

# Reinstall if needed
npm install -g @modelcontextprotocol/server-postgres
```

### "Connection failed" (Database)
```bash
# Test connection manually
psql postgresql://ocf:root@postgres:5432/ocf

# Check if postgres container is running
docker ps | grep postgres
```

### "Permission denied" (Docker)
```bash
# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker
```

## 📊 Impact

### Before MCPs:
- Database queries: Manual psql → Copy/paste → 2-3 minutes
- Container checks: Manual docker commands → 1-2 minutes
- Git history: Manual git log → grep → 2-3 minutes

### After MCPs:
- Database queries: Instant (5-10 seconds)
- Container checks: Instant (5 seconds)
- Git history: Instant (10 seconds)

**Time saved:** 5-10 minutes per debugging session = hours per week!

## 🚀 Next Steps

1. **Install MCPs:** Run `.claude/install-mcps.sh`
2. **Configure:** Copy `.claude/mcp-config.json` to Claude Code config
3. **Restart:** Restart Claude Code
4. **Test:** Ask me "What MCPs do you have?"
5. **Use:** Start asking database questions naturally!

## 💡 Pro Tips

1. **Combine with agents:** Use MCPs inside `/review-pr` or `/debug-test`
2. **Ask naturally:** Just ask questions, I'll use the right MCP
3. **Database queries:** No need to write SQL, just describe what you want
4. **Container ops:** No need to remember docker commands

## 📚 More Information

- Full guide: `.claude/MCP_SETUP_GUIDE.md`
- Official docs: https://modelcontextprotocol.io
- Available MCPs: https://github.com/modelcontextprotocol/servers

---

**Ready to get started?**

```bash
bash .claude/install-mcps.sh
```

Then ask me: "What MCPs do you have access to?" 🚀
