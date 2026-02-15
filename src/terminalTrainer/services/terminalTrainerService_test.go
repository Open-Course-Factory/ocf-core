package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"soli/formations/src/terminalTrainer/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetInstanceTypes_WithoutBackend(t *testing.T) {
	var capturedPath string
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]dto.InstanceType{
			{Name: "Alpine", Prefix: "alp", Description: "Alpine Linux", Size: "S"},
		})
	}))
	defer server.Close()

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		adminKey:   "test-admin-key",
	}

	instances, err := svc.GetInstanceTypes("")
	require.NoError(t, err)
	assert.Len(t, instances, 1)
	assert.Equal(t, "Alpine", instances[0].Name)
	assert.Equal(t, "/1.0/instances", capturedPath)
	assert.Empty(t, capturedQuery, "no query params when backend is empty")
}

func TestGetInstanceTypes_WithBackend(t *testing.T) {
	var capturedPath string
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]dto.InstanceType{
			{Name: "Alpine", Prefix: "alp", Description: "Alpine Linux", Size: "S"},
			{Name: "KVM", Prefix: "kvm", Description: "KVM Instance", Size: "M"},
		})
	}))
	defer server.Close()

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		adminKey:   "test-admin-key",
	}

	instances, err := svc.GetInstanceTypes("cloud1")
	require.NoError(t, err)
	assert.Len(t, instances, 2)
	assert.Equal(t, "/1.0/instances", capturedPath)
	assert.Equal(t, "backend=cloud1", capturedQuery)
}

func TestGetInstanceTypes_WithTerminalType(t *testing.T) {
	var capturedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]dto.InstanceType{})
	}))
	defer server.Close()

	svc := &terminalTrainerService{
		baseURL:      server.URL,
		apiVersion:   "1.0",
		adminKey:     "test-admin-key",
		terminalType: "kvm",
	}

	_, err := svc.GetInstanceTypes("")
	require.NoError(t, err)
	// When terminalType is set and no instanceType override, buildAPIPath includes it
	assert.Equal(t, "/1.0/kvm/instances", capturedPath)
}

func TestGetInstanceTypes_AdminKeyHeader(t *testing.T) {
	var capturedAdminKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAdminKey = r.Header.Get("X-Admin-Key")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]dto.InstanceType{})
	}))
	defer server.Close()

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		adminKey:   "my-secret-key",
	}

	_, err := svc.GetInstanceTypes("")
	require.NoError(t, err)
	assert.Equal(t, "my-secret-key", capturedAdminKey)
}

func TestGetInstanceTypes_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	svc := &terminalTrainerService{
		baseURL:    server.URL,
		apiVersion: "1.0",
		adminKey:   "test-admin-key",
	}

	instances, err := svc.GetInstanceTypes("")
	assert.Error(t, err)
	assert.Nil(t, instances)
}
