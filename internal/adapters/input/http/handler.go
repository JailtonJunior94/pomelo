package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jailtonjunior/pomelo/internal/application/ports"
	"github.com/jailtonjunior/pomelo/internal/domain"
)

// Handler wires HTTP routes to the use case.
type Handler struct {
	useCase ports.WebhookUseCase
}

func NewHandler(useCase ports.WebhookUseCase) *Handler {
	return &Handler{useCase: useCase}
}

// RegisterRoutes attaches all routes to the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhook/transactions", h.handleWebhook)
	mux.HandleFunc("GET /transactions/{id}", h.handleGetTransaction)
	mux.HandleFunc("GET /transactions", h.handleListTransactions)
	mux.HandleFunc("GET /health", h.handleHealth)
}

func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	var dto WebhookRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	cmd, err := dto.ToCommand()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "VALIDATION_ERROR")
		return
	}

	result, err := h.useCase.ProcessTransaction(r.Context(), cmd)
	if err != nil {
		h.handleDomainError(w, err, result)
		return
	}

	writeJSON(w, http.StatusOK, WebhookResponseDTO{
		TransactionID: result.TransactionID,
		Idempotent:    false,
		Message:       "transaction processed",
	})
}

func (h *Handler) handleDomainError(w http.ResponseWriter, err error, result ports.ProcessTransactionResult) {
	switch {
	case errors.Is(err, domain.ErrDuplicateIdempotencyKey):
		writeJSON(w, http.StatusOK, WebhookResponseDTO{
			TransactionID: result.TransactionID,
			Idempotent:    true,
			Message:       "duplicate event, already processed",
		})
	case errors.Is(err, domain.ErrTransactionNotFound):
		writeError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
	case errors.Is(err, domain.ErrExceedsOriginalAmount):
		writeError(w, http.StatusConflict, err.Error(), "EXCEEDS_ORIGINAL_AMOUNT")
	case errors.Is(err, domain.ErrPurchaseNotApproved):
		writeError(w, http.StatusConflict, err.Error(), "PURCHASE_NOT_APPROVED")
	case errors.Is(err, domain.ErrDuplicateTransactionID):
		writeError(w, http.StatusConflict, err.Error(), "DUPLICATE_TRANSACTION_ID")
	case errors.Is(err, domain.ErrAmountOutOfRange):
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "AMOUNT_OUT_OF_RANGE")
	case errors.Is(err, domain.ErrNegativeAmount):
		writeError(w, http.StatusBadRequest, err.Error(), "NEGATIVE_AMOUNT")
	case errors.Is(err, domain.ErrOriginalTransactionRequired):
		writeError(w, http.StatusBadRequest, err.Error(), "ORIGINAL_TRANSACTION_REQUIRED")
	case errors.Is(err, domain.ErrCurrencyMismatch):
		writeError(w, http.StatusBadRequest, err.Error(), "CURRENCY_MISMATCH")
	case errors.Is(err, domain.ErrInvalidTransactionType):
		writeError(w, http.StatusBadRequest, err.Error(), "INVALID_TRANSACTION_TYPE")
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error(), "INVALID_INPUT")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
	}
}

func (h *Handler) handleGetTransaction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tx, err := h.useCase.GetTransaction(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrTransactionNotFound) {
			writeError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, tx)
}

func (h *Handler) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	txs, err := h.useCase.ListTransactions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, txs)
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg, code string) {
	writeJSON(w, status, ErrorResponseDTO{Error: msg, Code: code})
}
