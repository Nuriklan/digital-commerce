package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/storage"
)

type CreatePaymentRequest struct {
	OrderID uuid.UUID `json:"order_id"`
}

type PaymentHandler struct {
	storage *storage.MemoryStorage
}

func NewPaymentHandler(storage *storage.MemoryStorage) *PaymentHandler {
	return &PaymentHandler{storage: storage}
}

func (h *PaymentHandler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	var req CreatePaymentRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	order, err := h.storage.GetOrderByID(req.OrderID)
	if err != nil {
		respondError(w, http.StatusNotFound, "order not found")
		return
	}

	total := order.CalculateTotal()
	payment, err := domain.NewPayment(order.ID, total)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := order.Pay(); err != nil {
		_ = payment.MarkFailed()
		h.storage.SavePayment(payment)
		respondError(w, http.StatusBadRequest, "payment failed: "+err.Error())
		return
	}

	_ = payment.MarkSuccess()
	h.storage.SaveOrder(order)
	h.storage.SavePayment(payment)

	respondJSON(w, http.StatusCreated, payment)
}

func (h *PaymentHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid payment id format")
		return
	}

	payment, err := h.storage.GetPaymentByID(id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			respondError(w, http.StatusNotFound, "payment not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get payment")
		return
	}

	respondJSON(w, http.StatusOK, payment)
}
