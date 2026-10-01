-- Migration 000004: Add authentication and RBAC fields to users table

-- Add a column for the secure storage of the bcrypt hash
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255) NOT NULL DEFAULT '';

-- Add a user role column (USER by default)
ALTER TABLE users ADD COLUMN IF NOT EXISTS role VARCHAR(50) NOT NULL DEFAULT 'USER';

-- Index for rapid filtering or role-based auditing
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
