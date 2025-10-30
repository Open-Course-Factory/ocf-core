# OCF Core Agents

This directory contains specialized subagents for analysis, review, and debugging tasks. Unlike slash commands, agents work with independent context windows and can handle complex analysis tasks without cluttering your main conversation.

## Available Agents

### Code Review & Quality

**`review-pr`** - Comprehensive PR review
- Deep code analysis with architecture validation
- Pattern compliance checking
- Security and quality assessment
- Provides actionable feedback with file:line references

**`review-entity`** - Entity implementation review
- Systematic checklist for entity completeness
- Checks models, DTOs, registration, tests
- Identifies missing components
- Ensures best practice compliance

**`security-scan`** - Security vulnerability scanner
- Checks for SQL injection, XSS, secrets exposure
- Validates authentication/authorization
- Reviews cryptography and error handling
- Generates severity-based reports

**`architecture-review`** - Architecture validation
- Validates clean architecture layers
- Identifies circular dependencies
- Assesses scalability patterns
- Evaluates framework readiness

**`performance-audit`** - Performance analysis
- Detects N+1 queries and missing indexes
- Identifies memory leaks and inefficiencies
- Benchmarks critical paths
- Provides optimization recommendations

### Development Support

**`debug-test`** - Test debugging specialist
- Systematic test failure analysis
- Common OCF Core test issue identification
- Proposes and verifies fixes
- Handles race conditions and database issues

**`explain`** - System explainer
- Deep dive into how systems work
- Architecture and data flow documentation
- Visual diagrams and code examples
- Links to related documentation

**`find-pattern`** - Code pattern finder
- Shows implementation examples from codebase
- Explains patterns and when to use them
- Identifies best practices
- Helps learn by example

**`check-permissions`** - Permission debugger
- Analyzes Casbin policies
- Traces permission flow
- Identifies missing/incorrect permissions
- Provides verification steps

## How to Use Agents

### From Claude Code

Agents are invoked automatically when you need deep analysis:
- "Review this PR for quality issues" → Uses `review-pr` agent
- "Why is this test failing?" → Uses `debug-test` agent
- "Explain how terminal sharing works" → Uses `explain` agent

### Manually via Task Tool

You can also explicitly request an agent:
```
Can you use the review-pr agent to analyze this pull request?
```

Or let Claude decide which agent is best for your task.

## Agents vs Slash Commands

### Use Agents For:
- Complex analysis requiring independent context
- Deep debugging and investigation
- Comprehensive reviews and audits
- Learning and exploration
- Multi-step analysis tasks

### Use Slash Commands For:
- Immediate actions in main conversation
- Quick scaffolding (e.g., `/new-entity`)
- Pre-commit validation (e.g., `/pre-commit`)
- Quick tests (e.g., `/test`)
- Direct code modifications (e.g., `/refactor`)

See `.claude/commands/README.md` for available commands.

## Agent Configuration

Each agent has:
- **name**: Unique identifier
- **description**: When to use the agent
- **tools**: Available tools (Read, Grep, Glob, Bash, Task)
- **model**: AI model to use (default: sonnet)

Agents are configured via YAML frontmatter in their markdown files.

## Customization

To modify an agent:
1. Edit the agent's `.md` file
2. Update the system prompt (below YAML frontmatter)
3. Adjust tool permissions if needed
4. Test the agent with example queries

## Best Practices

1. **Use agents for analysis** - Let them work independently
2. **Use commands for actions** - Execute changes in main conversation
3. **Review agent output** - Agents provide recommendations, you decide
4. **Iterate as needed** - Ask agents for clarification or deeper analysis

## Related Documentation

- `.claude/commands/` - Slash commands for immediate actions
- `.claude/docs/` - Project-specific documentation
- `CLAUDE.md` - Complete project guide for Claude Code
