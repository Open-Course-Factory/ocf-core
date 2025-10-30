---
description: Find implementation examples and code patterns in the codebase
tags: [search, examples, patterns]
---

# Code Pattern Finder

Find implementation examples to guide your development.

## Usage

I'll search for implementation patterns you need. Ask me to find:

**Common Patterns:**
- "How to implement a service with validation"
- "How to add relationships between entities"
- "How to create custom middleware"
- "How to handle bulk operations"
- "How to implement hooks (BeforeCreate, AfterUpdate, etc.)"
- "How to add custom routes to an entity"
- "How to use utils validators"
- "How to implement permission checks"
- "How to create DTOs with pointer fields"
- "How to handle transactions"

**Process:**

1. **Use Explore agent** to find relevant files quickly
2. **Extract key patterns** from best implementations
3. **Show code examples** with file references
4. **Explain the pattern** and when to use it
5. **Point out gotchas** and best practices

**Example Output:**

```go
// Pattern: Service with validation
// File: src/groups/services/groupService.go:45

func (s *GroupService) CreateGroup(input dto.CreateGroupInput) error {
    // 1. Validate inputs
    if err := utils.ChainValidators(
        utils.ValidateStringNotEmpty(input.Name, "name"),
        utils.ValidateStringLength(input.Name, 3, 100, "name"),
    ); err != nil {
        return err
    }

    // 2. Use generic service
    _, err := s.genericService.CreateEntity(input, "Group", userID)
    return err
}
```

Tell me what pattern you need!
