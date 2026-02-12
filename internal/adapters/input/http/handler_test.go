package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jailtonjunior/pomelo/internal/application/ports"
	"github.com/jailtonjunior/pomelo/internal/domain"
)

// --- Mock Use Case ---

type mockUseCase struct {
	processResult ports.ProcessTransactionResult
	processErr    error
	getTx         domain.Transaction
	getErr        error
	listTxs       []domain.Transaction
	listErr       error
}

func (m *mockUseCase) ProcessTransaction(_ context.Context, _ ports.ProcessTransactionCommand) (ports.ProcessTransactionResult, error) {
	return m.processResult, m.processErr
}

func (m *mockUseCase) GetTransaction(_ context.Context, _ string) (domain.Transaction, error) {
	return m.getTx, m.getErr
}

func (m *mockUseCase) ListTransactions(_ context.Context) ([]domain.Transaction, error) {
	return m.listTxs, m.listErr
}

// --- Helpers ---

func buildWebhookBody(txType, status, originalID string) []byte {
	dto := WebhookRequestDTO{}
	dto.ID = "tx1"
	dto.Type = txType
	dto.Status = status
	dto.OriginalTransactionID = originalID
	dto.Amount.Local.Total = 1000
	dto.Amount.Local.Currency = "BRL"
	dto.Amount.Transaction.Total = 1000
	dto.Amount.Transaction.Currency = "BRL"
	dto.Amount.Settlement.Total = 1000
	dto.Amount.Settlement.Currency = "BRL"
	dto.Amount.Original.Total = 1000
	dto.Amount.Original.Currency = "BRL"
	dto.Merchant.ID = "m1"
	dto.Merchant.Name = "Store"
	dto.Event.ID = "evt1"
	dto.Event.CreatedAt = "2024-01-01T00:00:00Z"
	dto.Event.IdempotencyKey = "idem1"
	dto.UserID = "u1"
	dto.CardID = "card1"
	dto.Country = "BR"
	dto.Currency = "BRL"
	b, _ := json.Marshal(dto)
	return b
}

func doPost(handler *Handler, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhook/transactions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)
	return w
}

// --- Tests ---

func TestWebhookPurchaseApproved(t *testing.T) {
	mock := &mockUseCase{processResult: ports.ProcessTransactionResult{TransactionID: "tx1"}}
	h := NewHandler(mock)
	w := doPost(h, buildWebhookBody("PURCHASE", "APPROVED", ""))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestWebhookDuplicateIdempotencyKey(t *testing.T) {
	mock := &mockUseCase{
		processResult: ports.ProcessTransactionResult{TransactionID: "tx1", Idempotent: true},
		processErr:    domain.ErrDuplicateIdempotencyKey,
	}
	h := NewHandler(mock)
	w := doPost(h, buildWebhookBody("PURCHASE", "APPROVED", ""))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp WebhookResponseDTO
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Idempotent {
		t.Error("expected idempotent=true")
	}
}

func TestWebhookTransactionNotFound(t *testing.T) {
	mock := &mockUseCase{processErr: domain.ErrTransactionNotFound}
	h := NewHandler(mock)
	w := doPost(h, buildWebhookBody("REFUND", "APPROVED", "tx-original"))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestWebhookExceedsOriginalAmount(t *testing.T) {
	mock := &mockUseCase{processErr: domain.ErrExceedsOriginalAmount}
	h := NewHandler(mock)
	w := doPost(h, buildWebhookBody("REFUND", "APPROVED", "tx-original"))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestWebhookPurchaseNotApproved(t *testing.T) {
	mock := &mockUseCase{processErr: domain.ErrPurchaseNotApproved}
	h := NewHandler(mock)
	w := doPost(h, buildWebhookBody("REFUND", "APPROVED", "tx-original"))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestWebhookAmountOutOfRange(t *testing.T) {
	mock := &mockUseCase{processErr: domain.ErrAmountOutOfRange}
	h := NewHandler(mock)
	w := doPost(h, buildWebhookBody("PURCHASE", "APPROVED", ""))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
	var resp ErrorResponseDTO
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Code != "AMOUNT_OUT_OF_RANGE" {
		t.Errorf("expected AMOUNT_OUT_OF_RANGE, got %s", resp.Code)
	}
}

func TestWebhookNegativeAmount(t *testing.T) {
	mock := &mockUseCase{processErr: domain.ErrNegativeAmount}
	h := NewHandler(mock)
	w := doPost(h, buildWebhookBody("PURCHASE", "APPROVED", ""))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestWebhookOriginalTransactionRequired(t *testing.T) {
	mock := &mockUseCase{processErr: domain.ErrOriginalTransactionRequired}
	h := NewHandler(mock)
	w := doPost(h, buildWebhookBody("REFUND", "APPROVED", "tx-original"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestWebhookInvalidTransactionType(t *testing.T) {
	mock := &mockUseCase{processErr: domain.ErrInvalidTransactionType}
	h := NewHandler(mock)
	w := doPost(h, buildWebhookBody("UNKNOWN_TYPE", "APPROVED", ""))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp ErrorResponseDTO
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Code != "INVALID_TRANSACTION_TYPE" {
		t.Errorf("expected INVALID_TRANSACTION_TYPE, got %s", resp.Code)
	}
}

func TestWebhookDuplicateTransactionID(t *testing.T) {
	mock := &mockUseCase{processErr: domain.ErrDuplicateTransactionID}
	h := NewHandler(mock)
	w := doPost(h, buildWebhookBody("PURCHASE", "APPROVED", ""))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
	var resp ErrorResponseDTO
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Code != "DUPLICATE_TRANSACTION_ID" {
		t.Errorf("expected DUPLICATE_TRANSACTION_ID, got %s", resp.Code)
	}
}

func TestWebhookInternalError(t *testing.T) {
	mock := &mockUseCase{processErr: errors.New("unexpected")}
	h := NewHandler(mock)
	w := doPost(h, buildWebhookBody("PURCHASE", "APPROVED", ""))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestWebhookInvalidStatus(t *testing.T) {
	mock := &mockUseCase{}
	h := NewHandler(mock)
	body := buildWebhookBody("PURCHASE", "PENDING", "") // invalid status
	w := doPost(h, body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp ErrorResponseDTO
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Code != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %s", resp.Code)
	}
}

func TestWebhookBadJSON(t *testing.T) {
	mock := &mockUseCase{}
	h := NewHandler(mock)
	req := httptest.NewRequest(http.MethodPost, "/webhook/transactions", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetTransactionNotFound(t *testing.T) {
	mock := &mockUseCase{getErr: domain.ErrTransactionNotFound}
	h := NewHandler(mock)
	req := httptest.NewRequest(http.MethodGet, "/transactions/tx999", nil)
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHealth(t *testing.T) {
	mock := &mockUseCase{}
	h := NewHandler(mock)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
