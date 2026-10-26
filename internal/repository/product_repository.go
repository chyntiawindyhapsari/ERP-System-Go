package repository

import (
	"context"
	"errors"

	"first-project/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProductRepository adalah kontrak akses data produk.
// Dengan interface, service mudah dites memakai data palsu (fake).
type ProductRepository interface {
	Create(ctx context.Context, in models.ProductInput) (*models.Product, error)
	GetByID(ctx context.Context, id int64) (*models.Product, error)
	List(ctx context.Context, p models.ListParams) ([]models.Product, int, error)
	Update(ctx context.Context, id int64, in models.ProductInput) (*models.Product, error)
	Delete(ctx context.Context, id int64) error
}

type productRepository struct {
	db *pgxpool.Pool
}

// NewProductRepository membuat implementasi PostgreSQL.
func NewProductRepository(db *pgxpool.Pool) ProductRepository {
	return &productRepository{db: db}
}

const productColumns = `id, name, description, price, stock, created_at, updated_at`

func scanProduct(row pgx.Row) (*models.Product, error) {
	var p models.Product
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

// Create -> INSERT
func (r *productRepository) Create(ctx context.Context, in models.ProductInput) (*models.Product, error) {
	query := `INSERT INTO products (name, description, price, stock)
	          VALUES ($1, $2, $3, $4)
	          RETURNING ` + productColumns
	return scanProduct(r.db.QueryRow(ctx, query, in.Name, in.Description, in.Price, in.Stock))
}

// GetByID -> SELECT satu baris
func (r *productRepository) GetByID(ctx context.Context, id int64) (*models.Product, error) {
	query := `SELECT ` + productColumns + ` FROM products WHERE id = $1`
	return scanProduct(r.db.QueryRow(ctx, query, id))
}

// List -> SELECT banyak baris + search + pagination
func (r *productRepository) List(ctx context.Context, p models.ListParams) ([]models.Product, int, error) {
	var total int
	countQuery := `SELECT COUNT(*) FROM products
	               WHERE $1::text = '' OR name ILIKE '%' || $1::text || '%'`
	if err := r.db.QueryRow(ctx, countQuery, p.Search).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := `SELECT ` + productColumns + ` FROM products
	              WHERE $1::text = '' OR name ILIKE '%' || $1::text || '%'
	              ORDER BY id DESC
	              LIMIT $2 OFFSET $3`
	rows, err := r.db.Query(ctx, listQuery, p.Search, p.Limit, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	products := make([]models.Product, 0, p.Limit)
	for rows.Next() {
		var pr models.Product
		if err := rows.Scan(&pr.ID, &pr.Name, &pr.Description, &pr.Price, &pr.Stock, &pr.CreatedAt, &pr.UpdatedAt); err != nil {
			return nil, 0, err
		}
		products = append(products, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

// Update -> UPDATE
func (r *productRepository) Update(ctx context.Context, id int64, in models.ProductInput) (*models.Product, error) {
	query := `UPDATE products
	          SET name = $1, description = $2, price = $3, stock = $4, updated_at = NOW()
	          WHERE id = $5
	          RETURNING ` + productColumns
	return scanProduct(r.db.QueryRow(ctx, query, in.Name, in.Description, in.Price, in.Stock, id))
}

// Delete -> DELETE
func (r *productRepository) Delete(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM products WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return models.ErrNotFound
	}
	return nil
}
