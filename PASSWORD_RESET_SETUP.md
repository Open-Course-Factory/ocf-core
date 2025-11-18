# Password Reset Email Setup Guide

This guide explains how to set up email functionality for password reset in the OCF platform.

## Overview

The password reset system is now fully implemented and allows users to:
1. Request a password reset via email
2. Receive a secure reset link
3. Reset their password without accessing Casdoor directly

**Key Features:**
- ✅ Secure token generation (256-bit cryptographic random tokens)
- ✅ Token expiration (1 hour)
- ✅ One-time use tokens
- ✅ User enumeration protection
- ✅ Beautiful HTML email templates
- ✅ Transparent Casdoor integration (users never see Casdoor)

## Quick Start

### 1. Configure Email Settings

Edit your `.env` file and configure one of the following email options:

#### Option A: Gmail (Easiest for Development)

1. Go to https://myaccount.google.com/apppasswords
2. Create an App Password for "Mail"
3. Update your `.env`:

```bash
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-email@gmail.com
SMTP_PASSWORD=your-16-char-app-password
SMTP_FROM_EMAIL=noreply@ocf.fr
SMTP_FROM_NAME=OCF Platform
FRONTEND_URL=http://localhost:3000
```

#### Option B: Mailgun (Recommended for Production)

Free tier: 5,000 emails/month

1. Sign up at https://www.mailgun.com
2. Verify your domain
3. Get your SMTP credentials
4. Update your `.env`:

```bash
SMTP_HOST=smtp.mailgun.org
SMTP_PORT=587
SMTP_USERNAME=postmaster@your-domain.mailgun.org
SMTP_PASSWORD=your-mailgun-smtp-password
SMTP_FROM_EMAIL=noreply@yourdomain.com
SMTP_FROM_NAME=OCF Platform
FRONTEND_URL=https://app.yourdomain.com
```

#### Option C: SendGrid (Also Production-Ready)

Free tier: 100 emails/day

1. Sign up at https://sendgrid.com
2. Create an API key
3. Update your `.env`:

```bash
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USERNAME=apikey
SMTP_PASSWORD=your-sendgrid-api-key
SMTP_FROM_EMAIL=noreply@yourdomain.com
SMTP_FROM_NAME=OCF Platform
FRONTEND_URL=https://app.yourdomain.com
```

### 2. Restart the Application

```bash
# Restart with docker-compose
docker-compose down
docker-compose up -d

# Or if running directly
go run main.go
```

### 3. Test the Password Reset

The database table `password_reset_tokens` will be created automatically on startup.

## API Endpoints

### Request Password Reset

```http
POST /api/v1/auth/password-reset/request
Content-Type: application/json

{
  "email": "user@example.com"
}
```

**Response:**
```json
{
  "success": true,
  "message": "If an account with that email exists, a password reset link has been sent."
}
```

**Note:** The API always returns success to prevent user enumeration attacks.

### Reset Password

```http
POST /api/v1/auth/password-reset/confirm
Content-Type: application/json

{
  "token": "abc123...",
  "new_password": "newSecurePassword123"
}
```

**Response (Success):**
```json
{
  "success": true,
  "message": "Password has been reset successfully. You can now login with your new password."
}
```

**Response (Error):**
```json
{
  "success": false,
  "message": "Reset token has expired"
}
```

## Frontend Integration

### Example: Request Password Reset

```javascript
async function requestPasswordReset(email) {
  const response = await fetch('http://localhost:8080/api/v1/auth/password-reset/request', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ email }),
  });

  const data = await response.json();
  console.log(data.message);
}
```

### Example: Reset Password Page

Your frontend should have a page at `/reset-password` that:
1. Extracts the token from the URL query parameter (`?token=abc123...`)
2. Shows a password reset form
3. Submits the token and new password to the API

```javascript
async function resetPassword(token, newPassword) {
  const response = await fetch('http://localhost:8080/api/v1/auth/password-reset/confirm', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      token,
      new_password: newPassword
    }),
  });

  const data = await response.json();

  if (data.success) {
    // Redirect to login page
    window.location.href = '/login';
  } else {
    alert(data.message);
  }
}
```

## Email Template

Users will receive a professional HTML email with:
- Clear instructions
- Prominent "Reset Password" button
- Plain text link (for clients that don't support buttons)
- Expiration warning (1 hour)
- Security notice (ignore if not requested)

## Security Features

### 1. **Secure Token Generation**
- Uses `crypto/rand` for cryptographically secure random tokens
- 256-bit (64 hex characters) tokens
- Virtually impossible to guess

### 2. **Token Expiration**
- Tokens expire after 1 hour
- Old tokens are automatically invalidated

### 3. **One-Time Use**
- Tokens can only be used once
- Marked as "used" after successful password reset

### 4. **User Enumeration Protection**
- API always returns success, even if email doesn't exist
- Prevents attackers from discovering valid email addresses

### 5. **Token Invalidation**
- New reset requests invalidate all previous unused tokens
- Prevents multiple active reset links

### 6. **Database-Backed**
- Tokens stored in PostgreSQL
- Survives server restarts
- Can audit password reset attempts

## Database Schema

The `password_reset_tokens` table:

```sql
CREATE TABLE password_reset_tokens (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    deleted_at TIMESTAMP,
    user_id VARCHAR(255) NOT NULL,
    token VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    used_at TIMESTAMP
);

CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);
CREATE INDEX idx_password_reset_tokens_expires_at ON password_reset_tokens(expires_at);
CREATE INDEX idx_password_reset_tokens_deleted_at ON password_reset_tokens(deleted_at);
```

## Architecture

```
┌──────────┐      ┌──────────────┐      ┌─────────────┐      ┌──────────┐
│ Frontend │ ───> │ Password     │ ───> │ Email       │ ───> │ SMTP     │
│          │      │ Reset API    │      │ Service     │      │ Server   │
└──────────┘      └──────────────┘      └─────────────┘      └──────────┘
                         │
                         │
                         v
                  ┌─────────────┐
                  │ PostgreSQL  │
                  │ (tokens)    │
                  └─────────────┘
                         │
                         │
                         v
                  ┌─────────────┐
                  │ Casdoor     │
                  │ (users)     │
                  └─────────────┘
```

## Files Created

### Core Services
- `src/email/services/emailService.go` - Email sending service
- `src/auth/services/passwordResetService.go` - Password reset business logic
- `src/auth/models/passwordResetToken.go` - Token model

### API Layer
- `src/auth/routes/passwordResetRoutes/passwordResetController.go` - API controller
- `src/auth/routes/passwordResetRoutes/passwordResetRouter.go` - Route registration
- `src/auth/dto/passwordResetDto.go` - Request/response DTOs

### Configuration
- Updated `main.go` - Route registration
- Updated `src/initialization/database.go` - Database migration
- Updated `.env` - Email configuration

## Troubleshooting

### Email Not Sending

1. **Check SMTP credentials:**
   ```bash
   # Verify environment variables are loaded
   env | grep SMTP
   ```

2. **Check logs:**
   ```bash
   docker-compose logs -f ocf-core
   # Look for "Password reset email sent to:" or error messages
   ```

3. **Test SMTP connection:**
   - Gmail: Make sure you're using an App Password, not your regular password
   - Mailgun: Verify your domain is verified
   - SendGrid: Check your API key is active

### Token Not Working

1. **Token expired:** Tokens expire after 1 hour
2. **Token already used:** Each token can only be used once
3. **Invalid token:** Check the token wasn't truncated in the URL

### User Not Receiving Email

1. **Check spam folder**
2. **Verify email address is correct in Casdoor**
3. **Check SMTP_FROM_EMAIL is not blacklisted**

## Next Steps

You can extend this system to:
- Send welcome emails when users register
- Send notifications for important events
- Add email verification for new accounts
- Send course completion certificates
- Add newsletter functionality

## Support

For questions or issues:
1. Check the logs: `docker-compose logs -f ocf-core`
2. Verify environment variables: `env | grep SMTP`
3. Test the API endpoints with curl or Postman
4. Check the database: `SELECT * FROM password_reset_tokens;`
