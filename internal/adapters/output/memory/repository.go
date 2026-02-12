package memory

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/jailtonjunior/pomelo/internal/domain"
)

// Repository is a thread-safe in-memory implementation of ports.TransactionRepository.
type Repository struct {
	mu              sync.RWMutex
	transactions    map[string]domain.Transaction
	adjustments     map[string][]domain.Adjustment
	idempotencyKeys map[string]string
}

func NewRepository() *Repository {
	return &Repository{
		transactions:    make(map[string]domain.Transaction),
		adjustments:     make(map[string][]domain.Adjustment),
		idempotencyKeys: make(map[string]string),
	}
}

// SaveTransaction atomically checks idempotency and transaction ID uniqueness, then saves.
// Both checks are performed under the same WLock, eliminating TOCTOU races.
func (r *Repository) SaveTransaction(_ context.Context, tx domain.Transaction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
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

// SaveAdjustment atomically checks idempotency and saves the adjustment.
// The check-then-write is performed under the same WLock, eliminating the TOCTOU race.
func (r *Repository) SaveAdjustment(_ context.Context, adj domain.Adjustment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.idempotencyKeys[adj.Event.IdempotencyKey]; exists {
		return domain.ErrDuplicateIdempotencyKey
	}
	r.idempotencyKeys[adj.Event.IdempotencyKey] = adj.ID
	r.adjustments[adj.OriginalTransactionID] = append(r.adjustments[adj.OriginalTransactionID], adj)
	return nil
}

func (r *Repository) GetTransactionByID(_ context.Context, id string) (domain.Transaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tx, ok := r.transactions[id]
	if !ok {
		return domain.Transaction{}, domain.ErrTransactionNotFound
	}
	return tx, nil
}

// GetAdjustmentsByTransactionID returns a copy of the slice to prevent external mutation.
func (r *Repository) GetAdjustmentsByTransactionID(_ context.Context, originalTxID string) ([]domain.Adjustment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.adjustments[originalTxID]), nil
}

// GetByIdempotencyKey is a read under RLock.
// NOTE: The atomicity guarantee (check+write) is enforced in SaveTransaction/SaveAdjustment
// which hold the write lock. Callers in the service layer must treat this as advisory.
func (r *Repository) GetByIdempotencyKey(_ context.Context, key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.idempotencyKeys[key]
	return id, ok
}

func (r *Repository) ListTransactions(_ context.Context) ([]domain.Transaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Collect(maps.Values(r.transactions)), nil
}
