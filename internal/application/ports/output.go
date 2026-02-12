package ports

import (
	"context"

	"github.com/jailtonjunior/pomelo/internal/domain"
)

type TransactionRepository interface {
	SaveTransaction(ctx context.Context, tx domain.Transaction) error
	SaveAdjustment(ctx context.Context, adj domain.Adjustment) error
	GetTransactionByID(ctx context.Context, id string) (domain.Transaction, error)
	GetAdjustmentsByTransactionID(ctx context.Context, originalTxID string) ([]domain.Adjustment, error)
	GetByIdempotencyKey(ctx context.Context, key string) (string, bool)
	ListTransactions(ctx context.Context) ([]domain.Transaction, error)
}
