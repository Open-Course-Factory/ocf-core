# Security Quick Start Guide

**For:** Developers and DevOps
**Quick Read:** 5 minutes
**Full Documentation:** See `SECURITY.md`

---

## 🚀 TL;DR - Security Status

✅ **86% of Critical Issues Resolved** (6 out of 7)
✅ **Production-Ready** (with temporary rate limiting via WAF)
⚠️ **Rate Limiting** needs Redis implementation (planned)

---

## ✅ What's Secure

| Feature | Status | Details |
|---------|--------|---------|
| Authentication | ✅ | JWT validation, token blacklist, secure transport |
| CORS | ✅ | Whitelist only, no wildcards |
| Payments | ✅ | 3D Secure enabled, PSD2 compliant |
| Webhooks | ✅ | Replay protection, signature verification |
| Audit Logging | ✅ | 70+ event types, compliance-ready |
| Authorization | ✅ | RBAC with Casbin |

---

## ⚠️ Known Gap

**Rate Limiting:** Not implemented (requires Redis)

**Temporary Workaround:**
- Use CloudFlare or AWS WAF for DDoS protection
- Configure nginx/reverse proxy connection limits
- Monitor logs and manually block abusive IPs

---

## 🔧 Quick Setup

### 1. Environment Variables (Required)

```bash
# .env file
ENVIRONMENT=production
FRONTEND_URL=https://app.yourdomain.com
ADMIN_FRONTEND_URL=https://admin.yourdomain.com
STRIPE_SECRET_KEY=sk_live_xxxxx
STRIPE_WEBHOOK_SECRET=whsec_xxxxx
```

### 2. Verify Security Features

```bash
# Build and run
go build -o ocf-server && ./ocf-server

# Check startup logs for:
✅ Webhook cleanup job started
✅ Audit log cleanup job started
✅ CORS allowed origins: [https://app.yourdomain.com ...]
```

### 3. Test Critical Features

```bash
# Test CORS (should be rejected)
curl -H "Origin: https://evil.com" \
     -X OPTIONS http://localhost:8080/api/v1/version

# Test auth (should work)
curl -H "Authorization: Bearer $TOKEN" \
     http://localhost:8080/api/v1/users/me

# Check audit logs
psql -d ocf -c "SELECT COUNT(*) FROM audit_logs WHERE created_at > NOW() - INTERVAL '1 hour';"
```

---

## 📋 Pre-Production Checklist

**Critical (Do Not Deploy Without):**
- [ ] `ENVIRONMENT=production` set
- [ ] Production Stripe keys configured
- [ ] CORS whitelist verified (only production domains)
- [ ] HTTPS enabled (TLS 1.2+)
- [ ] WAF or CDN rate limiting configured
- [ ] Webhook endpoint using HTTPS
- [ ] Database backups configured

**Important (Recommended):**
- [ ] Centralized logging (ELK, Datadog, etc.)
- [ ] Alerts for failed logins, webhook failures
- [ ] Monitoring dashboard (Grafana, etc.)
- [ ] Error tracking (Sentry, etc.)

**Nice to Have:**
- [ ] Penetration testing completed
- [ ] Security headers configured
- [ ] Enhanced input validation

---

## 🔍 Quick Diagnostics

### Check Security Status

```bash
# 1. CORS configuration
grep "CORS allowed origins" logs/server.log

# 2. Audit logging active
psql -d ocf -c "SELECT event_type, COUNT(*) FROM audit_logs GROUP BY event_type ORDER BY COUNT(*) DESC LIMIT 10;"

# 3. Webhook protection
psql -d ocf -c "SELECT COUNT(*) FROM webhook_events WHERE created_at > NOW() - INTERVAL '24 hours';"

# 4. Token blacklist
psql -d ocf -c "SELECT COUNT(*) FROM token_blacklist WHERE expires_at > NOW();"
```

### Common Issues

**Issue:** CORS errors in browser console
**Fix:** Verify `FRONTEND_URL` matches exactly (including protocol and port)

**Issue:** Webhooks failing signature verification
**Fix:** Check `STRIPE_WEBHOOK_SECRET` matches Stripe Dashboard

**Issue:** Audit logs not appearing
**Fix:** Check database migration ran: `SELECT COUNT(*) FROM audit_logs;`

---

## 📚 Documentation Map

- **`SECURITY.md`** ← Start here for complete security documentation
- **`AUDIT_LOGGING_IMPLEMENTATION.md`** ← Audit logging details
- **`tests/auth/SECURITY_TESTS_README.md`** ← Security testing guide
- **`documentation/archive/security-docs-archive-2025-11-05/`** ← Historical docs

---

## 🚨 Emergency Contacts

**Security Incident?**
1. Check audit logs for activity: `psql -d ocf -c "SELECT * FROM audit_logs WHERE severity='critical' ORDER BY created_at DESC LIMIT 20;"`
2. Revoke compromised tokens: Add to token_blacklist table
3. Block suspicious IPs: Configure WAF/firewall
4. Contact: [security@yourdomain.com] (TO BE SET UP)

---

## 🎯 Next Steps

1. **Read `SECURITY.md`** for comprehensive security guide
2. **Run security tests** in `tests/auth/`
3. **Configure monitoring** and alerts
4. **Schedule security review** (monthly recommended)
5. **Plan rate limiting implementation** (Redis setup)

---

**Quick Links:**
- [Full Security Documentation](./SECURITY.md)
- [Audit Logging Guide](./AUDIT_LOGGING_IMPLEMENTATION.md)
- [Security Tests](./tests/auth/SECURITY_TESTS_README.md)
- [Archived Docs](./documentation/archive/security-docs-archive-2025-11-05/)

**Last Updated:** 2025-11-05
**Version:** 1.0
