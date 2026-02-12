package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/jailtonjunior/pomelo/internal/application/ports"
	"github.com/jailtonjunior/pomelo/internal/domain"
)

// Service implements ports.WebhookUseCase.
type Service struct {
	repo ports.TransactionRepository
}

func NewService(repo ports.TransactionRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ProcessTransaction(ctx context.Context, cmd ports.ProcessTransactionCommand) (ports.ProcessTransactionResult, error) {
	switch domain.TransactionType(cmd.TransactionType) {
	case domain.TypePurchase:
		return s.processPurchase(ctx, cmd)
	case domain.TypeReversalPurchase, domain.TypeRefund:
		return s.processAdjustment(ctx, cmd)
	default:
		return ports.ProcessTransactionResult{}, fmt.Errorf("%w: %s", domain.ErrInvalidTransactionType, cmd.TransactionType)
	}
}

func (s *Service) GetTransaction(ctx context.Context, id string) (domain.Transaction, error) {
	return s.repo.GetTransactionByID(ctx, id)
}

func (s *Service) ListTransactions(ctx context.Context) ([]domain.Transaction, error) {
	return s.repo.ListTransactions(ctx)
}

func (s *Service) processPurchase(ctx context.Context, cmd ports.ProcessTransactionCommand) (ports.ProcessTransactionResult, error) {
	// 1. Advisory idempotency check (fast path — not atomic, eliminates most duplicates before object construction)
	if _, exists := s.repo.GetByIdempotencyKey(ctx, cmd.IdempotencyKey); exists {
		return ports.ProcessTransactionResult{TransactionID: cmd.TransactionID, Idempotent: true}, domain.ErrDuplicateIdempotencyKey
	}

	// 2. Build domain objects
	localMoney, err := domain.NewMoney(cmd.LocalAmount, cmd.LocalCurrency)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}
	txMoney, err := domain.NewMoney(cmd.TxAmount, cmd.TxCurrency)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}
	settlementMoney, err := domain.NewMoney(cmd.SettlementAmount, cmd.SettlementCurrency)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}
	originalMoney, err := domain.NewMoney(cmd.OriginalAmount, cmd.OriginalCurrency)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}

	amount := domain.AmountBreakdown{
		Local:       localMoney,
		Transaction: txMoney,
		Settlement:  settlementMoney,
		Original:    originalMoney,
	}
	merchant := domain.Merchant{
		ID:      cmd.MerchantID,
		MCC:     cmd.MerchantMCC,
		Address: cmd.MerchantAddress,
		Name:    cmd.MerchantName,
		City:    cmd.MerchantCity,
		State:   cmd.MerchantState,
	}
	event := domain.Event{
		ID:             cmd.EventID,
		CreatedAt:      cmd.EventCreatedAt,
		IdempotencyKey: cmd.IdempotencyKey,
	}

	// 3. Create purchase
	tx, err := domain.NewPurchase(
		cmd.TransactionID,
		domain.TransactionStatus(cmd.TransactionStatus),
		amount, merchant, event,
		cmd.UserID, cmd.CardID, cmd.Country, cmd.Currency, cmd.PointOfSale,
	)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}

	// 4. Save — atomically re-checks idempotency under WLock (handles the TOCTOU race case)
	if err := s.repo.SaveTransaction(ctx, tx); err != nil {
		if errors.Is(err, domain.ErrDuplicateIdempotencyKey) {
			return ports.ProcessTransactionResult{TransactionID: cmd.TransactionID, Idempotent: true}, err
		}
		return ports.ProcessTransactionResult{}, err
	}
	return ports.ProcessTransactionResult{TransactionID: tx.ID}, nil
}

func (s *Service) processAdjustment(ctx context.Context, cmd ports.ProcessTransactionCommand) (ports.ProcessTransactionResult, error) {
	// 1. Validate original transaction ID before any I/O — prevents 404 masking a 400 validation error
	if cmd.OriginalTransactionID == "" {
		return ports.ProcessTransactionResult{}, domain.ErrOriginalTransactionRequired
	}

	// 2. Advisory idempotency check (fast path — not atomic, eliminates most duplicates before object construction)
	if _, exists := s.repo.GetByIdempotencyKey(ctx, cmd.IdempotencyKey); exists {
		return ports.ProcessTransactionResult{TransactionID: cmd.TransactionID, Idempotent: true}, domain.ErrDuplicateIdempotencyKey
	}

	// 3. Get original transaction
	original, err := s.repo.GetTransactionByID(ctx, cmd.OriginalTransactionID)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}

	// 4. Build domain objects
	localMoney, err := domain.NewMoney(cmd.LocalAmount, cmd.LocalCurrency)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}
	txMoney, err := domain.NewMoney(cmd.TxAmount, cmd.TxCurrency)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}
	settlementMoney, err := domain.NewMoney(cmd.SettlementAmount, cmd.SettlementCurrency)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}
	originalMoney, err := domain.NewMoney(cmd.OriginalAmount, cmd.OriginalCurrency)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}

	amount := domain.AmountBreakdown{
		Local:       localMoney,
		Transaction: txMoney,
		Settlement:  settlementMoney,
		Original:    originalMoney,
	}
	merchant := domain.Merchant{
		ID:      cmd.MerchantID,
		MCC:     cmd.MerchantMCC,
		Address: cmd.MerchantAddress,
		Name:    cmd.MerchantName,
		City:    cmd.MerchantCity,
		State:   cmd.MerchantState,
	}
	event := domain.Event{
		ID:             cmd.EventID,
		CreatedAt:      cmd.EventCreatedAt,
		IdempotencyKey: cmd.IdempotencyKey,
	}

	// 4. Create adjustment
	adj, err := domain.NewAdjustment(
		cmd.TransactionID,
		domain.TransactionType(cmd.TransactionType),
		domain.TransactionStatus(cmd.TransactionStatus),
		amount, merchant, event,
		cmd.OriginalTransactionID,
		cmd.UserID, cmd.CardID, cmd.Country, cmd.Currency, cmd.PointOfSale,
	)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}

	// 5. Get existing adjustments and sum approved ones
	existingAdjs, err := s.repo.GetAdjustmentsByTransactionID(ctx, cmd.OriginalTransactionID)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}
	existingTotal, err := s.sumExistingAdjustments(existingAdjs, cmd.LocalCurrency)
	if err != nil {
		return ports.ProcessTransactionResult{}, err
	}

	// 6. Validate
	if err := adj.ValidateAgainstPurchase(original, existingTotal); err != nil {
		return ports.ProcessTransactionResult{}, err
	}

	// 7. Save — atomically re-checks idempotency under WLock (handles the TOCTOU race case)
	if err := s.repo.SaveAdjustment(ctx, adj); err != nil {
		if errors.Is(err, domain.ErrDuplicateIdempotencyKey) {
			return ports.ProcessTransactionResult{TransactionID: cmd.TransactionID, Idempotent: true}, err
		}
		return ports.ProcessTransactionResult{}, err
	}
	return ports.ProcessTransactionResult{TransactionID: adj.ID}, nil
}

func (s *Service) sumExistingAdjustments(adjs []domain.Adjustment, currency string) (domain.Money, error) {
	total, err := domain.NewMoney(0, currency)
	if err != nil {
		return domain.Money{}, err
	}
	for _, adj := range adjs {
		if adj.Status != domain.StatusApproved {
			continue
		}
		total, err = total.Add(adj.Amount.Local)
		if err != nil {
			return domain.Money{}, err
		}
	}
	return total, nil
}
