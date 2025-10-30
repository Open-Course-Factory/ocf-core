# OCF Core Slash Commands

This directory contains custom slash commands for OCF Core development. These commands provide **immediate actions** in your main conversation for common workflows.

**Looking for analysis/review tasks?** See `.claude/agents/` for specialized subagents that work independently for complex analysis, debugging, and code review.

## Commands vs Agents

### Use Slash Commands For:
- **Immediate actions** in main conversation
- **Quick scaffolding** (creating entities, migrations)
- **Direct modifications** (refactoring, updating docs)
- **Pre-commit validation** (before committing changes)
- **Quick tests** (running specific test suites)

### Use Agents For:
- **Deep analysis** (PR reviews, architecture reviews)
- **Complex debugging** (test failures, permission issues)
- **Learning** (code patterns, system explanations)
- **Audits** (security scans, performance analysis)

See `.claude/agents/README.md` for available agents.

## Available Commands

### Scaffolding & Creation

**`/new-entity`** - Create new entity with full CRUD
- Scaffolds model, DTOs, registration, tests
- Sets up automatic CRUD endpoints
- Generates Swagger documentation
- **Use when:** Starting a new entity from scratch

### Testing & Validation

**`/test`** - Smart test runner
- Runs appropriate tests based on recent changes
- Detects which modules changed
- Runs targeted test suites
- **Use when:** Quick verification during development

**`/api-test`** - Quick API endpoint testing
- Tests API endpoints with automatic authentication
- Generates sample requests
- Validates responses
- **Use when:** Testing individual endpoints quickly

**`/pre-commit`** - Comprehensive pre-commit validation ⭐
- Runs linting, tests, coverage checks
- Validates patterns and best practices
- Security scanning
- **USE THIS BEFORE EVERY COMMIT**
- **Essential command** - Acts as your quality gate

### Code Quality & Refactoring

**`/refactor`** - Systematic refactoring
- Applies project patterns consistently
- Updates to use utils helpers
- Maintains backward compatibility
- **Use when:** Cleaning up existing code

**`/improve`** - Continuous code improvement
- Suggests automated refactoring
- Identifies pattern violations
- Proposes optimizations
- **Use when:** Seeking improvement opportunities

**`/enforce-patterns`** - Pattern enforcement
- Scans codebase for violations
- Enforces coding standards
- Ensures consistency
- **Use when:** Auditing pattern compliance across codebase

### Database & Infrastructure

**`/migrate`** - Handle database migrations
- Creates migration files
- Applies schema changes
- Handles data migrations safely
- **Use when:** Making database schema changes

**`/update-docs`** - Regenerate API documentation
- Runs swag init with correct flags
- Updates Swagger documentation
- Validates API documentation completeness
- **Use when:** API endpoints change

## Related: Analysis & Review Agents

For deep analysis, debugging, and review tasks, use specialized **agents** (not commands):

**Available in `.claude/agents/`:**
- `review-pr` - Comprehensive PR review with architecture validation
- `review-entity` - Entity implementation completeness audit
- `debug-test` - Systematic test failure debugging
- `security-scan` - Security vulnerability scanning
- `architecture-review` - Architecture and scalability assessment
- `performance-audit` - Performance bottleneck analysis
- `explain` - Deep system explanations with diagrams
- `find-pattern` - Code pattern examples and guidance
- `check-permissions` - Permission system debugging

**Agents work differently:**
- Agents have **independent context windows** for complex analysis
- Commands execute **immediate actions** in your main conversation
- Agents are better for **learning and investigation**
- Commands are better for **doing and modifying**

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

# Pre-commit validation (run before every commit!)
/pre-commit

# Refactor code to use project patterns
/refactor
# Then: "Update permission management to use utils helpers"

# Systematic pattern enforcement
/enforce-patterns
# Then: "Check for direct Casbin calls"
```

## How Commands Work

Each command is a markdown file with:
1. YAML frontmatter with description and tags
2. Detailed instructions that guide the workflow
3. Best practices and patterns to follow
4. Examples and common use cases

Commands are loaded when you type `/command-name` and expand into the conversation.

## Creating Your Own Commands

Create a new `.md` file in `.claude/commands/`:

```markdown
---
description: Short description of what this command does
tags: [tag1, tag2]
---

# Command Name

Detailed instructions for what to do when this command runs...

## Steps

1. First step...
2. Second step...
3. etc.
```

Then use it: `/your-command-name`

## Best Practices

1. **Use `/pre-commit` before every commit** - Your quality gate
2. **Use commands for actions** - Quick scaffolding, refactoring, testing
3. **Use agents for analysis** - Reviews, debugging, learning
4. **Customize commands** - Edit the `.md` files to fit your workflow
5. **Chain commands** - Use multiple commands in sequence for complex workflows

## Related Documentation

- `.claude/agents/` - Analysis and review agents
- `.claude/docs/` - Project-specific documentation
- `CLAUDE.md` - Complete project guide for Claude Code
- `REVIEW_AGENTS_README.md` - Detailed agent documentation (legacy)
