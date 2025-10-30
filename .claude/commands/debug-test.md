---
description: Debug a failing test with deep analysis
tags: [test, debug, fix]
---

# Test Debugger

Debug a failing test systematically.

## Process

1. **Ask for the failing test:**
   - Test name or file
   - Error message if you have it
   - What behavior is expected

2. **Run the test in isolation:**
   ```bash
   go test -v -run TestName ./tests/path/to/test
   ```

3. **Analyze the failure:**
   - Read error message carefully
   - Check assertion failures
   - Look for nil pointer dereferences
   - Check database connection issues
   - Verify test setup/cleanup

4. **Common OCF Core test issues:**
   - **SQLite not using shared cache** → Check for `file::memory:?cache=shared`
   - **Services can't see test data** → Shared cache issue
   - **Foreign key constraint fails** → Check relationship setup
   - **Permission denied** → Check Casbin policy loading
   - **Connection refused (postgres)** → Using `localhost` instead of `postgres`
   - **Flaky test** → Race condition or timing issue

5. **Investigate the code:**
   - Read the test setup
   - Check the service implementation
   - Verify the repository
   - Look at entity registration

6. **Propose fix:**
   - Show the exact change needed
   - Explain why it fixes the issue
   - Apply the fix
   - Re-run the test

7. **Verify:**
   - Run the specific test again
   - Run related tests
   - Run full suite if critical change

**Pro tip:** If test hangs, use `timeout 30s go test ...` to prevent infinite waits.
