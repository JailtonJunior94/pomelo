package domain

import (
	"errors"
	"testing"
)

func makeApprovedPurchase(id string, amount int64) Transaction {
	tx, _ := NewPurchase(id, StatusApproved, makeAmountBreakdown(amount, "BRL"), makeMerchant(), makeEvent("idem-"+id), "u", "c", "BR", "BRL", "POS")
	return tx
}

func makeAdjustment(id string, txType TransactionType, amount int64, originalID string) Adjustment {
	adj, _ := NewAdjustment(id, txType, StatusApproved, makeAmountBreakdown(amount, "BRL"), makeMerchant(), makeEvent("adj-idem-"+id), originalID, "u", "c", "BR", "BRL", "POS")
	return adj
}

func TestNewAdjustment(t *testing.T) {
	t.Run("valid reversal", func(t *testing.T) {
		adj, err := NewAdjustment("adj1", TypeReversalPurchase, StatusApproved, makeAmountBreakdown(500, "BRL"), makeMerchant(), makeEvent("idem-adj1"), "tx1", "u", "c", "BR", "BRL", "POS")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adj.Type != TypeReversalPurchase {
			t.Errorf("expected REVERSAL_PURCHASE, got %s", adj.Type)
		}
	})
	t.Run("valid refund", func(t *testing.T) {
		_, err := NewAdjustment("adj1", TypeRefund, StatusApproved, makeAmountBreakdown(500, "BRL"), makeMerchant(), makeEvent("idem-adj1"), "tx1", "u", "c", "BR", "BRL", "POS")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("PURCHASE type rejected", func(t *testing.T) {
		_, err := NewAdjustment("adj1", TypePurchase, StatusApproved, makeAmountBreakdown(500, "BRL"), makeMerchant(), makeEvent("idem-adj1"), "tx1", "u", "c", "BR", "BRL", "POS")
		if !errors.Is(err, ErrInvalidTransactionType) {
			t.Errorf("expected ErrInvalidTransactionType, got %v", err)
		}
	})
	t.Run("missing original transaction id", func(t *testing.T) {
		_, err := NewAdjustment("adj1", TypeRefund, StatusApproved, makeAmountBreakdown(500, "BRL"), makeMerchant(), makeEvent("idem-adj1"), "", "u", "c", "BR", "BRL", "POS")
		if !errors.Is(err, ErrOriginalTransactionRequired) {
			t.Errorf("expected ErrOriginalTransactionRequired, got %v", err)
		}
	})
	t.Run("empty id rejected", func(t *testing.T) {
		_, err := NewAdjustment("", TypeRefund, StatusApproved, makeAmountBreakdown(500, "BRL"), makeMerchant(), makeEvent("idem-adj1"), "tx1", "u", "c", "BR", "BRL", "POS")
		if err == nil {
			t.Error("expected error for empty id")
		}
	})
}

func TestValidateAgainstPurchase(t *testing.T) {
	zero, _ := NewMoney(0, "BRL")

	t.Run("total reversal approved", func(t *testing.T) {
		purchase := makeApprovedPurchase("tx1", 1000)
		adj := makeAdjustment("adj1", TypeReversalPurchase, 1000, "tx1")
		if err := adj.ValidateAgainstPurchase(purchase, zero); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("partial refund approved", func(t *testing.T) {
		purchase := makeApprovedPurchase("tx1", 1000)
		adj := makeAdjustment("adj1", TypeRefund, 500, "tx1")
		if err := adj.ValidateAgainstPurchase(purchase, zero); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("exceeds original amount", func(t *testing.T) {
		purchase := makeApprovedPurchase("tx1", 1000)
		adj := makeAdjustment("adj1", TypeRefund, 1500, "tx1")
		if err := adj.ValidateAgainstPurchase(purchase, zero); !errors.Is(err, ErrExceedsOriginalAmount) {
			t.Errorf("expected ErrExceedsOriginalAmount, got %v", err)
		}
	})
	t.Run("accumulated adjustments exceed original", func(t *testing.T) {
		purchase := makeApprovedPurchase("tx1", 1000)
		adj := makeAdjustment("adj2", TypeRefund, 600, "tx1")
		existing, _ := NewMoney(500, "BRL")
		if err := adj.ValidateAgainstPurchase(purchase, existing); !errors.Is(err, ErrExceedsOriginalAmount) {
			t.Errorf("expected ErrExceedsOriginalAmount, got %v", err)
		}
	})
	t.Run("purchase not approved", func(t *testing.T) {
		rejected, _ := NewPurchase("tx1", StatusRejected, makeAmountBreakdown(1000, "BRL"), makeMerchant(), makeEvent("idem1"), "u", "c", "BR", "BRL", "POS")
		adj := makeAdjustment("adj1", TypeRefund, 500, "tx1")
		if err := adj.ValidateAgainstPurchase(rejected, zero); !errors.Is(err, ErrPurchaseNotApproved) {
			t.Errorf("expected ErrPurchaseNotApproved, got %v", err)
		}
	})
	t.Run("rejected adjustment does not count", func(t *testing.T) {
		purchase := makeApprovedPurchase("tx1", 1000)
		adj, _ := NewAdjustment("adj1", TypeRefund, StatusRejected, makeAmountBreakdown(1500, "BRL"), makeMerchant(), makeEvent("adj-idem-adj1"), "tx1", "u", "c", "BR", "BRL", "POS")
		// rejected adjustments with any amount should pass (no budget check)
		if err := adj.ValidateAgainstPurchase(purchase, zero); err != nil {
			t.Fatalf("unexpected error for rejected adjustment: %v", err)
		}
	})
}
