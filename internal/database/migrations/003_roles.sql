ALTER TABLE users
    ADD COLUMN IF NOT EXISTS role VARCHAR(20) NOT NULL DEFAULT 'staff';

-- Semua user yang sudah ada sebelum fitur role (termasuk admin awal) jadi admin.
UPDATE users SET role = 'admin';

ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'manager', 'staff'));
