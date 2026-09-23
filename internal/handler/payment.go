package handler

import (
	"errors"
	"net/http"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/google/uuid"
)

type CreatePaymentRequest struct {
	OrderID uuid.UUID `json:"order_id"`
}

type PaymentHandler struct {
	service *service.PaymentService
}

func NewPaymentHandler(service *service.PaymentService) *PaymentHandler {
	return &PaymentHandler{service: service}
}

func (h *PaymentHandler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	var req CreatePaymentRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	payment, err := h.service.ProcessPayment(r.Context(), req.OrderID, idempotencyKey)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, domain.ErrOrderAlreadyPaid) ||
			errors.Is(err, domain.ErrOrderAlreadyCancelled) ||
			errors.Is(err, domain.ErrEmptyOrder) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, payment)
}

func (h *PaymentHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid payment id format")
		return
	}

	payment, err := h.service.GetPayment(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondError(w, http.StatusNotFound, "payment not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get payment")
		return
	}

	respondJSON(w, http.StatusOK, payment)
}
