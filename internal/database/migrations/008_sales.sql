CREATE TABLE IF NOT EXISTS sales (
    id BIGSERIAL PRIMARY KEY,
    customer_name VARCHAR(100) NOT NULL DEFAULT '',
    payment_method VARCHAR(20) NOT NULL DEFAULT 'tunai'
        CHECK (payment_method IN ('tunai', 'transfer', 'qris')),
    status VARCHAR(20) NOT NULL DEFAULT 'completed'
        CHECK (status IN ('completed', 'cancelled')),
    notes TEXT NOT NULL DEFAULT '',
    total BIGINT NOT NULL DEFAULT 0,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sale_items (
    id BIGSERIAL PRIMARY KEY,
    sale_id BIGINT NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity INT NOT NULL CHECK (quantity > 0),
    unit_price BIGINT NOT NULL CHECK (unit_price >= 0)
);

CREATE INDEX IF NOT EXISTS idx_sale_items_sale ON sale_items(sale_id);

-- Penjualan otomatis tercatat sebagai kas masuk "Penjualan".
ALTER TABLE cash_transactions
    ADD COLUMN IF NOT EXISTS sale_id BIGINT UNIQUE REFERENCES sales(id) ON DELETE SET NULL;
