package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jailtonjunior/pomelo/internal/application/ports"
	"github.com/jailtonjunior/pomelo/internal/domain"
)

// --- Mock Repository ---

type mockRepo struct {
	transactions    map[string]domain.Transaction
	adjustments     map[string][]domain.Adjustment
	idempotencyKeys map[string]string
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		transactions:    make(map[string]domain.Transaction),
		adjustments:     make(map[string][]domain.Adjustment),
		idempotencyKeys: make(map[string]string),
	}
}

func (r *mockRepo) SaveTransaction(_ context.Context, tx domain.Transaction) error {
	if _, exists := r.idempotencyKeys[tx.Event.IdempotencyKey]; exists {
		return domain.ErrDuplicateIdempotencyKey
	}
	if _, exists := r.transactions[tx.ID]; exists {
		return domain.ErrDuplicateTransactionID
	}
	r.idempotencyKeys[tx.Event.IdempotencyKey] = tx.ID
	r.transactions[tx.ID] = tx
	return nil
}

func (r *mockRepo) SaveAdjustment(_ context.Context, adj domain.Adjustment) error {
	if _, exists := r.idempotencyKeys[adj.Event.IdempotencyKey]; exists {
		return domain.ErrDuplicateIdempotencyKey
	}
	r.idempotencyKeys[adj.Event.IdempotencyKey] = adj.ID
	r.adjustments[adj.OriginalTransactionID] = append(r.adjustments[adj.OriginalTransactionID], adj)
	return nil
}

func (r *mockRepo) GetTransactionByID(_ context.Context, id string) (domain.Transaction, error) {
	tx, ok := r.transactions[id]
	if !ok {
		return domain.Transaction{}, domain.ErrTransactionNotFound
	}
	return tx, nil
}

func (r *mockRepo) GetAdjustmentsByTransactionID(_ context.Context, originalTxID string) ([]domain.Adjustment, error) {
	return r.adjustments[originalTxID], nil
}

func (r *mockRepo) GetByIdempotencyKey(_ context.Context, key string) (string, bool) {
	id, ok := r.idempotencyKeys[key]
	return id, ok
}

func (r *mockRepo) ListTransactions(_ context.Context) ([]domain.Transaction, error) {
	result := make([]domain.Transaction, 0, len(r.transactions))
	for _, tx := range r.transactions {
		result = append(result, tx)
	}
	return result, nil
}

// --- Helpers ---

func makePurchaseCmd(id, status, idemKey string, amount int64) ports.ProcessTransactionCommand {
	return ports.ProcessTransactionCommand{
		TransactionID:      id,
		TransactionType:    "PURCHASE",
		TransactionStatus:  status,
		LocalAmount:        amount,
		LocalCurrency:      "BRL",
		TxAmount:           amount,
		TxCurrency:         "BRL",
		SettlementAmount:   amount,
		SettlementCurrency: "BRL",
		OriginalAmount:     amount,
		OriginalCurrency:   "BRL",
		MerchantID:         "m1",
		MerchantName:       "Store",
		EventID:            "evt-" + id,
		EventCreatedAt:     time.Now(),
		IdempotencyKey:     idemKey,
		UserID:             "u1",
		CardID:             "card1",
		Country:            "BR",
		Currency:           "BRL",
		PointOfSale:        "POS",
	}
}

func makeAdjustCmd(id, txType, status, originalID, idemKey string, amount int64) ports.ProcessTransactionCommand {
	return ports.ProcessTransactionCommand{
		TransactionID:         id,
		TransactionType:       txType,
		TransactionStatus:     status,
		OriginalTransactionID: originalID,
		LocalAmount:           amount,
		LocalCurrency:         "BRL",
		TxAmount:              amount,
		TxCurrency:            "BRL",
		SettlementAmount:      amount,
		SettlementCurrency:    "BRL",
		OriginalAmount:        amount,
		OriginalCurrency:      "BRL",
		MerchantID:            "m1",
		MerchantName:          "Store",
		EventID:               "evt-" + id,
		EventCreatedAt:        time.Now(),
		IdempotencyKey:        idemKey,
		UserID:                "u1",
		CardID:                "card1",
		Country:               "BR",
		Currency:              "BRL",
		PointOfSale:           "POS",
	}
}

// --- Tests ---

func TestProcessPurchaseApproved(t *testing.T) {
	svc := NewService(newMockRepo())
	result, err := svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TransactionID != "tx1" {
		t.Errorf("expected tx1, got %s", result.TransactionID)
	}
	if result.Idempotent {
		t.Error("should not be idempotent")
	}
}

func TestProcessPurchaseRejected(t *testing.T) {
	svc := NewService(newMockRepo())
	result, err := svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "REJECTED", "idem1", 1000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TransactionID != "tx1" {
		t.Errorf("expected tx1, got %s", result.TransactionID)
	}
}

func TestProcessDuplicateIdempotencyKey(t *testing.T) {
	svc := NewService(newMockRepo())
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	result, err := svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	if !errors.Is(err, domain.ErrDuplicateIdempotencyKey) {
		t.Errorf("expected ErrDuplicateIdempotencyKey, got %v", err)
	}
	if !result.Idempotent {
		t.Error("should be idempotent")
	}
}

func TestProcessReversalTotal(t *testing.T) {
	svc := NewService(newMockRepo())
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	result, err := svc.ProcessTransaction(context.Background(), makeAdjustCmd("adj1", "REVERSAL_PURCHASE", "APPROVED", "tx1", "idem-adj1", 1000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TransactionID != "adj1" {
		t.Errorf("expected adj1, got %s", result.TransactionID)
	}
}

func TestProcessReversalPartial(t *testing.T) {
	svc := NewService(newMockRepo())
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	_, err := svc.ProcessTransaction(context.Background(), makeAdjustCmd("adj1", "REVERSAL_PURCHASE", "APPROVED", "tx1", "idem-adj1", 500))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessRefundTotal(t *testing.T) {
	svc := NewService(newMockRepo())
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	_, err := svc.ProcessTransaction(context.Background(), makeAdjustCmd("adj1", "REFUND", "APPROVED", "tx1", "idem-adj1", 1000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessRefundPartialMultiple(t *testing.T) {
	svc := NewService(newMockRepo())
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	svc.ProcessTransaction(context.Background(), makeAdjustCmd("adj1", "REFUND", "APPROVED", "tx1", "idem-adj1", 400))
	_, err := svc.ProcessTransaction(context.Background(), makeAdjustCmd("adj2", "REFUND", "APPROVED", "tx1", "idem-adj2", 400))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessRefundExceedsAmount(t *testing.T) {
	svc := NewService(newMockRepo())
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	_, err := svc.ProcessTransaction(context.Background(), makeAdjustCmd("adj1", "REFUND", "APPROVED", "tx1", "idem-adj1", 1500))
	if !errors.Is(err, domain.ErrExceedsOriginalAmount) {
		t.Errorf("expected ErrExceedsOriginalAmount, got %v", err)
	}
}

func TestProcessReversalAfterPartialRefund(t *testing.T) {
	svc := NewService(newMockRepo())
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	svc.ProcessTransaction(context.Background(), makeAdjustCmd("adj1", "REFUND", "APPROVED", "tx1", "idem-adj1", 600))
	_, err := svc.ProcessTransaction(context.Background(), makeAdjustCmd("adj2", "REVERSAL_PURCHASE", "APPROVED", "tx1", "idem-adj2", 1000))
	if !errors.Is(err, domain.ErrExceedsOriginalAmount) {
		t.Errorf("expected ErrExceedsOriginalAmount, got %v", err)
	}
}

func TestProcessOutOfOrder(t *testing.T) {
	svc := NewService(newMockRepo())
	_, err := svc.ProcessTransaction(context.Background(), makeAdjustCmd("adj1", "REFUND", "APPROVED", "tx1", "idem-adj1", 500))
	if !errors.Is(err, domain.ErrTransactionNotFound) {
		t.Errorf("expected ErrTransactionNotFound, got %v", err)
	}
}

func TestProcessDuplicateTransactionID(t *testing.T) {
	svc := NewService(newMockRepo())
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	// Same tx ID, different idempotency key (different event)
	_, err := svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem2", 1000))
	if !errors.Is(err, domain.ErrDuplicateTransactionID) {
		t.Errorf("expected ErrDuplicateTransactionID, got %v", err)
	}
}

func TestProcessAdjustmentMissingOriginalID(t *testing.T) {
	svc := NewService(newMockRepo())
	cmd := makeAdjustCmd("adj1", "REFUND", "APPROVED", "", "idem-adj1", 500) // empty original ID
	_, err := svc.ProcessTransaction(context.Background(), cmd)
	if !errors.Is(err, domain.ErrOriginalTransactionRequired) {
		t.Errorf("expected ErrOriginalTransactionRequired, got %v", err)
	}
}

func TestProcessAdjustmentOnRejectedPurchase(t *testing.T) {
	svc := NewService(newMockRepo())
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "REJECTED", "idem1", 1000))
	_, err := svc.ProcessTransaction(context.Background(), makeAdjustCmd("adj1", "REFUND", "APPROVED", "tx1", "idem-adj1", 500))
	if !errors.Is(err, domain.ErrPurchaseNotApproved) {
		t.Errorf("expected ErrPurchaseNotApproved, got %v", err)
	}
}

func TestGetTransaction(t *testing.T) {
	svc := NewService(newMockRepo())
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	tx, err := svc.GetTransaction(context.Background(), "tx1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.ID != "tx1" {
		t.Errorf("expected tx1, got %s", tx.ID)
	}
}

func TestGetTransactionNotFound(t *testing.T) {
	svc := NewService(newMockRepo())
	_, err := svc.GetTransaction(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrTransactionNotFound) {
		t.Errorf("expected ErrTransactionNotFound, got %v", err)
	}
}

func TestListTransactions(t *testing.T) {
	svc := NewService(newMockRepo())
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem1", 1000))
	svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx2", "APPROVED", "idem2", 2000))
	txs, err := svc.ListTransactions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 2 {
		t.Errorf("expected 2 transactions, got %d", len(txs))
	}
}

func TestInvalidTransactionType(t *testing.T) {
	svc := NewService(newMockRepo())
	cmd := makePurchaseCmd("tx1", "APPROVED", "idem1", 1000)
	cmd.TransactionType = "UNKNOWN"
	_, err := svc.ProcessTransaction(context.Background(), cmd)
	if !errors.Is(err, domain.ErrInvalidTransactionType) {
		t.Errorf("expected ErrInvalidTransactionType, got %v", err)
	}
}

func TestProcessAmountOutOfRange(t *testing.T) {
	svc := NewService(newMockRepo())
	t.Run("amount too low", func(t *testing.T) {
		_, err := svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem-low", 50))
		if !errors.Is(err, domain.ErrAmountOutOfRange) {
			t.Errorf("expected ErrAmountOutOfRange, got %v", err)
		}
	})
	t.Run("amount too high", func(t *testing.T) {
		_, err := svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx2", "APPROVED", "idem-high", 600_000))
		if !errors.Is(err, domain.ErrAmountOutOfRange) {
			t.Errorf("expected ErrAmountOutOfRange, got %v", err)
		}
	})
}

func TestProcessNegativeAmount(t *testing.T) {
	svc := NewService(newMockRepo())
	_, err := svc.ProcessTransaction(context.Background(), makePurchaseCmd("tx1", "APPROVED", "idem-neg", -100))
	if !errors.Is(err, domain.ErrNegativeAmount) {
		t.Errorf("expected ErrNegativeAmount, got %v", err)
	}
}
