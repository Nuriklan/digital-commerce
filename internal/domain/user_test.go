package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/domain"
)

func TestNewUserWithPassword_Success(t *testing.T) {
	user, err := domain.NewUserWithPassword("Alice", "alice@example.com", "secret123", domain.RoleUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.Name != "Alice" || user.Email != "alice@example.com" {
		t.Errorf("unexpected user data: %+v", user)
	}
	if user.Role != domain.RoleUser {
		t.Errorf("expected role %s, got %s", domain.RoleUser, user.Role)
	}
	if user.PasswordHash == "" || user.PasswordHash == "secret123" {
		t.Errorf("password must be securely hashed, got: %s", user.PasswordHash)
	}

	if !user.CheckPassword("secret123") {
		t.Errorf("expected password to match")
	}
	if user.CheckPassword("wrong_password") {
		t.Errorf("expected wrong password to fail")
	}
}

func TestNewUserWithPassword_ValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		userName string
		email    string
		password string
		role     domain.Role
		wantErr  error
	}{
		{
			name:     "empty name",
			userName: "",
			email:    "test@example.com",
			password: "password123",
			role:     domain.RoleUser,
			wantErr:  domain.ErrEmptyName,
		},
		{
			name:     "empty email",
			userName: "Bob",
			email:    "",
			password: "password123",
			role:     domain.RoleUser,
			wantErr:  domain.ErrEmptyEmail,
		},
		{
			name:     "short password",
			userName: "Bob",
			email:    "bob@example.com",
			password: "123",
			role:     domain.RoleUser,
			wantErr:  domain.ErrPasswordTooShort,
		},
		{
			name:     "invalid role",
			userName: "Bob",
			email:    "bob@example.com",
			password: "password123",
			role:     domain.Role("SUPERUSER"),
			wantErr:  domain.ErrInvalidRole,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewUserWithPassword(tt.userName, tt.email, tt.password, tt.role)
			if err != tt.wantErr {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestUser_PasswordHashNotExposedInJSON(t *testing.T) {
	user, err := domain.NewUserWithPassword("Admin", "admin@example.com", "admin_secret", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	data, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("failed to marshal user: %v", err)
	}

	jsonStr := string(data)
	if jsonStr == "" {
		t.Fatalf("json is empty")
	}

	if strings.Contains(jsonStr, "admin_secret") || strings.Contains(jsonStr, user.PasswordHash) {
		t.Errorf("sensitive password data leaked in JSON: %s", jsonStr)
	}
}
