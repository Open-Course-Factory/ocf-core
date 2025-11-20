package dto

import (
	"encoding/json"
	"testing"
)

func TestFlexibleInt_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		jsonInput string
		wantValue int
		wantError bool
	}{
		{
			name:      "int value",
			jsonInput: `{"status": 0}`,
			wantValue: 0,
			wantError: false,
		},
		{
			name:      "int value non-zero",
			jsonInput: `{"status": 1}`,
			wantValue: 1,
			wantError: false,
		},
		{
			name:      "string value zero",
			jsonInput: `{"status": "0"}`,
			wantValue: 0,
			wantError: false,
		},
		{
			name:      "string value non-zero",
			jsonInput: `{"status": "1"}`,
			wantValue: 1,
			wantError: false,
		},
		{
			name:      "invalid string value",
			jsonInput: `{"status": "invalid"}`,
			wantValue: 0,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response struct {
				Status FlexibleInt `json:"status"`
			}

			err := json.Unmarshal([]byte(tt.jsonInput), &response)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if int(response.Status) != tt.wantValue {
				t.Errorf("got status = %d, want %d", response.Status, tt.wantValue)
			}
		})
	}
}

func TestTerminalTrainerSessionResponse_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		jsonInput string
		wantError bool
	}{
		{
			name: "status as int",
			jsonInput: `{
				"id": "test-session-1",
				"status": 0,
				"expires_at": 1234567890,
				"created_at": 1234567890,
				"machine_size": "M"
			}`,
			wantError: false,
		},
		{
			name: "status as string",
			jsonInput: `{
				"id": "test-session-2",
				"status": "0",
				"expires_at": 1234567890,
				"created_at": 1234567890,
				"machine_size": "L"
			}`,
			wantError: false,
		},
		{
			name: "status as non-zero int",
			jsonInput: `{
				"id": "test-session-3",
				"status": 1,
				"expires_at": 1234567890
			}`,
			wantError: false,
		},
		{
			name: "status as non-zero string",
			jsonInput: `{
				"id": "test-session-4",
				"status": "2",
				"expires_at": 1234567890
			}`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response TerminalTrainerSessionResponse

			err := json.Unmarshal([]byte(tt.jsonInput), &response)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify basic parsing worked
			if response.SessionID == "" {
				t.Errorf("SessionID should not be empty")
			}
		})
	}
}
