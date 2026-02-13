## Git Workflow Rules

1. **Always fetch before comparing**: Run `git fetch origin` before any branch comparison
2. **MR target is always `main`**: Never target a feature branch unless I explicitly say otherwise
3. **Validate before MR creation**: Before creating any MR, run `git log --oneline origin/main..HEAD` and show me the commit count. If it exceeds 10 commits, STOP and ask me to confirm.
4. **Never use git filter-branch**: For history cleanup, use `git rebase -i` scoped to only commits ahead of origin/main
5. **Branch naming**: Use `fix/` for bugs, `feat/` for features, `chore/` for maintenance
6. **Confirm remote state**: Before creating an MR, verify the branch doesn't already have an open MR with `glab mr list --source-branch=$(git branch --show-current)`
7. **Commits**: Must follow conventional patterns and not mention Claude

## GitLab (not GitHub)

This project uses GitLab, not GitHub. Use `glab` CLI for merge requests and issues. MR workflow:
1. `glab issue create --title "..." --description "..."`
2. `glab mr create --source-branch <branch> --target-branch main --title "..." --description "..."`
Never use `gh` or assume GitHub APIs.

## Debugging Approach

- When diagnosing bugs, do NOT celebrate or declare success until the user confirms the fix works in their environment.
- Exhaust the simplest explanations first before diving into framework internals (e.g., check middleware, environment config, null values before blaming GORM or library bugs).
- If a fix attempt doesn't resolve the issue, step back and re-read the user's original problem statement before trying the next hypothesis.

## Go Project Conventions

- This is a Go backend project. Always run `go build ./...` after code changes to catch compile errors.
- The API server runs on port 8080 (not 3333 or other ports).
- Swagger docs are generated — check the correct output directory before regenerating.
