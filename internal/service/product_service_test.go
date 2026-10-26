package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"first-project/internal/models"
)

// fakeRepo adalah repository palsu di memori, sehingga test tidak butuh database.
type fakeRepo struct {
	data   map[int64]models.Product
	nextID int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{data: map[int64]models.Product{}, nextID: 1}
}

func (f *fakeRepo) Create(_ context.Context, in models.ProductInput) (*models.Product, error) {
	p := models.Product{ID: f.nextID, Name: in.Name, Description: in.Description,
		Price: in.Price, Stock: in.Stock, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	f.data[p.ID] = p
	f.nextID++
	return &p, nil
}

func (f *fakeRepo) GetByID(_ context.Context, id int64) (*models.Product, error) {
	p, ok := f.data[id]
	if !ok {
		return nil, models.ErrNotFound
	}
	return &p, nil
}

func (f *fakeRepo) List(_ context.Context, p models.ListParams) ([]models.Product, int, error) {
	out := make([]models.Product, 0, len(f.data))
	for _, v := range f.data {
		out = append(out, v)
	}
	return out, len(out), nil
}

func (f *fakeRepo) Update(_ context.Context, id int64, in models.ProductInput) (*models.Product, error) {
	p, ok := f.data[id]
	if !ok {
		return nil, models.ErrNotFound
	}
	p.Name, p.Description, p.Price, p.Stock = in.Name, in.Description, in.Price, in.Stock
	f.data[id] = p
	return &p, nil
}

func (f *fakeRepo) Delete(_ context.Context, id int64) error {
	if _, ok := f.data[id]; !ok {
		return models.ErrNotFound
	}
	delete(f.data, id)
	return nil
}

func TestCreate_Validation(t *testing.T) {
	tests := []struct {
		name      string
		input     models.ProductInput
		wantField string // kosong = harus sukses
	}{
		{"valid", models.ProductInput{Name: "Laptop", Price: 5000000, Stock: 3}, ""},
		{"nama terlalu pendek", models.ProductInput{Name: "ab", Price: 1000}, "name"},
		{"harga nol", models.ProductInput{Name: "Laptop", Price: 0}, "price"},
		{"stok negatif", models.ProductInput{Name: "Laptop", Price: 1000, Stock: -1}, "stock"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewProductService(newFakeRepo())
			_, err := svc.Create(context.Background(), tt.input)

			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("tidak diharapkan error: %v", err)
				}
				return
			}

			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("harapannya ValidationError, dapat: %v", err)
			}
			if _, ok := ve.Fields[tt.wantField]; !ok {
				t.Errorf("harapannya error pada field %q, dapat %v", tt.wantField, ve.Fields)
			}
		})
	}
}

func TestCRUDFlow(t *testing.T) {
	ctx := context.Background()
	svc := NewProductService(newFakeRepo())

	created, err := svc.Create(ctx, models.ProductInput{Name: "  Headset  ", Price: 200000, Stock: 5})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Headset" {
		t.Errorf("nama harus di-trim, dapat %q", created.Name)
	}

	updated, err := svc.Update(ctx, created.ID, models.ProductInput{Name: "Headset Pro", Price: 300000, Stock: 2})
	if err != nil || updated.Price != 300000 {
		t.Fatalf("update gagal: %v %+v", err, updated)
	}

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetByID(ctx, created.ID); !errors.Is(err, models.ErrNotFound) {
		t.Errorf("harapannya ErrNotFound, dapat %v", err)
	}
}

func TestList_DefaultPagination(t *testing.T) {
	svc := NewProductService(newFakeRepo())
	if _, _, err := svc.List(context.Background(), models.ListParams{Page: -5, Limit: 9999}); err != nil {
		t.Fatal(err)
	}
}
