ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'gudang', 'supplier', 'kasir', 'keuangan'));

CREATE TABLE IF NOT EXISTS cash_transactions (
    id BIGSERIAL PRIMARY KEY,
    tx_date DATE NOT NULL DEFAULT CURRENT_DATE,
    type VARCHAR(3) NOT NULL CHECK (type IN ('in', 'out')),
    category VARCHAR(40) NOT NULL,
    description VARCHAR(200) NOT NULL DEFAULT '',
    amount BIGINT NOT NULL CHECK (amount > 0),
    -- diisi otomatis saat pembelian diterima (satu pembelian = satu pengeluaran)
    purchase_order_id BIGINT UNIQUE REFERENCES purchase_orders(id) ON DELETE SET NULL,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cash_tx_date ON cash_transactions(tx_date DESC, id DESC);
