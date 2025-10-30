---
description: Regenerate Swagger API documentation and validate it
tags: [swagger, docs, api]
---

# Update API Documentation

Regenerate Swagger documentation after API changes.

## Steps

1. **Check for swagger annotations:**
   - Verify `@Summary`, `@Description`, `@Tags` in handlers
   - Check `@Param` definitions for all parameters
   - Validate `@Success` and `@Failure` responses
   - Ensure DTOs have proper JSON tags

2. **Run swagger generation:**
   ```bash
   swag init --parseDependency --parseInternal
   ```

3. **Check for errors:**
   - Parse errors in handlers
   - Missing type definitions
   - Invalid annotation syntax

4. **Validate generated docs:**
   - Check `docs/swagger.json` was updated
   - Look for new endpoints
   - Verify response schemas

5. **Test the documentation:**
   - Start server if not running
   - Visit http://localhost:8080/swagger/
   - Try the "Try it out" feature on new endpoints
   - Verify request/response examples

6. **Common issues to fix:**
   - Missing `// @Router` annotations
   - Incorrect parameter types
   - DTOs not exported (lowercase)
   - Missing JSON tags on DTO fields

**Important:** The `docs/` folder is auto-generated. Never edit it manually!
