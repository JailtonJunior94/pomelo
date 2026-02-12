package ports

import (
	"context"
	"time"

	"github.com/jailtonjunior/pomelo/internal/domain"
)

// ProcessTransactionCommand holds primitive types only — no domain types.
type ProcessTransactionCommand struct {
	// Transaction fields
	TransactionID         string
	TransactionType       string
	TransactionStatus     string
	OriginalTransactionID string

	// Amount breakdown (all in cents)
	LocalAmount       int64
	LocalCurrency     string
	TxAmount          int64
	TxCurrency        string
	SettlementAmount  int64
	SettlementCurrency string
	OriginalAmount    int64
	OriginalCurrency  string

	// Merchant
	MerchantID      string
	MerchantMCC     string
	MerchantAddress string
	MerchantName    string
	MerchantCity    string
	MerchantState   string

	// Event
	EventID        string
	EventCreatedAt time.Time
	IdempotencyKey string

	// User/Card context
	UserID      string
	CardID      string
	Country     string
	Currency    string
	PointOfSale string
}

type ProcessTransactionResult struct {
	TransactionID string
	Idempotent    bool
}

type WebhookUseCase interface {
	ProcessTransaction(ctx context.Context, cmd ProcessTransactionCommand) (ProcessTransactionResult, error)
	GetTransaction(ctx context.Context, id string) (domain.Transaction, error)
	ListTransactions(ctx context.Context) ([]domain.Transaction, error)
}
