# 🚀 Quick Reference Card - OCF Core Agents

## Essential Commands (Memorize These!)

| Command | When to Use | Time |
|---------|------------|------|
| `/pre-commit` | **Before every commit** | 45s |
| `/review-pr` | Before pushing PR | 2-3min |
| `/test` | After code changes | 30-60s |
| `/security-scan` | Before releases | 1-2min |

## All Commands by Category

### 🏗️ Development (3)
```bash
/new-entity         # Scaffold complete entity
/migrate            # Database migrations
/update-docs        # Regenerate Swagger
```

### 🧪 Testing (3)
```bash
/test               # Smart test runner
/debug-test         # Debug failing tests
/api-test           # Quick endpoint testing
```

### 🔍 Analysis (3)
```bash
/find-pattern       # Find code examples
/explain            # Explain systems
/check-permissions  # Debug permissions
```

### 🤖 Review Agents (7) - THE POWERHOUSES!
```bash
/review-pr          # Full PR review ⭐
/enforce-patterns   # Pattern compliance (10+ patterns)
/security-scan      # Security audit (10 categories)
/performance-audit  # Performance analysis
/architecture-review # Architecture validation
/pre-commit         # Pre-commit gate (7 phases) ⭐
/improve            # Continuous improvement
```

### 🔧 Maintenance (2)
```bash
/refactor           # Systematic refactoring
/review-entity      # Entity quality check
```

## Daily Workflow Cheat Sheet

### Morning Routine
```bash
git pull
/enforce-patterns   # Check for any new violations
```

### During Development
```bash
# Write code...
/api-test          # Test as you go
/test              # Run relevant tests
```

### Before Commit
```bash
/pre-commit        # ALWAYS! ⭐⭐⭐
```

### Before PR
```bash
/review-pr         # Full review
/security-scan     # If security-related
/update-docs       # If API changed
```

### Weekly Maintenance (Fridays)
```bash
/enforce-patterns  # Pattern compliance
/improve           # Find 3 improvements
/refactor          # Apply improvements
```

### Before Release
```bash
/review-pr
/security-scan
/performance-audit
/architecture-review
/test              # Full suite
```

## Common Scenarios

### Scenario: New Feature
```
/find-pattern → /new-entity → CODE → /test → /pre-commit → /review-pr
```

### Scenario: Bug Fix
```
/explain → CODE → /debug-test → /test → /pre-commit
```

### Scenario: Refactoring
```
/enforce-patterns → /refactor → /test → /architecture-review
```

### Scenario: Security Issue
```
/security-scan → FIX → /check-permissions → /test → /security-scan
```

### Scenario: Performance Problem
```
/performance-audit → OPTIMIZE → BENCHMARK → /test
```

## Power User Shortcuts

### Full Quality Check (Run All)
```
Just ask: "Full quality check"
I'll run: /review-pr + /security-scan + /performance-audit in parallel
```

### Context-Aware
```
Just describe: "I added payment processing"
I'll suggest: /security-scan + /review-pr with payment focus
```

### Auto-Fix
```
/enforce-patterns → Shows violations → Ask: "Fix automatically" → Done!
```

## Pattern Enforcement Checklist

The `/enforce-patterns` agent checks for:

✅ Permission management (utils helpers)
✅ Error handling (utils.Err* constructors)
✅ Validation (ChainValidators)
✅ DTO tags (json + mapstructure)
✅ Converter usage (GenericModelToOutput)
✅ Logging (utils.Debug/Info/Warn/Error)
✅ Database (postgres, not localhost)
✅ SQLite (shared cache)
✅ EditDto (pointer fields)
✅ Foreign keys (naming + indexes)

## Security Scan Categories

The `/security-scan` agent checks:

🔒 Authentication & Authorization
🔒 SQL Injection Prevention
🔒 Secrets Management
🔒 Input Validation
🔒 API Security (CORS, rate limiting)
🔒 Cryptography
🔒 Error Information Disclosure
🔒 Dependencies
🔒 Business Logic Security
🔒 Terminal/SSH Security

## Pre-Commit Phases

The `/pre-commit` agent runs:

**Phase 1:** Code Quality (lint, build)
**Phase 2:** Pattern Compliance (quick checks)
**Phase 3:** Testing (intelligent selection)
**Phase 4:** Documentation (Swagger)
**Phase 5:** Security (quick scan)
**Phase 6:** Architecture (layer checks)
**Phase 7:** Git Quality (commit message, file sizes)

## Time-Saving Estimates

| Task | Without Agents | With Agents | Savings |
|------|---------------|-------------|---------|
| Code Review | 30 min | 3 min | 27 min |
| Security Check | 30 min | 2 min | 28 min |
| Find Patterns | 20 min | 1 min | 19 min |
| Test Debug | 40 min | 5 min | 35 min |
| Documentation | 15 min | 30s | 14.5 min |
| **Per Feature** | **2.5 hrs** | **10 min** | **2+ hrs** |
| **Per Week** | **12.5 hrs** | **1 hr** | **10+ hrs** |

## Quality Score Targets

Track weekly with agents:

```
Pattern Compliance:  → 95%
Test Coverage:       → 90%
Security Score:      → 95/100
Performance Score:   → 85/100
```

## Must-Read Documentation

1. **`.claude/AGENTS_GUIDE.md`** - Complete guide (READ THIS FIRST!)
2. **`.claude/commands/REVIEW_AGENTS_README.md`** - Agent details
3. **`.claude/commands/README.md`** - Command reference
4. **`CLAUDE.md`** - Project context

## Emergency Commands

| Problem | Command | Fix |
|---------|---------|-----|
| Tests failing | `/debug-test` | Systematic debug |
| Security issue | `/security-scan` | Full audit |
| Performance slow | `/performance-audit` | Find bottlenecks |
| Pattern violations | `/enforce-patterns` | Auto-fix |
| Architecture mess | `/architecture-review` | Validate structure |

## Golden Rules

1. ⭐ **ALWAYS run `/pre-commit` before committing**
2. ⭐ **ALWAYS run `/review-pr` before pushing PR**
3. ⭐ **Weekly `/enforce-patterns` on Mondays**
4. Monthly `/improve` sessions
5. Pre-release `/security-scan` + `/performance-audit`

## First Steps

1. **Right now:** Try `/pre-commit`
2. **Today:** Read `.claude/AGENTS_GUIDE.md`
3. **This week:** Use agents on every commit
4. **Next week:** Try all 7 review agents
5. **This month:** Reach Gold level mastery

---

**Print this and keep it visible! 📋**

Type `/` in chat to see all available commands!
