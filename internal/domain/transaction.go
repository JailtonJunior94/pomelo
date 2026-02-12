package domain

import (
	"fmt"
	"time"
)

const (
	// MinPurchaseAmount é R$1,00 em centavos.
	MinPurchaseAmount int64 = 100
	// MaxPurchaseAmount é R$5.000,00 em centavos.
	MaxPurchaseAmount int64 = 500_000
)

type TransactionType string

const (
	TypePurchase         TransactionType = "PURCHASE"
	TypeReversalPurchase TransactionType = "REVERSAL_PURCHASE"
	TypeRefund           TransactionType = "REFUND"
)

type TransactionStatus string

const (
	StatusApproved TransactionStatus = "APPROVED"
	StatusRejected TransactionStatus = "REJECTED"
)

type AmountBreakdown struct {
	Local       Money
	Transaction Money
	Settlement  Money
	Original    Money
}

type Merchant struct {
	ID      string
	MCC     string
	Address string
	Name    string
	City    string
	State   string
}

type Event struct {
	ID             string
	CreatedAt      time.Time
	IdempotencyKey string
}

// Transaction is the aggregate root for PURCHASE events.
type Transaction struct {
	ID                    string
	Type                  TransactionType
	Status                TransactionStatus
	Amount                AmountBreakdown
	Merchant              Merchant
	Event                 Event
	OriginalTransactionID string
	UserID                string
	CardID                string
	Country               string
	Currency              string
	PointOfSale           string
}

func NewPurchase(
	id string,
	status TransactionStatus,
	amount AmountBreakdown,
	merchant Merchant,
	event Event,
	userID, cardID, country, currency, pointOfSale string,
) (Transaction, error) {
	if id == "" {
		return Transaction{}, fmt.Errorf("%w: transaction id is required", ErrInvalidInput)
	}
	if event.ID == "" || event.IdempotencyKey == "" {
		return Transaction{}, fmt.Errorf("%w: event id and idempotency key are required", ErrInvalidInput)
	}
	if amount.Local.Amount < MinPurchaseAmount || amount.Local.Amount > MaxPurchaseAmount {
		return Transaction{}, ErrAmountOutOfRange
	}
	return Transaction{
		ID:          id,
		Type:        TypePurchase,
		Status:      status,
		Amount:      amount,
		Merchant:    merchant,
		Event:       event,
		UserID:      userID,
		CardID:      cardID,
		Country:     country,
		Currency:    currency,
		PointOfSale: pointOfSale,
	}, nil
}

func (t Transaction) IsApprovedPurchase() bool {
	return t.Type == TypePurchase && t.Status == StatusApproved
}

func (t Transaction) CanReceiveAdjustment() bool {
	return t.IsApprovedPurchase()
}
