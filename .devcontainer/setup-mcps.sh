#!/bin/bash
set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔌 Setting up MCPs for OCF Core Development"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Check Node.js installation
if ! command -v node &> /dev/null; then
    echo "❌ Node.js is not installed!"
    exit 1
fi

echo "✅ Node.js $(node --version)"
echo "✅ npm $(npm --version)"
echo ""

# Create npm global directory for user (avoid permission issues)
mkdir -p ~/.npm-global
npm config set prefix ~/.npm-global
export PATH=~/.npm-global/bin:$PATH

# Add to bashrc if not already there
if ! grep -q "npm-global/bin" ~/.bashrc; then
    echo 'export PATH=~/.npm-global/bin:$PATH' >> ~/.bashrc
fi

echo "📥 Installing MCP servers..."
echo ""

# Note: As of now, official MCP packages might not be published yet
# We'll use npx to run them, which will download on-demand
# This section will install them if/when they become available

# Try to install MCPs (fail silently if not available)
echo "1️⃣  Checking PostgreSQL MCP..."
npm list -g @modelcontextprotocol/server-postgres 2>/dev/null || \
    npm install -g @modelcontextprotocol/server-postgres 2>/dev/null || \
    echo "   ⚠️  Not available yet (will use npx on-demand)"

echo ""
echo "2️⃣  Checking Docker MCP..."
npm list -g @modelcontextprotocol/server-docker 2>/dev/null || \
    npm install -g @modelcontextprotocol/server-docker 2>/dev/null || \
    echo "   ⚠️  Not available yet (will use npx on-demand)"

echo ""
echo "3️⃣  Checking Git MCP..."
npm list -g @modelcontextprotocol/server-git 2>/dev/null || \
    npm install -g @modelcontextprotocol/server-git 2>/dev/null || \
    echo "   ⚠️  Not available yet (will use npx on-demand)"

echo ""
echo "4️⃣  Checking Filesystem MCP..."
npm list -g @modelcontextprotocol/server-filesystem 2>/dev/null || \
    npm install -g @modelcontextprotocol/server-filesystem 2>/dev/null || \
    echo "   ⚠️  Not available yet (will use npx on-demand)"

echo ""
echo "5️⃣  Checking Fetch MCP..."
npm list -g @modelcontextprotocol/server-fetch 2>/dev/null || \
    npm install -g @modelcontextprotocol/server-fetch 2>/dev/null || \
    echo "   ⚠️  Not available yet (will use npx on-demand)"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ MCP Setup Complete!"
echo ""
echo "📋 Available MCPs:"
echo "   • PostgreSQL - Direct database queries"
echo "   • Docker - Container management"
echo "   • Git - Repository analysis"
echo "   • Filesystem - Enhanced file operations"
echo "   • Fetch - HTTP requests"
echo ""
echo "🔧 Configuration:"
echo "   • MCPs will be used via 'npx' (auto-download)"
echo "   • Claude Code settings configured automatically"
echo "   • Database: postgresql://ocf:root@postgres:5432/ocf"
echo ""
echo "📖 Documentation:"
echo "   • .claude/MCP_QUICKSTART.md"
echo "   • .claude/MCP_SETUP_GUIDE.md"
echo ""
echo "✨ Try asking Claude:"
echo '   "What MCPs do you have access to?"'
echo '   "Show me all organizations from the database"'
echo '   "Check if all containers are running"'
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
