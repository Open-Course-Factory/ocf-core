# Claude Code Documentation

This directory contains detailed technical documentation generated during development sessions with Claude Code. These documents explain implementation decisions, architectural changes, and provide context for future development.

## Purpose

These files serve as:
- **Implementation guides** for complex features
- **Architecture documentation** for modular systems
- **Troubleshooting references** for common issues
- **Integration guides** for frontend/backend coordination

## Documents

### Feature System

- **`MODULAR_FEATURES.md`** - Complete guide to the modular feature flag system
  - How modules declare features
  - Architecture and design patterns
  - How to add new modules with features
  - Migration path to microservices
  - Database-backed feature flags
  - API endpoints and integration

### Subscription & Metrics

- **`METRICS_FIX_PLAN_FEATURES.md`** - ⚠️ **CURRENT IMPLEMENTATION** - Fix for usage metrics not being created
  - Root cause analysis (plan.Features vs feature flags confusion)
  - Distinction between display strings and feature keys
  - Clarifies that only global feature flags control metrics (NOT plan.Features)

- **`USAGE_ENDPOINT_FIX.md`** - Fix for usage endpoint returning null
  - Changed to return empty array instead of null
  - Sync mechanism for initializing metrics

### Terminal Pricing System (Historical Reference)

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

### Frontend Integration

- **`FRONTEND_INTEGRATION_PROMPT.md`** - Complete guide for frontend team
  - API endpoints and usage
  - React/JavaScript examples
  - User dashboard implementation
  - Admin feature management UI
  - Caching strategies

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
