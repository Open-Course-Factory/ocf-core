#!/bin/bash

echo "🔌 Installing MCPs for OCF Core Development"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Check if npm is installed
if ! command -v npm &> /dev/null; then
    echo "📦 Installing Node.js and npm..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt-get install -y nodejs
fi

echo ""
echo "✅ Node.js version: $(node --version)"
echo "✅ npm version: $(npm --version)"
echo ""

# Install MCPs globally
echo "📥 Installing MCP servers..."
echo ""

echo "1️⃣  Installing PostgreSQL MCP..."
npm install -g @modelcontextprotocol/server-postgres || echo "⚠️  PostgreSQL MCP installation failed (might not be available yet)"

echo ""
echo "2️⃣  Installing Docker MCP..."
npm install -g @modelcontextprotocol/server-docker || echo "⚠️  Docker MCP installation failed (might not be available yet)"

echo ""
echo "3️⃣  Installing Git MCP..."
npm install -g @modelcontextprotocol/server-git || echo "⚠️  Git MCP installation failed (might not be available yet)"

echo ""
echo "4️⃣  Installing Filesystem MCP..."
npm install -g @modelcontextprotocol/server-filesystem || echo "⚠️  Filesystem MCP installation failed (might not be available yet)"

echo ""
echo "5️⃣  Installing Fetch MCP..."
npm install -g @modelcontextprotocol/server-fetch || echo "⚠️  Fetch MCP installation failed (might not be available yet)"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ MCP Installation Complete!"
echo ""
echo "📋 Installed MCPs:"
ls -1 $(npm root -g)/@modelcontextprotocol/ 2>/dev/null || echo "No MCPs installed yet (they may not be published)"
echo ""
echo "🔧 Next Steps:"
echo "1. Create Claude Code config file"
echo "2. Add MCP servers to configuration"
echo "3. Restart Claude Code"
echo ""
echo "📖 See .claude/MCP_SETUP_GUIDE.md for configuration details"
