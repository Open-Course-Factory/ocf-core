# Claude Code Documentation

This directory contains detailed technical documentation generated during development sessions with Claude Code. These documents explain implementation decisions, architectural changes, and provide context for future development.

## Purpose

These files serve as:
- **Implementation guides** for complex features
- **Architecture documentation** for modular systems
- **Troubleshooting references** for common issues
- **Integration guides** for frontend/backend coordination

## Documents

### 🏗️ Architecture & Refactoring

- **`REFACTORING_COMPLETE_SUMMARY.md`** - ⭐ **Comprehensive 6-phase refactoring summary**
  - 100% permission management refactoring completed
  - 12 utility helpers created
  - ~2,600 lines of code eliminated
  - Zero breaking changes
  - Framework readiness: 60% → 85%
  - Complete guide to new patterns and utilities

- **`ORGANIZATION_GROUPS_SYSTEM.md`** - ⭐ **GitLab-style multi-tenant architecture**
  - Organizations & groups hierarchy
  - Multi-tenancy support
  - Cascading permissions (org → groups)
  - Personal organizations
  - Role-based access (owner/manager/member)
  - Complete API documentation
  - Integration guide with examples

### 📊 Feature System

- **`MODULAR_FEATURES.md`** - Complete guide to the modular feature flag system
  - How modules declare features
  - Architecture and design patterns
  - How to add new modules with features
  - Migration path to microservices
  - Database-backed feature flags
  - API endpoints and integration

### 💰 Subscription & Metrics

- **`METRICS_FIX_PLAN_FEATURES.md`** - ⚠️ **CURRENT IMPLEMENTATION** - Fix for usage metrics not being created
  - Root cause analysis (plan.Features vs feature flags confusion)
  - Distinction between display strings and feature keys
  - Clarifies that only global feature flags control metrics (NOT plan.Features)

- **`USAGE_ENDPOINT_FIX.md`** - Fix for usage endpoint returning null
  - Changed to return empty array instead of null
  - Sync mechanism for initializing metrics

### 💻 Terminal Pricing System (Historical Reference)

These documents describe the initial terminal pricing implementation. Keep for reference on pricing strategy and business model.

- **`TERMINAL_PRICING_PLAN.md`** - Original design document for terminal pricing
  - Pricing tiers and business model
  - Plan limits and restrictions
  - Migration from courses-based to terminals-based pricing

- **`TERMINAL_PRICING_IMPLEMENTATION_SUMMARY.md`** - Implementation details
  - Code changes and files modified
  - Database schema additions

- **`TERMINAL_PRICING_TESTING_SUMMARY.md`** - Test results and validation
  - Manual testing procedures
  - Known issues that were fixed

### 🎨 Frontend Integration

- **`FRONTEND_INTEGRATION_PROMPT.md`** - Complete guide for frontend team
  - API endpoints and usage
  - React/JavaScript examples
  - User dashboard implementation
  - Admin feature management UI
  - Caching strategies

- **`BULK_LICENSE_FRONTEND_GUIDE.md`** - Bulk license purchase frontend
  - Multi-user license purchase flow
  - Payment and checkout integration

- **`GROUPS_FRONTEND_INTEGRATION.md`** - Groups system frontend guide
  - Group management UI
  - Member management
  - Role assignment

- **`USER_SETTINGS_FRONTEND_GUIDE.md`** - User settings frontend
  - Profile management
  - Account settings
  - Preferences

- **`TERMINAL_HIDING_FRONTEND_GUIDE.md`** - Terminal hiding/showing UI
  - Terminal visibility controls
  - User experience patterns

- **`TERMINAL_SHARING_ACCESS_SYSTEM.md`** - Terminal sharing frontend
  - Share terminal with other users
  - Access level management (read/write/admin)

- **`COURSE_VERSION_MANAGEMENT_FRONTEND.md`** - Course versioning UI
  - Version control for courses
  - Publishing workflow

## Usage

These documents are intended for:

1. **Claude Code** - Context for future development sessions
2. **Developers** - Understanding implementation decisions
3. **Frontend Team** - Integration guides and API documentation
4. **DevOps** - Deployment and migration procedures

## Maintenance

- Keep documents updated when making significant architectural changes
- Add new documents for complex features or troubleshooting
- Remove outdated documents when features are deprecated
- Use clear, concise titles that describe the content

## Related Files

- **`/CLAUDE.md`** - Main project guidance for Claude Code (in root)
- **`.claude/`** - Claude Code configuration directory
