# 🤖 Code Review & Quality Agents

Powerful autonomous agents that act as your senior engineering team, ensuring code quality, security, and continuous improvement.

## 🎯 Available Review Agents

### 1. `/review-pr` - Pull Request Reviewer
**Your Senior Code Reviewer**

Comprehensive PR review covering:
- Entity implementation completeness
- Service logic and validation
- Database operations and queries
- Test coverage and quality
- API documentation
- Security vulnerabilities
- Pattern adherence

**Use when:** Ready to merge a feature branch

**Example:**
```
/review-pr
```

**Output:** Detailed review report with:
- ✅ What's done well
- ⚠️ Issues to fix (Critical/Important/Minor)
- 🧪 Test coverage analysis
- 🚀 Recommendations
- ✅ Approval status

---

### 2. `/enforce-patterns` - Pattern Consistency Enforcer
**Your Code Standards Guardian**

Scans for violations of 10+ core patterns:
- Permission management (utils helpers)
- Error handling (utils.Err* constructors)
- Validation (ChainValidators)
- DTO tags (json + mapstructure)
- Converter usage
- Logging patterns
- Database patterns
- And more...

**Use when:** Want to ensure codebase consistency

**Example:**
```
/enforce-patterns
→ "Run full pattern scan"
```

**Output:** Pattern compliance report with:
- Compliance percentage per pattern
- Specific violations with line numbers
- Auto-fix suggestions
- Overall quality score

---

### 3. `/security-scan` - Security Auditor
**Your Security Expert**

Comprehensive security audit covering:
- Authentication & authorization
- SQL injection prevention
- Secrets management
- Input validation
- API security (CORS, rate limiting)
- Cryptography
- Error information disclosure
- Dependency vulnerabilities
- Business logic security
- Terminal/SSH security

**Use when:** Before releases, after security-sensitive changes

**Example:**
```
/security-scan
```

**Output:** Security report with:
- Critical/High/Medium/Low issues
- Exact locations and exploit scenarios
- Fix code for each issue
- Compliance checklist
- Security score

---

### 4. `/performance-audit` - Performance Analyzer
**Your Performance Engineer**

Analyzes:
- N+1 query detection
- Missing database indexes
- Memory usage and leaks
- API response times
- Caching opportunities
- Inefficient algorithms
- Concurrency issues
- External service timeouts

**Use when:** Performance issues, before optimization work

**Example:**
```
/performance-audit
```

**Output:** Performance report with:
- Critical bottlenecks
- Current vs expected metrics
- Optimization potential
- Specific fixes with benchmarks
- Priority ranking by impact

---

### 5. `/architecture-review` - Architecture Validator
**Your Solution Architect**

Reviews:
- Clean architecture layer separation
- Entity management system usage
- Dependency management
- Module organization
- Error handling flow
- Security architecture
- Data flow patterns
- Scalability readiness
- Testing architecture
- Configuration patterns

**Use when:** Major features, before framework migration

**Example:**
```
/architecture-review
→ "Review src/organizations/ architecture"
```

**Output:** Architecture report with:
- Overall architecture score
- Layer violations
- Module dependency graph
- Scalability assessment
- Future recommendations
- Action items

---

### 6. `/pre-commit` - Pre-Commit Gate
**Your Quality Gatekeeper**

Runs before every commit:
- **Phase 1:** Linting & build
- **Phase 2:** Pattern compliance checks
- **Phase 3:** Intelligent test selection
- **Phase 4:** Documentation updates
- **Phase 5:** Security quick scan
- **Phase 6:** Architecture validation
- **Phase 7:** Git quality checks

**Use when:** Before every commit (make it a habit!)

**Example:**
```
/pre-commit
```

**Output:** Pass/fail with:
- Results for each phase
- Violations found
- Time taken
- Ready to commit status

---

### 7. `/improve` - Continuous Improvement Agent
**Your Code Evolution Guide**

Proactively suggests:
- Code duplication elimination
- Function complexity reduction
- Modern Go patterns (generics, context)
- Performance optimizations
- Testing improvements
- Documentation enhancements
- Structural improvements

**Use when:** Weekly/monthly code quality initiatives

**Example:**
```
/improve
→ "Scan codebase for improvement opportunities"
```

**Output:** Improvement report with:
- High/Medium/Low impact opportunities
- Effort estimates
- Specific fixes
- Prioritized roadmap
- Progress tracking

---

## 🔄 Recommended Workflows

### **Daily Development Workflow**
```
1. Make changes
2. /pre-commit             → Quick validation
3. Fix any issues
4. Commit
```

### **Feature Complete Workflow**
```
1. Feature implemented
2. /review-pr              → Comprehensive review
3. /test                   → Run appropriate tests
4. /update-docs            → Regenerate Swagger
5. /security-scan          → Quick security check
6. Fix issues
7. /pre-commit             → Final validation
8. Push to PR
```

### **Weekly Quality Workflow**
```
1. /enforce-patterns       → Check consistency
2. /improve                → Find opportunities
3. Apply top 3 improvements
4. /architecture-review    → Validate structure
5. Document learnings
```

### **Release Preparation Workflow**
```
1. /review-pr              → Full PR review
2. /security-scan          → Comprehensive security
3. /performance-audit      → Check bottlenecks
4. /test                   → Full test suite
5. Fix all critical/high issues
6. /pre-commit             → Final gate
7. Release!
```

### **Refactoring Workflow**
```
1. /enforce-patterns       → Identify violations
2. /improve                → Find duplications
3. /refactor               → Apply systematic fixes
4. /test                   → Ensure no regressions
5. /architecture-review    → Validate improvements
```

---

## 🎓 Best Practices

### **1. Run Pre-Commit Always**
Make it muscle memory:
```bash
# Before every commit
/pre-commit
```

### **2. Review Before PRs**
Don't wait for human reviewers:
```bash
# When feature complete
/review-pr
```

### **3. Weekly Pattern Enforcement**
Keep codebase consistent:
```bash
# Every Monday
/enforce-patterns
```

### **4. Monthly Improvement Cycles**
Continuous code evolution:
```bash
# First week of month
/improve
→ Apply top improvements
```

### **5. Pre-Release Security**
Always audit before releases:
```bash
# Before every release
/security-scan
/performance-audit
```

---

## 🚀 Power User Tips

### **Combine Agents**
```
You: "Full quality check"
Me: [Runs /review-pr + /security-scan + /performance-audit in parallel]
```

### **Context-Aware Agent Selection**
Just ask naturally:
```
You: "I just added payment processing, what should I check?"
Me: [Automatically runs /security-scan + /review-pr focusing on payment code]
```

### **Agent Learning**
Agents learn from your codebase:
```
/enforce-patterns
→ Learns your project-specific patterns
→ Suggests consistency improvements
→ Adapts to your style
```

### **Progressive Quality**
Start strict, get stricter:
```
Week 1: Fix critical issues only
Week 2: Fix high priority too
Week 3: Address medium issues
Week 4: 100% compliance goal
```

---

## 📊 Quality Metrics Dashboard

Track your progress with agent runs:

```
📈 Project Quality Metrics

Code Quality:     ████████░░ 82%  (Target: 90%)
Security:         ██████████ 100% ✅
Performance:      ███████░░░ 75%  (Target: 85%)
Architecture:     █████████░ 88%  ✅
Test Coverage:    █████████░ 87%  ✅
Pattern Compliance: ███████░░░ 78%  (Target: 95%)

Recent Improvements:
✅ Fixed 23 error handling violations
✅ Added 15 missing indexes
✅ Eliminated 300 lines of duplicate code
⚠️  12 pattern violations remaining

Next Actions:
1. Fix remaining DTO tag violations (2 hours)
2. Add missing test coverage in payment module
3. Optimize N+1 queries in course listing
```

---

## 🛠️ Customizing Agents

All agents are markdown files you can edit:

```bash
# Make an agent more strict
nano .claude/commands/pre-commit.md

# Add custom pattern checks
nano .claude/commands/enforce-patterns.md

# Add project-specific security rules
nano .claude/commands/security-scan.md
```

---

## 🤝 Agent Integration with CI/CD

Set up agents in your pipeline:

```yaml
# .github/workflows/pr.yml
name: PR Quality Check

on: [pull_request]

jobs:
  quality:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Run Pre-Commit
        run: |
          make lint
          make test
          make coverage

      # Could integrate Claude Code agents here
      # (future: automated PR reviews)
```

---

## 💡 Philosophy

These agents embody the philosophy:

> **"Code is read more than it's written"**

Every agent focuses on:
- **Clarity** - Code that's easy to understand
- **Consistency** - Predictable patterns everywhere
- **Correctness** - Tested, validated, secure
- **Continuous Improvement** - Always getting better

---

## 🎯 Quick Reference

| Need | Agent | Time |
|------|-------|------|
| Pre-commit check | `/pre-commit` | 45s |
| PR ready? | `/review-pr` | 2-3min |
| Security audit | `/security-scan` | 1-2min |
| Performance issues? | `/performance-audit` | 2-3min |
| Architecture validation | `/architecture-review` | 2-3min |
| Enforce consistency | `/enforce-patterns` | 1-2min |
| Find improvements | `/improve` | 1-2min |

---

**Start using agents today and watch your code quality soar! 🚀**
