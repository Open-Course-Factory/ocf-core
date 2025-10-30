---
description: Quick API endpoint testing with automatic authentication
tags: [api, curl, test, debug]
---

# API Endpoint Tester

Test an API endpoint with automatic authentication handling.

## Steps

1. **Ask for endpoint details:**
   - HTTP method (GET, POST, PATCH, DELETE)
   - Endpoint path (e.g., `/api/v1/organizations`)
   - Request body (if POST/PATCH)
   - Whether authentication is required

2. **If authentication required:**
   - Get a fresh JWT token using:
     ```bash
     curl -s -X POST http://localhost:8080/api/v1/auth/login \
       -H "Content-Type: application/json" \
       -d '{"email":"1.supervisor@test.com","password":"testtest"}'
     ```
   - Extract the `access_token` from response
   - Use it in the Authorization header

3. **Make the API request:**
   - Use `curl` with proper headers
   - Pretty-print JSON response with `python3 -m json.tool`
   - Show HTTP status code

4. **Analyze response:**
   - Check for errors
   - Validate response structure
   - Highlight important fields

5. **Suggest next steps:**
   - Related endpoints to test
   - What to verify in database
   - Potential issues to watch for

Test credentials:
- Email: `1.supervisor@test.com`
- Password: `testtest`
- Roles: administrator, supervisor (full access)
