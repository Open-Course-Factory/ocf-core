---
description: Smart test runner - runs appropriate tests based on recent changes
tags: [test, ci, validation]
---

# Smart Test Runner

Analyze recent changes and run the appropriate test suite.

## Steps

1. **Check git status** to see what files changed
2. **Determine test scope:**
   - If entity management files changed → `make test-entity-manager`
   - If auth/permissions changed → Run auth tests
   - If payment system changed → Run payment tests
   - If organizations/groups changed → Run org/group tests
   - If multiple modules → `make test`
   - If no code changes → Ask what to test

3. **Run tests with timeout** (2 minutes max)
4. **If failures occur:**
   - Show the error details
   - Identify the failing test
   - Suggest fixes based on error patterns

5. **After success:**
   - Run `make lint` to check code quality
   - Show coverage summary if available
   - Confirm all tests passed

Use the Bash tool to run tests. If tests hang, kill and report the issue.
