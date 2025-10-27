package utils_test

import (
	"fmt"
	"testing"

	"soli/formations/src/auth/mocks"
	"soli/formations/src/utils"

	"github.com/stretchr/testify/assert"
)

// ==========================================
// Low-Level Permission Function Tests
// ==========================================

func TestAddPolicy(t *testing.T) {
	t.Run("Success - Add policy", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.AddPolicy(mockEnforcer, "user123", "/api/v1/groups/abc", "GET|POST", opts)

		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetAddPolicyCallCount())
	})

	t.Run("Error - Add policy fails", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return false, fmt.Errorf("policy already exists")
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.AddPolicy(mockEnforcer, "user123", "/api/v1/groups/abc", "GET", opts)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add policy")
	})

	t.Run("Warn on error - Add policy fails but only warns", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return false, fmt.Errorf("policy already exists")
		}

		opts := utils.PermissionOptions{LoadPolicyFirst: false, WarnOnError: true}
		err := utils.AddPolicy(mockEnforcer, "user123", "/api/v1/groups/abc", "GET", opts)

		assert.NoError(t, err) // Should not error, only warn
	})

	t.Run("LoadPolicy first", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.LoadPolicyFunc = func() error {
			return nil
		}
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		opts := utils.PermissionOptions{LoadPolicyFirst: true, WarnOnError: false}
		err := utils.AddPolicy(mockEnforcer, "user123", "/api/v1/groups/abc", "GET", opts)

		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetLoadPolicyCallCount())
	})
}

func TestRemovePolicy(t *testing.T) {
	t.Run("Success - Remove policy", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.RemovePolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.RemovePolicy(mockEnforcer, "user123", "/api/v1/groups/abc", "GET|POST", opts)

		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetRemovePolicyCallCount())
	})

	t.Run("Error - Remove policy fails", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.RemovePolicyFunc = func(params ...interface{}) (bool, error) {
			return false, fmt.Errorf("policy not found")
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.RemovePolicy(mockEnforcer, "user123", "/api/v1/groups/abc", "GET", opts)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove policy")
	})
}

func TestAddGroupingPolicy(t *testing.T) {
	t.Run("Success - Add user to role group", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.AddGroupingPolicy(mockEnforcer, "user123", "group:abc", opts)

		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetAddGroupingPolicyCallCount())
	})

	t.Run("Error - Add grouping policy fails", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return false, fmt.Errorf("user already in role")
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.AddGroupingPolicy(mockEnforcer, "user123", "group:abc", opts)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add user to role group")
	})
}

func TestRemoveGroupingPolicy(t *testing.T) {
	t.Run("Success - Remove user from role group", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.RemoveGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.RemoveGroupingPolicy(mockEnforcer, "user123", "group:abc", opts)

		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetRemoveGroupingPolicyCallCount())
	})

	t.Run("Error - Remove grouping policy fails", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.RemoveGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return false, fmt.Errorf("user not in role")
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.RemoveGroupingPolicy(mockEnforcer, "user123", "group:abc", opts)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove user from role group")
	})
}

// ==========================================
// High-Level Entity Permission Function Tests
// ==========================================

func TestGrantEntityAccess(t *testing.T) {
	t.Run("Success - Grant entity access", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.GrantEntityAccess(mockEnforcer, "user123", "group", "abc-def-ghi", "GET|POST", opts)

		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetAddGroupingPolicyCallCount(), "Should add user to role group")
		assert.Equal(t, 1, mockEnforcer.GetAddPolicyCallCount(), "Should add route permission")
	})

	t.Run("Error - AddGroupingPolicy fails", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return false, fmt.Errorf("user already in role")
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.GrantEntityAccess(mockEnforcer, "user123", "group", "abc-def-ghi", "GET", opts)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add user to role group")
	})
}

func TestRevokeEntityAccess(t *testing.T) {
	t.Run("Success - Revoke entity access", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.RemoveGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.RevokeEntityAccess(mockEnforcer, "user123", "group", "abc-def-ghi", opts)

		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetRemoveGroupingPolicyCallCount())
	})
}

func TestGrantEntitySubResourceAccess(t *testing.T) {
	t.Run("Success - Grant sub-resource access", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.GrantEntitySubResourceAccess(mockEnforcer, "group:abc", "group", "abc", "members", "GET|POST", opts)

		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetAddPolicyCallCount())
	})
}

func TestGrantCompleteEntityAccess(t *testing.T) {
	t.Run("Success - Grant complete entity access with sub-resources", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		subResources := []string{"members", "settings"}
		opts := utils.DefaultPermissionOptions()
		err := utils.GrantCompleteEntityAccess(mockEnforcer, "user123", "group", "abc-def", subResources, opts)

		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetAddGroupingPolicyCallCount(), "Should add user to role group")
		// 1 for main route + 2 for sub-resources
		assert.Equal(t, 3, mockEnforcer.GetAddPolicyCallCount(), "Should add 3 policies (1 main + 2 sub-resources)")
	})

	t.Run("LoadPolicy only called once", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.LoadPolicyFunc = func() error {
			return nil
		}
		mockEnforcer.AddGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		subResources := []string{"members", "settings"}
		opts := utils.PermissionOptions{LoadPolicyFirst: true, WarnOnError: false}
		err := utils.GrantCompleteEntityAccess(mockEnforcer, "user123", "group", "abc-def", subResources, opts)

		assert.NoError(t, err)
		// LoadPolicy should be called only once at the beginning, not for each sub-call
		assert.Equal(t, 1, mockEnforcer.GetLoadPolicyCallCount())
	})
}

// ==========================================
// Specialized Permission Function Tests
// ==========================================

func TestGrantManagerPermissions(t *testing.T) {
	t.Run("Success - Grant manager permissions", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		manageableSubResources := []string{"members", "groups"}
		opts := utils.DefaultPermissionOptions()
		err := utils.GrantManagerPermissions(mockEnforcer, "user123", "organization", "abc-def", manageableSubResources, opts)

		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetAddGroupingPolicyCallCount(), "Should add user to manager role")
		// 1 for main route + 2 for sub-resources
		assert.Equal(t, 3, mockEnforcer.GetAddPolicyCallCount(), "Should add 3 policies (1 main + 2 sub-resources)")
	})

	t.Run("Error - AddGroupingPolicy fails", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return false, fmt.Errorf("user already manager")
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.GrantManagerPermissions(mockEnforcer, "user123", "organization", "abc-def", []string{"members"}, opts)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to add user to role group")
	})
}

func TestRevokeManagerPermissions(t *testing.T) {
	t.Run("Success - Revoke manager permissions", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.RemoveGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		opts := utils.DefaultPermissionOptions()
		err := utils.RevokeManagerPermissions(mockEnforcer, "user123", "organization", "abc-def", opts)

		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetRemoveGroupingPolicyCallCount())
	})
}

// ==========================================
// Integration Tests
// ==========================================

func TestPermissionsIntegration(t *testing.T) {
	t.Run("Complete workflow - Grant, use, and revoke permissions", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}
		mockEnforcer.RemoveGroupingPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		opts := utils.DefaultPermissionOptions()

		// 1. Grant entity access
		err := utils.GrantEntityAccess(mockEnforcer, "user123", "group", "abc-def", "GET|POST", opts)
		assert.NoError(t, err)

		// 2. Grant sub-resource access
		err = utils.GrantEntitySubResourceAccess(mockEnforcer, "group:abc-def", "group", "abc-def", "members", "GET", opts)
		assert.NoError(t, err)

		// 3. Revoke entity access
		err = utils.RevokeEntityAccess(mockEnforcer, "user123", "group", "abc-def", opts)
		assert.NoError(t, err)

		// Verify all operations succeeded
		assert.Equal(t, 1, mockEnforcer.GetAddGroupingPolicyCallCount())
		assert.Equal(t, 2, mockEnforcer.GetAddPolicyCallCount())
		assert.Equal(t, 1, mockEnforcer.GetRemoveGroupingPolicyCallCount())
	})
}

// ==========================================
// Options Tests
// ==========================================

func TestPermissionOptions(t *testing.T) {
	t.Run("DefaultPermissionOptions", func(t *testing.T) {
		opts := utils.DefaultPermissionOptions()

		assert.False(t, opts.LoadPolicyFirst, "LoadPolicyFirst should default to false")
		assert.False(t, opts.WarnOnError, "WarnOnError should default to false")
	})

	t.Run("Custom options - WarnOnError prevents error return", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return false, fmt.Errorf("simulated error")
		}

		// With WarnOnError: false (default)
		opts := utils.DefaultPermissionOptions()
		err := utils.AddPolicy(mockEnforcer, "user123", "/api/v1/groups/abc", "GET", opts)
		assert.Error(t, err, "Should return error when WarnOnError is false")

		// With WarnOnError: true
		opts.WarnOnError = true
		err = utils.AddPolicy(mockEnforcer, "user123", "/api/v1/groups/abc", "GET", opts)
		assert.NoError(t, err, "Should not return error when WarnOnError is true")
	})

	t.Run("LoadPolicyFirst triggers LoadPolicy", func(t *testing.T) {
		mockEnforcer := mocks.NewMockEnforcer()
		mockEnforcer.LoadPolicyFunc = func() error {
			return nil
		}
		mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
			return true, nil
		}

		// Without LoadPolicyFirst
		opts := utils.DefaultPermissionOptions()
		err := utils.AddPolicy(mockEnforcer, "user123", "/api/v1/groups/abc", "GET", opts)
		assert.NoError(t, err)
		assert.Equal(t, 0, mockEnforcer.GetLoadPolicyCallCount(), "LoadPolicy should not be called")

		// With LoadPolicyFirst
		mockEnforcer.Reset()
		opts.LoadPolicyFirst = true
		err = utils.AddPolicy(mockEnforcer, "user123", "/api/v1/groups/abc", "GET", opts)
		assert.NoError(t, err)
		assert.Equal(t, 1, mockEnforcer.GetLoadPolicyCallCount(), "LoadPolicy should be called once")
	})
}
