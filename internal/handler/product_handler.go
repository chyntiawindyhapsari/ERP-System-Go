package handler

import (
	"net/http"
	"strconv"

	"first-project/internal/models"
	"first-project/internal/service"
)

type ProductHandler struct {
	svc *service.ProductService
}

func NewProductHandler(svc *service.ProductService) *ProductHandler {
	return &ProductHandler{svc: svc}
}

// Register mendaftarkan route. Pola "METHOD /path" tersedia sejak Go 1.22.
func (h *ProductHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/products", h.Create)
	mux.HandleFunc("GET /api/v1/products", h.List)
	mux.HandleFunc("GET /api/v1/products/{id}", h.GetByID)
	mux.HandleFunc("PUT /api/v1/products/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/products/{id}", h.Delete)
}

func parseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in models.ProductInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	product, err := h.svc.Create(r.Context(), in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, Response{Success: true, Message: "produk berhasil dibuat", Data: product})
}

func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))

	params := models.ListParams{Page: page, Limit: limit, Search: q.Get("search")}
	products, total, err := h.svc.List(r.Context(), params)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Hitung ulang dengan nilai default yang sama seperti di service
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Limit < 1 {
		params.Limit = 10
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	totalPages := (total + params.Limit - 1) / params.Limit

	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Data:    products,
		Meta:    Meta{Page: params.Page, Limit: params.Limit, Total: total, TotalPages: totalPages},
	})
}

func (h *ProductHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}

	product, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Data: product})
}

func (h *ProductHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}

	var in models.ProductInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	product, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Message: "produk berhasil diperbarui", Data: product})
}

func (h *ProductHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Message: "produk berhasil dihapus"})
}
