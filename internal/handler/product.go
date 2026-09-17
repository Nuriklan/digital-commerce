package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/storage"
	"github.com/google/uuid"
)

type CreateProductRequest struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

type ProductHandler struct {
	storage *storage.MemoryStorage
}

func NewProductHandler(storage *storage.MemoryStorage) *ProductHandler {
	return &ProductHandler{storage: storage}
}

func (h *ProductHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req CreateProductRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	product, err := domain.NewProduct(req.Name, req.Price)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.storage.SaveProduct(product)
	respondJSON(w, http.StatusCreated, product)
}

func (h *ProductHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid product id format")
		return
	}

	product, err := h.storage.GetProductByID(id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			respondError(w, http.StatusNotFound, "product not found")
			return
		}
		respondError(w, http.StatusNotFound, "failed to get product")
		return
	}

	respondJSON(w, http.StatusOK, product)
}

func (h *ProductHandler) ListProducts(w http.ResponseWriter, r *http.Request) {
	limit := 10
	offset := 0

	query := r.URL.Query()
	if l := query.Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}
	if o := query.Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	products := h.storage.ListProducts(limit, offset)
	respondJSON(w, http.StatusOK, map[string]any{
		"items":  products,
		"limit":  limit,
		"offset": offset,
	})
}
