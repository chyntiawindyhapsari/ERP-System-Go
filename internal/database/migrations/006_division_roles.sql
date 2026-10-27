-- Role berbasis divisi: admin (semua), gudang (produk & persediaan),
-- supplier (data supplier), kasir (pembelian).
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;

UPDATE users SET role = 'gudang' WHERE role = 'manager';
UPDATE users SET role = 'kasir'  WHERE role = 'staff';

ALTER TABLE users ALTER COLUMN role SET DEFAULT 'kasir';
ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'gudang', 'supplier', 'kasir'));
