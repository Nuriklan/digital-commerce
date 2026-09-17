package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/storage"
	"github.com/Nuriklan/digital-commerce/internal/worker"
)

type OrderItemRequest struct {
	ProductID uuid.UUID `json:"product_id"`
	Quantity  int       `json:"quantity"`
}

type CreateOrderRequest struct {
	UserID uuid.UUID          `json:"user_id"`
	Items  []OrderItemRequest `json:"items"`
}

type OrderHandler struct {
	storage *storage.MemoryStorage
	pool    *worker.Pool
}

func NewOrderHandler(storage *storage.MemoryStorage, pool *worker.Pool) *OrderHandler {
	return &OrderHandler{
		storage: storage,
		pool:    pool,
	}
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if _, err := h.storage.GetUserByID(req.UserID); err != nil {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	order := domain.NewOrder(req.UserID)

	for _, itemReq := range req.Items {
		if itemReq.Quantity <= 0 {
			respondError(w, http.StatusBadRequest, "quantity must be greater than 0")
			return
		}
		product, err := h.storage.GetProductByID(itemReq.ProductID)
		if err != nil {
			respondError(w, http.StatusNotFound, "product not found: "+itemReq.ProductID.String())
			return
		}
		order.AddItem(product, itemReq.Quantity)
	}

	h.storage.SaveOrder(order)

	if h.pool != nil {
		h.pool.Submit(r.Context(), worker.Job{
			Name:    "send_notification",
			Payload: order,
		})
		h.pool.Submit(r.Context(), worker.Job{
			Name:    "update_analytics",
			Payload: order,
		})
	}

	respondJSON(w, http.StatusCreated, order)
}

func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid order id format")
		return
	}

	order, err := h.storage.GetOrderByID(id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			respondError(w, http.StatusNotFound, "order not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get order")
		return
	}

	respondJSON(w, http.StatusOK, order)
}

func (h *OrderHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid order id format")
		return
	}

	order, err := h.storage.GetOrderByID(id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			respondError(w, http.StatusNotFound, "order not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get order")
		return
	}

	if err := order.Cancel(); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.storage.SaveOrder(order)
	respondJSON(w, http.StatusOK, order)
}
