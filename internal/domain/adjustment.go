package domain

import "fmt"

// Adjustment is an immutable entity representing REVERSAL_PURCHASE or REFUND.
type Adjustment struct {
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

func NewAdjustment(
	id string,
	txType TransactionType,
	status TransactionStatus,
	amount AmountBreakdown,
	merchant Merchant,
	event Event,
	originalTransactionID string,
	userID, cardID, country, currency, pointOfSale string,
) (Adjustment, error) {
	if txType != TypeReversalPurchase && txType != TypeRefund {
		return Adjustment{}, fmt.Errorf("%w: %s", ErrInvalidTransactionType, txType)
	}
	if originalTransactionID == "" {
		return Adjustment{}, ErrOriginalTransactionRequired
	}
	if id == "" {
		return Adjustment{}, fmt.Errorf("%w: adjustment id is required", ErrInvalidInput)
	}
	if event.ID == "" || event.IdempotencyKey == "" {
		return Adjustment{}, fmt.Errorf("%w: event id and idempotency key are required", ErrInvalidInput)
	}
	return Adjustment{
		ID:                    id,
		Type:                  txType,
		Status:                status,
		Amount:                amount,
		Merchant:              merchant,
		Event:                 event,
		OriginalTransactionID: originalTransactionID,
		UserID:                userID,
		CardID:                cardID,
		Country:               country,
		Currency:              currency,
		PointOfSale:           pointOfSale,
	}, nil
}

// ValidateAgainstPurchase checks business rules for the adjustment against the original purchase.
// existingTotal is the sum of all previously approved adjustments for this purchase.
func (a Adjustment) ValidateAgainstPurchase(original Transaction, existingTotal Money) error {
	if !original.CanReceiveAdjustment() {
		return ErrPurchaseNotApproved
	}
	if a.Status != StatusApproved {
		// Rejected adjustments don't consume budget
		return nil
	}
	newTotal, err := existingTotal.Add(a.Amount.Local)
	if err != nil {
		return err
	}
	exceeds, err := newTotal.GreaterThan(original.Amount.Local)
	if err != nil {
		return err
	}
	if exceeds {
		return ErrExceedsOriginalAmount
	}
	return nil
}
