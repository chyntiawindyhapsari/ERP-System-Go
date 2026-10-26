package models

import (
	"errors"
	"time"
)

// ErrNotFound dikembalikan jika data tidak ada di database.
var ErrNotFound = errors.New("data tidak ditemukan")

// Product adalah representasi satu baris tabel products.
type Product struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       int64     `json:"price"` // dalam Rupiah (integer, hindari float untuk uang)
	Stock       int       `json:"stock"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProductInput adalah body JSON untuk create & update.
type ProductInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       int64  `json:"price"`
	Stock       int    `json:"stock"`
}

// ListParams adalah parameter query untuk daftar produk.
type ListParams struct {
	Page   int
	Limit  int
	Search string
}

// Offset menghitung jumlah baris yang dilewati untuk pagination.
func (p ListParams) Offset() int {
	return (p.Page - 1) * p.Limit
}
