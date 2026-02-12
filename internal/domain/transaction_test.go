package domain

import (
	"errors"
	"testing"
	"time"
)

func makeAmountBreakdown(amount int64, currency string) AmountBreakdown {
	m, _ := NewMoney(amount, currency)
	return AmountBreakdown{Local: m, Transaction: m, Settlement: m, Original: m}
}

func makeMerchant() Merchant {
	return Merchant{ID: "m1", MCC: "5411", Name: "Test Store", City: "SP", State: "SP"}
}

func makeEvent(idempotencyKey string) Event {
	return Event{ID: "evt1", CreatedAt: time.Now(), IdempotencyKey: idempotencyKey}
}

func TestNewPurchase(t *testing.T) {
	t.Run("valid purchase", func(t *testing.T) {
		tx, err := NewPurchase(
			"tx1", StatusApproved,
			makeAmountBreakdown(1000, "BRL"),
			makeMerchant(), makeEvent("idem1"),
			"user1", "card1", "BR", "BRL", "POS",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tx.Type != TypePurchase {
			t.Errorf("expected PURCHASE type, got %s", tx.Type)
		}
		if tx.ID != "tx1" {
			t.Errorf("expected id tx1, got %s", tx.ID)
		}
	})
	t.Run("empty id rejected", func(t *testing.T) {
		_, err := NewPurchase(
			"", StatusApproved,
			makeAmountBreakdown(1000, "BRL"),
			makeMerchant(), makeEvent("idem1"),
			"user1", "card1", "BR", "BRL", "POS",
		)
		if err == nil {
			t.Error("expected error for empty id")
		}
	})
	t.Run("empty event id rejected", func(t *testing.T) {
		evt := Event{ID: "", CreatedAt: time.Now(), IdempotencyKey: "idem1"}
		_, err := NewPurchase(
			"tx1", StatusApproved,
			makeAmountBreakdown(1000, "BRL"),
			makeMerchant(), evt,
			"user1", "card1", "BR", "BRL", "POS",
		)
		if err == nil {
			t.Error("expected error for empty event id")
		}
	})
	t.Run("empty idempotency key rejected", func(t *testing.T) {
		evt := Event{ID: "evt1", CreatedAt: time.Now(), IdempotencyKey: ""}
		_, err := NewPurchase(
			"tx1", StatusApproved,
			makeAmountBreakdown(1000, "BRL"),
			makeMerchant(), evt,
			"user1", "card1", "BR", "BRL", "POS",
		)
		if err == nil {
			t.Error("expected error for empty idempotency key")
		}
	})
}

func TestNewPurchaseAmountRange(t *testing.T) {
	newPurchase := func(amount int64) error {
		_, err := NewPurchase("tx1", StatusApproved, makeAmountBreakdown(amount, "BRL"),
			makeMerchant(), makeEvent("idem1"), "u", "c", "BR", "BRL", "POS")
		return err
	}

	t.Run("valor mínimo R$1,00 aceito", func(t *testing.T) {
		if err := newPurchase(MinPurchaseAmount); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("valor máximo R$5.000,00 aceito", func(t *testing.T) {
		if err := newPurchase(MaxPurchaseAmount); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("abaixo do mínimo (R$0,99) rejeitado", func(t *testing.T) {
		if err := newPurchase(MinPurchaseAmount - 1); !errors.Is(err, ErrAmountOutOfRange) {
			t.Errorf("expected ErrAmountOutOfRange, got %v", err)
		}
	})
	t.Run("zero rejeitado", func(t *testing.T) {
		if err := newPurchase(0); !errors.Is(err, ErrAmountOutOfRange) {
			t.Errorf("expected ErrAmountOutOfRange, got %v", err)
		}
	})
	t.Run("acima do máximo (R$5.000,01) rejeitado", func(t *testing.T) {
		if err := newPurchase(MaxPurchaseAmount + 1); !errors.Is(err, ErrAmountOutOfRange) {
			t.Errorf("expected ErrAmountOutOfRange, got %v", err)
		}
	})
}

func TestIsApprovedPurchase(t *testing.T) {
	approved, _ := NewPurchase("tx1", StatusApproved, makeAmountBreakdown(1000, "BRL"), makeMerchant(), makeEvent("idem1"), "u", "c", "BR", "BRL", "POS")
	if !approved.IsApprovedPurchase() {
		t.Error("should be approved purchase")
	}

	rejected, _ := NewPurchase("tx2", StatusRejected, makeAmountBreakdown(1000, "BRL"), makeMerchant(), makeEvent("idem2"), "u", "c", "BR", "BRL", "POS")
	if rejected.IsApprovedPurchase() {
		t.Error("rejected purchase should not be approved")
	}
}

func TestCanReceiveAdjustment(t *testing.T) {
	approved, _ := NewPurchase("tx1", StatusApproved, makeAmountBreakdown(1000, "BRL"), makeMerchant(), makeEvent("idem1"), "u", "c", "BR", "BRL", "POS")
	if !approved.CanReceiveAdjustment() {
		t.Error("approved purchase should receive adjustments")
	}

	rejected, _ := NewPurchase("tx2", StatusRejected, makeAmountBreakdown(1000, "BRL"), makeMerchant(), makeEvent("idem2"), "u", "c", "BR", "BRL", "POS")
	if rejected.CanReceiveAdjustment() {
		t.Error("rejected purchase should not receive adjustments")
	}
}
