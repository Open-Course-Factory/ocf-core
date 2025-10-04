# CI/CD Workflow Diagrams

## Release Workflow

```
┌─────────────────────────────────────────────────────────────────┐
│                     Developer Updates VERSION                   │
│                                                                 │
│  echo "0.2.0" > VERSION                                         │
│  git commit -m "Bump version to 0.2.0"                          │
│  git push origin main                                           │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                Pipeline 1: Main Branch Push                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Stage: check                                                   │
│  ┌──────────────────────────────────────────────────┐           │
│  │  check:version job                               │           │
│  │  1. Read VERSION file (0.2.0)                    │           │
│  │  2. Get latest tag (v0.1.0)                      │           │
│  │  3. Compare versions (0.2.0 ≠ 0.1.0)             │           │
│  │  4. Create tag v0.2.0                            │           │
│  │  5. Push tag to GitLab                           │  ✓ PASS   │
│  └──────────────────────────────────────────────────┘           │
│                                                                 │
│  Stage: test                                                    │
│  ┌──────────────────────────────────────────────────┐           │
│  │  All test jobs                                    │  SKIPPED │
│  │  (Only run on MR or manual trigger)              │           │
│  └──────────────────────────────────────────────────┘           │
│                                                                 │
│  Stage: build                                                   │
│  ┌──────────────────────────────────────────────────┐           │
│  │  build job                                        │  SKIPPED │
│  │  (Only runs on tags)                             │           │
│  └──────────────────────────────────────────────────┘           │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      │ Tag push triggers new pipeline
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│              Pipeline 2: Tag v0.2.0 Created                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Stage: check                                                   │
│  ┌──────────────────────────────────────────────────┐           │
│  │  check:version job                               │  SKIPPED  │
│  │  (Only runs on main/develop branches)            │           │
│  └──────────────────────────────────────────────────┘           │
│                                                                 │
│  Stage: test                                                    │
│  ┌──────────────────────────────────────────────────┐           │
│  │  All test jobs                                   │  SKIPPED  │
│  │  (Only run on MR or manual trigger)              │           │
│  └──────────────────────────────────────────────────┘           │
│                                                                 │
│  Stage: build                                                   │
│  ┌──────────────────────────────────────────────────┐           │
│  │  build job                                       │           │
│  │  1. Build Docker image                           │           │
│  │  2. Tag as ocf-core:0.2.0                        │           │
│  │  3. Tag as ocf-core:latest                       │  ✓ PASS   │
│  │  4. Push to registry                             │           │
│  └──────────────────────────────────────────────────┘           │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
                 ✅ RELEASE COMPLETE

         Docker images published:
         - registry/ocf-core:0.2.0
         - registry/ocf-core:latest
```

## Merge Request Workflow

```
┌─────────────────────────────────────────────────────────────────┐
│               Developer Creates Merge Request                   │
│                                                                 │
│  git checkout -b feature/new-feature                            │
│  git push origin feature/new-feature                            │
│  [Create MR in GitLab]                                          │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                 Pipeline: Merge Request                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Stage: check                                                   │
│  ┌──────────────────────────────────────────────────┐           │
│  │  check:version job                               │  SKIPPED  │
│  │  (Only runs on main/develop branches)            │           │
│  └──────────────────────────────────────────────────┘           │
│                                                                 │
│  Stage: test                                                    │
│  ┌──────────────────────────────────────────────────┐           │
│  │  test:entity-management                          │  ✓ PASS   │
│  │  test:courses                                    │  ✓ PASS   │
│  │  test:quick                                      │  ✓ PASS   │
│  │  test:race                                       │  ✓ PASS   │
│  │  test:auth                                       │  ✓ PASS   │
│  └──────────────────────────────────────────────────┘           │
│                                                                 │
│  Stage: build                                                   │
│  ┌──────────────────────────────────────────────────┐           │
│  │  build job                                       │  SKIPPED  │
│  │  (Only runs on tags)                             │           │
│  └──────────────────────────────────────────────────┘           │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
              ✅ All tests passed
           Ready to merge!
```

## Regular Commit Workflow (Fast)

```
┌─────────────────────────────────────────────────────────────────┐
│              Developer Pushes to Main/Develop                   │
│                                                                 │
│  git push origin main                                           │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                Pipeline: Regular Commit                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Stage: check                                                   │
│  ┌──────────────────────────────────────────────────┐           │
│  │  check:version job                               │           │
│  │  VERSION unchanged → No tag created              │  ✓ PASS   │
│  └──────────────────────────────────────────────────┘           │
│                                                                 │
│  Stage: test                                                    │
│  ┌──────────────────────────────────────────────────┐           │
│  │  All test jobs                                   │  SKIPPED  │
│  │  (Only run on MR or manual trigger)              │           │
│  └──────────────────────────────────────────────────┘           │
│                                                                 │
│  Stage: build                                                   │
│  ┌──────────────────────────────────────────────────┐           │
│  │  build job                                       │  SKIPPED  │
│  │  (Only runs on tags)                             │           │
│  └──────────────────────────────────────────────────┘           │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
                 ✅ COMPLETE
           (Fast - ~10 seconds)
```

## Pipeline Decision Matrix

| Trigger | check:version | Tests | Build |
|---------|---------------|-------|-------|
| Push to main (VERSION unchanged) | ✓ Run (skip tag) | ✗ Skip | ✗ Skip |
| Push to main (VERSION changed) | ✓ Run (create tag) | ✗ Skip | ✗ Skip |
| Tag v* created | ✗ Skip | ✗ Skip | ✓ Run |
| Merge Request | ✗ Skip | ✓ Run | ✗ Skip |
| Manual trigger | ✗ Skip | ✓ Run | ✗ Skip |
