# 🚀 OCF Core - Agents & Automation Guide

This guide shows you how to leverage Claude Code's agents and custom commands to maximize productivity and code quality.

## 📊 Quick Stats

**Total Commands:** 20
**Categories:** 5
**Review Agents:** 7
**Est. Time Saved:** 10-15 hours/week

## 🎯 The Complete Workflow

### Level 1: Daily Development (Fast)

```mermaid
graph LR
    A[Start Coding] --> B[Make Changes]
    B --> C[/pre-commit]
    C --> D{Pass?}
    D -->|Yes| E[Commit]
    D -->|No| F[Fix Issues]
    F --> C
```

**Commands:**
- `/pre-commit` - Before every commit (45 seconds)
- `/test` - When you change code (30-60 seconds)
- `/api-test` - Quick endpoint testing (10 seconds)

**Goal:** Catch issues immediately, keep code clean

---

### Level 2: Feature Complete (Medium)

```mermaid
graph TD
    A[Feature Done] --> B[/review-pr]
    B --> C[/test]
    C --> D[/update-docs]
    D --> E[/security-scan]
    E --> F{All Clear?}
    F -->|Yes| G[Push to PR]
    F -->|No| H[Fix Issues]
    H --> B
```

**Commands:**
- `/review-pr` - Comprehensive review (2-3 minutes)
- `/test` - Full test suite
- `/update-docs` - Regenerate Swagger
- `/security-scan` - Quick security check

**Goal:** Ship high-quality, secure features

---

### Level 3: Weekly Maintenance (Proactive)

```mermaid
graph TD
    A[Monday Morning] --> B[/enforce-patterns]
    B --> C{Violations?}
    C -->|Yes| D[Fix Top 5]
    D --> E[/improve]
    C -->|No| E
    E --> F{Opportunities?}
    F -->|Yes| G[Apply Top 3]
    G --> H[/test]
    F -->|No| I[Done]
    H --> I
```

**Commands:**
- `/enforce-patterns` - Pattern compliance scan
- `/improve` - Find improvement opportunities
- `/refactor` - Systematic code improvements
- `/architecture-review` - Structure validation

**Goal:** Continuous code quality improvement

---

### Level 4: Release Preparation (Thorough)

```mermaid
graph TD
    A[Pre-Release] --> B[/review-pr]
    B --> C[/security-scan]
    C --> D[/performance-audit]
    D --> E[/architecture-review]
    E --> F[/test Full Suite]
    F --> G{All Pass?}
    G -->|Yes| H[Release!]
    G -->|No| I[Fix Critical]
    I --> B
```

**Commands:**
- `/review-pr` - Full code review
- `/security-scan` - Comprehensive security audit
- `/performance-audit` - Performance validation
- `/architecture-review` - Scalability check

**Goal:** Ship with confidence

---

## 🎓 Agent Mastery Levels

### 🥉 Bronze: Getting Started

**Week 1 Goals:**
- [ ] Run `/pre-commit` before every commit
- [ ] Use `/test` when code changes
- [ ] Try `/api-test` for manual testing
- [ ] Read through available commands

**Commands to Master:**
1. `/pre-commit` - Make it a habit
2. `/test` - Run often
3. `/api-test` - Quick validation

**Time Investment:** 5 extra minutes per day
**Benefit:** Catch 80% of issues before PR

---

### 🥈 Silver: Productive Developer

**Week 2-4 Goals:**
- [ ] Use `/review-pr` before pushing
- [ ] Run `/new-entity` for new entities
- [ ] Use `/find-pattern` to learn patterns
- [ ] Check `/update-docs` after API changes

**Commands to Master:**
4. `/review-pr` - Before every PR
5. `/new-entity` - Scaffold entities
6. `/find-pattern` - Learn by example
7. `/update-docs` - Keep docs current

**Time Investment:** 10 minutes per feature
**Benefit:** Ship features 2x faster with higher quality

---

### 🥇 Gold: Quality Champion

**Month 2 Goals:**
- [ ] Weekly `/enforce-patterns` runs
- [ ] Use `/security-scan` proactively
- [ ] Run `/improve` monthly
- [ ] Create custom commands

**Commands to Master:**
8. `/enforce-patterns` - Weekly consistency
9. `/security-scan` - Proactive security
10. `/improve` - Continuous improvement
11. `/refactor` - Systematic cleanup

**Time Investment:** 1 hour per week on quality
**Benefit:** 90%+ code quality, minimal tech debt

---

### 💎 Platinum: Agent Expert

**Quarter 1 Goals:**
- [ ] Use all 7 review agents regularly
- [ ] Customize agents for your workflow
- [ ] Run `/architecture-review` quarterly
- [ ] Mentor team on agent usage

**Commands to Master:**
12. `/performance-audit` - Optimize proactively
13. `/architecture-review` - Big picture validation
14. Custom commands - Your own agents

**Time Investment:** Built into workflow
**Benefit:** World-class code quality, team productivity

---

## 🔥 Power User Workflows

### Workflow 1: New Feature Flow

```bash
# 1. Understand existing patterns
/find-pattern
→ "Show me how to implement a service with validation"

# 2. Create entity if needed
/new-entity
→ "Create License entity with fields: user_id, key, expires_at"

# 3. Develop feature
# ... write code ...

# 4. Test as you go
/api-test
→ "Test POST /api/v1/licenses with auth"

# 5. Pre-commit check
/pre-commit

# 6. Comprehensive review
/review-pr

# 7. Push!
git push
```

**Time:** Feature development + 5 minutes
**Quality:** 95%+ on first review

---

### Workflow 2: Bug Fix Flow

```bash
# 1. Understand the system
/explain
→ "How does terminal sharing work?"

# 2. Find similar implementations
/find-pattern
→ "Show me permission handling patterns"

# 3. Fix the bug
# ... write fix ...

# 4. Test the fix
/debug-test
→ "Run TestTerminalSharing"

# 5. Check for regressions
/test

# 6. Security check (if security-related)
/security-scan
→ "Check terminal permission security"

# 7. Pre-commit
/pre-commit

# 8. Done!
```

**Time:** Bug fix + 3 minutes
**Confidence:** High (tested, validated, secure)

---

### Workflow 3: Refactoring Flow

```bash
# 1. Identify violations
/enforce-patterns
→ "Run full pattern scan"

# 2. Plan refactoring
/refactor
→ "Fix error handling pattern violations"

# 3. Review architecture
/architecture-review
→ "Check for architectural issues"

# 4. Apply improvements
# ... refactor code ...

# 5. Validate no regressions
/test

# 6. Measure improvement
/improve
→ "Scan codebase for improvement opportunities"
```

**Time:** Refactoring + 10 minutes
**Impact:** Measurable quality improvement

---

### Workflow 4: Security Hardening Flow

```bash
# 1. Full security audit
/security-scan

# 2. Fix critical issues
# ... apply fixes ...

# 3. Check permissions specifically
/check-permissions
→ "Audit all permission grants"

# 4. Review architecture
/architecture-review
→ "Review security architecture"

# 5. Test thoroughly
/test

# 6. Re-scan to verify
/security-scan

# 7. Document findings
# ... update security docs ...
```

**Time:** 2-3 hours (comprehensive)
**Result:** Production-ready security

---

## 💡 Pro Tips

### Tip 1: Parallel Agent Execution
```bash
# Run multiple agents at once
You: "Full quality check"
Me: [Spawns /review-pr + /security-scan + /performance-audit in parallel]
```

**Benefit:** 3x faster than sequential

---

### Tip 2: Context-Aware Agent Usage
```bash
# Just describe what you did
You: "I added payment webhooks"
Me: [Automatically suggests /security-scan + /test + /review-pr]
```

**Benefit:** Right checks at the right time

---

### Tip 3: Agent Chaining
```bash
# One command triggers others
/pre-commit → Detects violations → Auto-runs /refactor → Applies fixes → Re-runs /test
```

**Benefit:** Automated quality loops

---

### Tip 4: Custom Agent Creation
```bash
# Create project-specific agents
nano .claude/commands/deploy-check.md

# Use it!
/deploy-check
```

**Benefit:** Tailored to your workflow

---

### Tip 5: Agent Learning
```bash
# Agents learn your patterns
/enforce-patterns (Week 1) → 45 violations
/enforce-patterns (Week 4) → 5 violations

# Agents adapt to your style
```

**Benefit:** Increasingly relevant feedback

---

## 📈 Measuring Success

### Code Quality Metrics

**Track these with agents:**

```
Week 1 Baseline:
- Pattern Compliance: 65%
- Test Coverage: 75%
- Security Score: 70/100
- Performance Score: 60/100

Week 8 Target:
- Pattern Compliance: 95%
- Test Coverage: 90%
- Security Score: 95/100
- Performance Score: 85/100
```

**How to track:**
```bash
# Weekly quality report
/enforce-patterns → Note compliance %
/test → Check coverage %
/security-scan → Note security score
/performance-audit → Note performance score
```

---

### Time Savings Estimate

**Without Agents:**
- Manual code review: 30 min
- Finding patterns: 20 min
- Security checks: 30 min
- Performance testing: 20 min
- Test debugging: 40 min
- Documentation: 15 min
**Total:** ~2.5 hours per feature

**With Agents:**
- `/review-pr`: 3 min
- `/find-pattern`: 1 min
- `/security-scan`: 2 min
- `/performance-audit`: 2 min
- `/test`: 1 min
- `/update-docs`: 30 sec
**Total:** ~10 minutes per feature

**Savings:** 2+ hours per feature × 5 features/week = **10 hours/week**

---

## 🎯 30-Day Agent Adoption Plan

### Week 1: Foundation
- [ ] Day 1: Read this guide
- [ ] Day 2: Try `/pre-commit` on every commit
- [ ] Day 3: Use `/test` and `/api-test`
- [ ] Day 4: Run `/review-pr` on a feature
- [ ] Day 5: Experiment with `/find-pattern`

**Goal:** Build muscle memory for basic agents

---

### Week 2: Expansion
- [ ] Day 8: Try `/new-entity` for a new feature
- [ ] Day 9: Run `/security-scan` on your module
- [ ] Day 10: Use `/debug-test` to fix a failing test
- [ ] Day 11: Check `/update-docs` workflow
- [ ] Day 12: Review with `/review-entity`

**Goal:** Expand agent usage to more scenarios

---

### Week 3: Quality Focus
- [ ] Day 15: Run `/enforce-patterns` on codebase
- [ ] Day 16: Fix top 10 pattern violations
- [ ] Day 17: Run `/improve` and apply 3 improvements
- [ ] Day 18: Use `/refactor` for systematic cleanup
- [ ] Day 19: Validate with `/architecture-review`

**Goal:** Measurable quality improvement

---

### Week 4: Mastery
- [ ] Day 22: Full `/performance-audit`
- [ ] Day 23: Optimize top bottlenecks
- [ ] Day 24: Complete `/security-scan` audit
- [ ] Day 25: Create your first custom command
- [ ] Day 26: Run full release workflow

**Goal:** Agent expert status

---

## 🚀 Next Steps

1. **Start today:** Run `/pre-commit` on your next commit
2. **Read the agents guide:** `.claude/commands/REVIEW_AGENTS_README.md`
3. **Pick your level:** Bronze, Silver, Gold, or Platinum
4. **Track progress:** Weekly quality metrics
5. **Share learnings:** Help your team adopt agents

---

## 📚 Resources

- **Command Reference:** `.claude/commands/README.md`
- **Review Agents:** `.claude/commands/REVIEW_AGENTS_README.md`
- **Project Docs:** `.claude/docs/`
- **Main Project Guide:** `CLAUDE.md`

---

**Ready to become an Agent Power User? Start with `/pre-commit` right now! 🚀**
