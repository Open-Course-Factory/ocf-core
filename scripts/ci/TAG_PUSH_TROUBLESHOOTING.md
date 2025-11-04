# GitLab CI Tag Push Issue - Troubleshooting Guide

## 🔍 The Problem

**Symptom:** CI creates tags but they don't appear in GitLab repository

When your CI pipeline runs:
1. ✅ The `check:version` job detects version changes
2. ✅ It creates the tag locally (e.g., `v0.2.0`)
3. ❌ The `git push` appears to succeed but the tag isn't visible in GitLab
4. ⚠️ On the next run, CI detects the tag exists (from `git fetch --tags`)
5. ❌ But you still don't see it in the GitLab UI

**Root Cause:** The `CI_JOB_TOKEN` doesn't have permission to push tags to your repository.

## 🔐 Why This Happens

GitLab's `CI_JOB_TOKEN` has **read-only** access by default. It can:
- ✅ Clone the repository
- ✅ Fetch branches and tags
- ❌ **Cannot** push tags or branches

The tag gets created in the CI runner's local context but the push fails silently.

## ✅ Solution 1: Configure CI_PUSH_TOKEN (Recommended)

### Step 1: Create Project Access Token

1. Go to your GitLab project
2. Navigate to: **Settings** → **Access Tokens**
3. Click **Add new token**
4. Configure:
   - **Token name:** `CI_PUSH_TOKEN`
   - **Role:** `Maintainer` (or `Developer` if sufficient)
   - **Scopes:** ✅ Check `write_repository`
   - **Expiration:** Set appropriate date (or no expiration)
5. Click **Create project access token**
6. **⚠️ Copy the token immediately** (you won't see it again!)

### Step 2: Add Token to CI/CD Variables

1. Go to: **Settings** → **CI/CD** → **Variables**
2. Click **Add variable**
3. Configure:
   - **Key:** `CI_PUSH_TOKEN`
   - **Value:** [paste the token you copied]
   - **Type:** Variable
   - **Environment scope:** All (default)
   - **Protect variable:** ✅ Yes (recommended)
   - **Mask variable:** ✅ Yes (recommended)
4. Click **Add variable**

### Step 3: Verify

Your `.gitlab-ci.yml` already uses `CI_PUSH_TOKEN` with fallback:
```yaml
TOKEN=${CI_PUSH_TOKEN:-$CI_JOB_TOKEN}
```

Next pipeline run will use the new token automatically.

## ✅ Solution 2: Configure Protected Tags

If you want to use `CI_JOB_TOKEN`, you need to adjust protected tag settings:

1. Go to: **Settings** → **Repository** → **Protected tags**
2. Add a new rule:
   - **Tag:** `v*` (matches v0.1.0, v0.2.0, etc.)
   - **Allowed to create:**
     - ✅ Maintainers
     - ✅ Developers (if needed)
3. Save changes

**Note:** This only works if GitLab Runner has the necessary permissions, which varies by GitLab version.

## ✅ Solution 3: Use Deploy Token

Alternative to Project Access Token:

1. Go to: **Settings** → **Repository** → **Deploy tokens**
2. Click **Add token**
3. Configure:
   - **Name:** `ci-deploy-token`
   - **Scopes:** ✅ `write_repository`
4. Add to CI/CD variables as `CI_PUSH_TOKEN`

## 🧪 Testing & Diagnosis

### Run Diagnostic Script in CI

Add this job to `.gitlab-ci.yml` temporarily:

```yaml
diagnose:tags:
  stage: check
  image: alpine:latest
  before_script:
    - apk add --no-cache git bash
  script:
    - ./scripts/ci/diagnose-tag-push.sh
  rules:
    - if: $CI_COMMIT_BRANCH == "main"
      when: manual
```

Run it manually from the pipeline to diagnose the issue.

### Check CI Logs

Look for these patterns in your CI logs:

**Success:**
```
✅ Tag v0.2.0 created and pushed successfully
⏳ A new pipeline will start for the tag...
```

**Failure (permission denied):**
```
remote: You are not allowed to push code to protected branches on this project.
remote: error: GH006: Protected branch update failed
```

**Failure (protected tags):**
```
remote: GitLab: You are not allowed to push tags to this project.
```

## 🔄 How It Should Work

### Normal Flow (Working)

1. Developer commits VERSION file change to `main`
2. CI `check:version` job runs
3. Detects version change (e.g., 0.1.0 → 0.2.0)
4. Creates tag `v0.2.0` locally
5. **Pushes tag successfully** using `CI_PUSH_TOKEN`
6. Tag appears in GitLab: Repository → Tags
7. Tag triggers `build` job automatically
8. Docker image built and pushed

### Current Flow (Broken)

1. Developer commits VERSION file change to `main`
2. CI `check:version` job runs
3. Detects version change
4. Creates tag `v0.2.0` locally
5. **Push fails silently** (insufficient permissions)
6. Tag NOT visible in GitLab
7. Next run: `git fetch --tags` sees local tag, skips creation
8. Build never triggers

## 📊 Verification

After implementing the solution:

### 1. Check Tags Locally
```bash
git fetch --tags
git tag -l
```

### 2. Check Tags on Remote
```bash
git ls-remote --tags origin
```

### 3. Verify in GitLab UI
- Go to: **Repository** → **Tags**
- You should see: `v0.2.0` with your commit message

### 4. Check Pipeline Triggers
- New tag should trigger `build` job automatically
- Check: **CI/CD** → **Pipelines**
- Look for pipeline with source: "Tag"

## 🐛 Common Issues

### "Tag already exists" but not visible

**Cause:** Tag created in previous CI run but push failed
**Fix:**
```bash
# In CI, delete the failed tag first
git push --delete origin v0.2.0 2>/dev/null || true
```

### "Permission denied" even with CI_PUSH_TOKEN

**Cause:** Token has wrong scope or role
**Fix:** Recreate token with `write_repository` scope and `Maintainer` role

### Protected branch blocking tag push

**Cause:** Repository settings prevent tag pushes
**Fix:** Adjust protected tags settings (see Solution 2)

## 📚 Related GitLab Documentation

- [CI/CD Variables](https://docs.gitlab.com/ee/ci/variables/)
- [Project Access Tokens](https://docs.gitlab.com/ee/user/project/settings/project_access_tokens.html)
- [Protected Tags](https://docs.gitlab.com/ee/user/project/protected_tags.html)
- [CI/CD Job Token](https://docs.gitlab.com/ee/ci/jobs/ci_job_token.html)

## ✅ Final Checklist

- [ ] Created Project Access Token with `write_repository` scope
- [ ] Added `CI_PUSH_TOKEN` to CI/CD variables (protected + masked)
- [ ] Verified token has `Maintainer` or `Developer` role
- [ ] Tested with diagnostic script
- [ ] Confirmed tag appears in GitLab UI
- [ ] Verified `build` job triggers on new tags
- [ ] Documented token expiration date (if applicable)

---

**Last Updated:** 2025-11-04
**Issue:** Tags created in CI but not visible in GitLab
**Status:** Solution implemented with improved error reporting
