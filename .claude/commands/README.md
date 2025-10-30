# OCF Core Custom Commands

Custom Claude Code commands for efficient development workflow.

## 📚 Command Categories

### 🏗️ Development & Scaffolding

- `/new-entity` - Create a complete entity (model, DTOs, registration, tests)
- `/migrate` - Handle database migrations and schema changes
- `/update-docs` - Regenerate Swagger API documentation

### 🧪 Testing & Debugging

- `/test` - Smart test runner based on recent changes
- `/debug-test` - Debug a failing test systematically
- `/api-test` - Quick API endpoint testing with auto-auth

### 🔍 Analysis & Understanding

- `/find-pattern` - Find implementation examples in the codebase
- `/explain` - Deep explanation of systems and features
- `/check-permissions` - Debug Casbin permission system

### 🤖 Code Review & Quality Agents

**[See full documentation: REVIEW_AGENTS_README.md](REVIEW_AGENTS_README.md)**

- `/review-pr` - Comprehensive PR review (like a senior engineer)
- `/enforce-patterns` - Scan and fix pattern violations (10+ patterns)
- `/security-scan` - Security vulnerability audit
- `/performance-audit` - Performance analysis and optimization
- `/architecture-review` - Architecture validation and scalability
- `/pre-commit` - Pre-commit quality gate (run before every commit!)
- `/improve` - Continuous code improvement suggestions

### 🔧 Code Maintenance

- `/refactor` - Systematic refactoring with pattern consistency
- `/review-entity` - Review entity implementation for best practices

## Usage Examples

```bash
# Create a new entity
/new-entity
# Then: "Create a License entity with fields: user_id, plan_id, key, expires_at"

# Run appropriate tests
/test
# Auto-detects what changed and runs relevant tests

# Test an API endpoint
/api-test
# Then: "Test GET /api/v1/organizations with authentication"

# Find a pattern
/find-pattern
# Then: "How to implement a service with validation"

# Debug permissions
/check-permissions
# Then: "Check permissions for user 123e4567-..."

# Review entity quality
/review-entity
# Then: "Review the Organization entity"
```

## How Commands Work

Each command is a specialized prompt that:
1. Guides me through a specific workflow
2. Enforces best practices
3. Uses the right tools automatically
4. Provides systematic, consistent results

Commands are just markdown files in `.claude/commands/` - you can edit them to customize behavior!

## Creating Your Own Commands

Create a new `.md` file in `.claude/commands/`:

```markdown
---
description: Short description of what this command does
tags: [tag1, tag2]
---

# Command Name

Detailed instructions for what I should do when this command runs...
```

Then use it: `/your-command-name`
