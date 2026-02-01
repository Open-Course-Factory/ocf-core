#!/bin/bash
# Setup script for GitLab CLI (glab)

# Check if GITLAB_TOKEN environment variable is set
if [ -z "$GITLAB_TOKEN" ]; then
    echo "GITLAB_TOKEN environment variable is not set."
    echo "Please run: export GITLAB_TOKEN=your_token_here"
    echo "Or pass the token as an argument: ./setup-glab.sh <your_token>"

    # If token is passed as argument, use it
    if [ -n "$1" ]; then
        GITLAB_TOKEN="$1"
    else
        exit 1
    fi
fi

# Configure glab with the custom GitLab instance
echo "Configuring glab for usine.solution-libre.fr..."
echo "$GITLAB_TOKEN" | glab auth login --hostname usine.solution-libre.fr --stdin

# Set the default host
glab config set -g host usine.solution-libre.fr

# Verify authentication
echo ""
echo "Verifying authentication..."
glab auth status

echo ""
echo "glab configuration complete!"
