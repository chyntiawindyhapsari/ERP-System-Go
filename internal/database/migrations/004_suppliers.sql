CREATE TABLE IF NOT EXISTS suppliers (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    contact_person VARCHAR(100) NOT NULL DEFAULT '',
    email VARCHAR(120) NOT NULL DEFAULT '',
    phone VARCHAR(30) NOT NULL DEFAULT '',
    city VARCHAR(80) NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_suppliers_name ON suppliers (LOWER(name));

INSERT INTO suppliers (name, contact_person, email, phone, city, address) VALUES
    ('PT Sumber Elektronik', 'Andi Wijaya', 'andi@sumberelektronik.example', '021-5550101', 'Jakarta', 'Jl. Mangga Dua No. 12'),
    ('CV Maju Perkasa', 'Siti Rahma', 'siti@majuperkasa.example', '031-5550202', 'Surabaya', 'Jl. Rungkut Industri 8'),
    ('Toko Aksesoris Nusantara', 'Budi Hartono', 'budi@aksesoris.example', '022-5550303', 'Bandung', 'Jl. Cihampelas 45');
