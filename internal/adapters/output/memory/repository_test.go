package memory

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jailtonjunior/pomelo/internal/domain"
)

func makeMoney(amount int64) domain.Money {
	m, _ := domain.NewMoney(amount, "BRL")
	return m
}

func makeAmountBreakdown(amount int64) domain.AmountBreakdown {
	m := makeMoney(amount)
	return domain.AmountBreakdown{Local: m, Transaction: m, Settlement: m, Original: m}
}

func makePurchase(id, idemKey string, amount int64) domain.Transaction {
	event := domain.Event{ID: "evt-" + id, CreatedAt: time.Now(), IdempotencyKey: idemKey}
	tx, _ := domain.NewPurchase(id, domain.StatusApproved, makeAmountBreakdown(amount),
		domain.Merchant{ID: "m1", Name: "Store"}, event, "u1", "card1", "BR", "BRL", "POS")
	return tx
}

func makeAdjustment(id, originalID, idemKey string, amount int64) domain.Adjustment {
	event := domain.Event{ID: "evt-" + id, CreatedAt: time.Now(), IdempotencyKey: idemKey}
	adj, _ := domain.NewAdjustment(id, domain.TypeRefund, domain.StatusApproved, makeAmountBreakdown(amount),
		domain.Merchant{ID: "m1", Name: "Store"}, event, originalID, "u1", "card1", "BR", "BRL", "POS")
	return adj
}

func TestSaveAndGetTransaction(t *testing.T) {
	repo := NewRepository()
	ctx := context.Background()
	tx := makePurchase("tx1", "idem1", 1000)

	if err := repo.SaveTransaction(ctx, tx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := repo.GetTransactionByID(ctx, "tx1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "tx1" {
		t.Errorf("expected tx1, got %s", got.ID)
	}
}

func TestGetTransactionNotFound(t *testing.T) {
	repo := NewRepository()
	_, err := repo.GetTransactionByID(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error")
	}
}

func TestSaveAndGetAdjustment(t *testing.T) {
	repo := NewRepository()
	ctx := context.Background()
	tx := makePurchase("tx1", "idem1", 1000)
	repo.SaveTransaction(ctx, tx)

	adj := makeAdjustment("adj1", "tx1", "adj-idem1", 500)
	if err := repo.SaveAdjustment(ctx, adj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	adjs, err := repo.GetAdjustmentsByTransactionID(ctx, "tx1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adjs) != 1 {
		t.Errorf("expected 1 adjustment, got %d", len(adjs))
	}
}

func TestGetByIdempotencyKey(t *testing.T) {
	repo := NewRepository()
	ctx := context.Background()
	tx := makePurchase("tx1", "idem1", 1000)
	repo.SaveTransaction(ctx, tx)

	id, ok := repo.GetByIdempotencyKey(ctx, "idem1")
	if !ok {
		t.Error("expected idempotency key to exist")
	}
	if id != "tx1" {
		t.Errorf("expected tx1, got %s", id)
	}

	_, ok = repo.GetByIdempotencyKey(ctx, "nonexistent")
	if ok {
		t.Error("expected idempotency key to not exist")
	}
}

func TestListTransactions(t *testing.T) {
	repo := NewRepository()
	ctx := context.Background()
	repo.SaveTransaction(ctx, makePurchase("tx1", "idem1", 1000))
	repo.SaveTransaction(ctx, makePurchase("tx2", "idem2", 2000))

	txs, err := repo.ListTransactions(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 2 {
		t.Errorf("expected 2, got %d", len(txs))
	}
}

func TestSaveTransactionDuplicateID(t *testing.T) {
	repo := NewRepository()
	ctx := context.Background()
	repo.SaveTransaction(ctx, makePurchase("tx1", "idem1", 1000))

	// Same transaction ID, different idempotency key — must be rejected
	tx2 := makePurchase("tx1", "idem2", 1000)
	err := repo.SaveTransaction(ctx, tx2)
	if !errors.Is(err, domain.ErrDuplicateTransactionID) {
		t.Errorf("expected ErrDuplicateTransactionID, got %v", err)
	}
}

func TestGetAdjustmentsReturnsCopy(t *testing.T) {
	repo := NewRepository()
	ctx := context.Background()
	tx := makePurchase("tx1", "idem1", 1000)
	repo.SaveTransaction(ctx, tx)
	repo.SaveAdjustment(ctx, makeAdjustment("adj1", "tx1", "adj-idem1", 500))

	adjs, _ := repo.GetAdjustmentsByTransactionID(ctx, "tx1")
	// Mutate the returned slice
	if len(adjs) > 0 {
		adjs[0].ID = "mutated"
	}

	// Original should be unchanged
	adjs2, _ := repo.GetAdjustmentsByTransactionID(ctx, "tx1")
	if len(adjs2) > 0 && adjs2[0].ID == "mutated" {
		t.Error("internal slice was mutated by external modification")
	}
}

func TestConcurrentAccess(t *testing.T) {
	repo := NewRepository()
	ctx := context.Background()
	var wg sync.WaitGroup

	// 100 concurrent writers — Go 1.22: loop var is per-iteration
	// Go 1.25: wg.Go combines Add(1) + go + defer Done()
	// i+1 ensures amount starts at 100 (MinPurchaseAmount), avoiding ErrAmountOutOfRange for i=0.
	for i := range 100 {
		wg.Go(func() {
			id := "tx" + string(rune('a'+i%26)) + "-" + string(rune('0'+i/26))
			repo.SaveTransaction(ctx, makePurchase(id, "idem-"+id, int64((i+1)*100)))
		})
	}

	// 100 concurrent readers
	for range 100 {
		wg.Go(func() {
			repo.ListTransactions(ctx)
		})
	}

	wg.Wait()
}
