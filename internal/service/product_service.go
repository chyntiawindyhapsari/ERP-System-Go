package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"first-project/internal/models"
	"first-project/internal/repository"
)

// ValidationError berisi pesan error per field.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return "validasi gagal" }

// ProductService berisi business logic (validasi, aturan pagination, dll).
type ProductService struct {
	repo repository.ProductRepository
}

func NewProductService(repo repository.ProductRepository) *ProductService {
	return &ProductService{repo: repo}
}

func (s *ProductService) Create(ctx context.Context, in models.ProductInput) (*models.Product, error) {
	in, err := validate(in)
	if err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, in)
}

func (s *ProductService) GetByID(ctx context.Context, id int64) (*models.Product, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ProductService) List(ctx context.Context, p models.ListParams) ([]models.Product, int, error) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Limit < 1 {
		p.Limit = 10
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	p.Search = strings.TrimSpace(p.Search)
	return s.repo.List(ctx, p)
}

func (s *ProductService) Update(ctx context.Context, id int64, in models.ProductInput) (*models.Product, error) {
	in, err := validate(in)
	if err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, id, in)
}

func (s *ProductService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// validate membersihkan input dan memeriksa aturan bisnis.
func validate(in models.ProductInput) (models.ProductInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)

	errs := map[string]string{}

	switch n := utf8.RuneCountInString(in.Name); {
	case n < 3:
		errs["name"] = "nama minimal 3 karakter"
	case n > 100:
		errs["name"] = "nama maksimal 100 karakter"
	}
	if utf8.RuneCountInString(in.Description) > 500 {
		errs["description"] = "deskripsi maksimal 500 karakter"
	}
	if in.Price <= 0 {
		errs["price"] = "harga harus lebih dari 0"
	}
	if in.Stock < 0 {
		errs["stock"] = "stok tidak boleh negatif"
	}

	if len(errs) > 0 {
		return in, &ValidationError{Fields: errs}
	}
	return in, nil
}
