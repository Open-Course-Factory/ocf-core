# 🔌 MCP Setup Guide - OCF Core

Model Context Protocol (MCP) servers extend Claude Code's capabilities with specialized tools.

## 🎯 Recommended MCPs for OCF Core

### **Priority 1: Essential MCPs**

#### 1. **PostgreSQL MCP** ⭐⭐⭐
**Why:** Direct database queries, inspect data, debug issues

**Installation:**
```bash
npm install -g @modelcontextprotocol/server-postgres
```

**Configuration:** Add to Claude Code settings
```json
{
  "mcpServers": {
    "postgres": {
      "command": "mcp-server-postgres",
      "args": ["postgresql://user:password@postgres:5432/ocf"]
    }
  }
}
```

**What You Can Do:**
- Query database directly: "Show me all users with role administrator"
- Inspect tables: "Show the schema for organizations table"
- Debug data: "Find all subscriptions that expired last week"
- Analyze relationships: "Show all groups belonging to organization X"

---

#### 2. **Docker MCP** ⭐⭐⭐
**Why:** Manage containers, check logs, restart services

**Installation:**
```bash
npm install -g @modelcontextprotocol/server-docker
```

**Configuration:**
```json
{
  "mcpServers": {
    "docker": {
      "command": "mcp-server-docker"
    }
  }
}
```

**What You Can Do:**
- Check container status: "Are all services running?"
- View logs: "Show postgres logs from last 10 minutes"
- Restart services: "Restart the casdoor container"
- Inspect networks: "Show docker network connections"

---

#### 3. **Git MCP** ⭐⭐
**Why:** Advanced git operations, history analysis

**Installation:**
```bash
npm install -g @modelcontextprotocol/server-git
```

**Configuration:**
```json
{
  "mcpServers": {
    "git": {
      "command": "mcp-server-git",
      "args": ["/workspaces/ocf-core"]
    }
  }
}
```

**What You Can Do:**
- Analyze history: "Show commits that touched payment code"
- Find authors: "Who wrote the terminal sharing feature?"
- Compare branches: "What's different between main and feature-branch?"
- Search commits: "Find commits mentioning security fixes"

---

### **Priority 2: Productivity MCPs**

#### 4. **File System MCP** ⭐⭐
**Why:** Enhanced file operations, search, bulk operations

**Installation:**
```bash
npm install -g @modelcontextprotocol/server-filesystem
```

**Configuration:**
```json
{
  "mcpServers": {
    "filesystem": {
      "command": "mcp-server-filesystem",
      "args": ["/workspaces/ocf-core"]
    }
  }
}
```

**What You Can Do:**
- Find files: "Find all files modified in last 24 hours"
- Bulk operations: "Rename all *_test.go files to *Test.go"
- Directory operations: "Create test fixtures directory structure"

---

#### 5. **SQLite MCP** ⭐
**Why:** Query test databases, inspect SQLite data

**Installation:**
```bash
npm install -g @modelcontextprotocol/server-sqlite
```

**Configuration:**
```json
{
  "mcpServers": {
    "sqlite": {
      "command": "mcp-server-sqlite",
      "args": ["file::memory:?cache=shared"]
    }
  }
}
```

**What You Can Do:**
- Inspect test data: "Show data in test database"
- Debug tests: "Query the users table from test run"

---

### **Priority 3: API & Integration MCPs**

#### 6. **Stripe MCP** ⭐⭐
**Why:** Test payments, inspect subscriptions, debug webhooks

**Note:** May need custom implementation

**What You'd Want:**
- "Show all test subscriptions"
- "Create a test customer"
- "Trigger a webhook event"
- "Check subscription status for user X"

---

#### 7. **HTTP/REST MCP** ⭐
**Why:** Test API endpoints, mock responses

**Installation:**
```bash
npm install -g @modelcontextprotocol/server-fetch
```

**Configuration:**
```json
{
  "mcpServers": {
    "fetch": {
      "command": "mcp-server-fetch"
    }
  }
}
```

**What You Can Do:**
- Test endpoints: "Call the organizations API"
- Mock responses: "Create mock subscription data"
- Debug webhooks: "Send test webhook payload"

---

## 🚀 Quick Setup (All Essential MCPs)

### Step 1: Install MCPs
```bash
# Install all essential MCPs at once
npm install -g \
  @modelcontextprotocol/server-postgres \
  @modelcontextprotocol/server-docker \
  @modelcontextprotocol/server-git \
  @modelcontextprotocol/server-filesystem \
  @modelcontextprotocol/server-fetch
```

### Step 2: Configure Claude Code

Create or update your Claude Code configuration file:

**Location:** `~/.config/claude-code/config.json` (Linux/Mac)
or `%APPDATA%/claude-code/config.json` (Windows)

```json
{
  "mcpServers": {
    "postgres": {
      "command": "mcp-server-postgres",
      "args": ["postgresql://user:yourpassword@postgres:5432/ocf"],
      "env": {
        "PGPASSWORD": "yourpassword"
      }
    },
    "docker": {
      "command": "mcp-server-docker"
    },
    "git": {
      "command": "mcp-server-git",
      "args": ["/workspaces/ocf-core"]
    },
    "filesystem": {
      "command": "mcp-server-filesystem",
      "args": ["/workspaces/ocf-core"],
      "permissions": {
        "read": true,
        "write": true
      }
    },
    "fetch": {
      "command": "mcp-server-fetch",
      "allowedDomains": [
        "localhost",
        "127.0.0.1",
        "api.stripe.com"
      ]
    }
  }
}
```

### Step 3: Restart Claude Code

After configuration, restart Claude Code to load the MCPs.

### Step 4: Verify MCPs Loaded

Ask me: "What MCPs do you have access to?"

---

## 💡 Usage Examples

### **Example 1: Database Debugging**
```
You: "Show me all organizations with more than 10 members"
Me: [Uses postgres MCP to query directly]
```

### **Example 2: Container Management**
```
You: "Check if postgres is running and show recent logs"
Me: [Uses docker MCP to check status and fetch logs]
```

### **Example 3: Git History**
```
You: "Find all commits that modified the payment service"
Me: [Uses git MCP to search history]
```

### **Example 4: API Testing**
```
You: "Test the organizations endpoint with authentication"
Me: [Uses fetch MCP to make authenticated API call]
```

### **Example 5: File Operations**
```
You: "Find all DTO files missing mapstructure tags"
Me: [Uses filesystem MCP to search and read files]
```

---

## 🎯 MCP-Enhanced Agent Commands

Once MCPs are installed, create enhanced versions of commands:

### `/db-query` Command
```markdown
---
description: Query the database directly using PostgreSQL MCP
---

# Database Query Command

Use the PostgreSQL MCP to query the database directly.

Ask me: "Show me all users", "Find expired subscriptions", etc.
```

### `/container-status` Command
```markdown
---
description: Check Docker container status and logs
---

# Container Status Command

Use the Docker MCP to check services and logs.

Ask me: "Show all containers", "Check postgres logs", etc.
```

### `/git-history` Command
```markdown
---
description: Analyze git history and find changes
---

# Git History Command

Use the Git MCP to analyze repository history.

Ask me: "Who wrote this file?", "Find payment-related commits", etc.
```

---

## 🔧 Advanced Configuration

### Environment-Specific Config

**Development:**
```json
{
  "postgres": {
    "args": ["postgresql://user:password@postgres:5432/ocf"]
  }
}
```

**Testing:**
```json
{
  "postgres": {
    "args": ["postgresql://user:password@postgres:5432/ocf_test"]
  }
}
```

### Security Best Practices

1. **Never commit MCP credentials** to git
2. **Use environment variables** for sensitive data:
```json
{
  "postgres": {
    "command": "mcp-server-postgres",
    "args": ["${DATABASE_URL}"]
  }
}
```

3. **Restrict permissions** on filesystem MCP:
```json
{
  "filesystem": {
    "args": ["/workspaces/ocf-core"],
    "permissions": {
      "read": true,
      "write": false  // Read-only for safety
    }
  }
}
```

---

## 🐛 Troubleshooting

### MCP Not Loading
```bash
# Check if MCP is installed
which mcp-server-postgres

# Check Claude Code logs
# Look for MCP initialization errors
```

### Connection Issues
```bash
# Test postgres connection manually
psql postgresql://user:password@postgres:5432/ocf

# Test docker connection
docker ps
```

### Permission Errors
- Ensure filesystem MCP has read/write permissions
- Check Docker socket access
- Verify database credentials

---

## 📊 Before & After MCPs

### Without MCPs:
```
You: "Show me expired subscriptions"
Me: [Writes psql command] → You run it → You paste results → I analyze
Time: 2-3 minutes
```

### With MCPs:
```
You: "Show me expired subscriptions"
Me: [Uses postgres MCP] → Immediate results with analysis
Time: 10 seconds
```

---

## 🚀 Next Steps

1. **Install essential MCPs** (postgres, docker, git)
2. **Configure Claude Code** with MCP settings
3. **Restart Claude Code**
4. **Test with:** "What MCPs do you have?"
5. **Try examples** from this guide

---

## 📚 Resources

- **MCP Documentation:** https://modelcontextprotocol.io
- **Available MCPs:** https://github.com/modelcontextprotocol/servers
- **Create Custom MCPs:** See MCP SDK documentation

---

**Ready to supercharge your development? Install MCPs now!** 🚀
