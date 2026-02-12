package domain

import (
	"errors"
	"testing"
)

func TestNewMoney(t *testing.T) {
	t.Run("valid amount", func(t *testing.T) {
		m, err := NewMoney(100, "BRL")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.Amount != 100 || m.Currency != "BRL" {
			t.Errorf("got %+v", m)
		}
	})
	t.Run("zero amount is valid", func(t *testing.T) {
		_, err := NewMoney(0, "BRL")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("negative amount rejected", func(t *testing.T) {
		_, err := NewMoney(-1, "BRL")
		if !errors.Is(err, ErrNegativeAmount) {
			t.Errorf("expected ErrNegativeAmount, got %v", err)
		}
	})
}

func TestMoneyAdd(t *testing.T) {
	t.Run("same currency", func(t *testing.T) {
		a, _ := NewMoney(100, "BRL")
		b, _ := NewMoney(50, "BRL")
		result, err := a.Add(b)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Amount != 150 {
			t.Errorf("expected 150, got %d", result.Amount)
		}
	})
	t.Run("currency mismatch", func(t *testing.T) {
		a, _ := NewMoney(100, "BRL")
		b, _ := NewMoney(50, "USD")
		_, err := a.Add(b)
		if !errors.Is(err, ErrCurrencyMismatch) {
			t.Errorf("expected ErrCurrencyMismatch, got %v", err)
		}
	})
}

func TestMoneyGreaterThan(t *testing.T) {
	a, _ := NewMoney(100, "BRL")
	b, _ := NewMoney(50, "BRL")

	greater, err := a.GreaterThan(b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !greater {
		t.Error("100 should be greater than 50")
	}

	lesser, err := b.GreaterThan(a)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lesser {
		t.Error("50 should not be greater than 100")
	}

	t.Run("currency mismatch", func(t *testing.T) {
		c, _ := NewMoney(50, "USD")
		_, err := a.GreaterThan(c)
		if !errors.Is(err, ErrCurrencyMismatch) {
			t.Errorf("expected ErrCurrencyMismatch, got %v", err)
		}
	})
}

