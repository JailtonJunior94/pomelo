package domain

import "errors"

var (
	ErrTransactionNotFound         = errors.New("transaction not found")
	ErrPurchaseNotApproved         = errors.New("adjustment target must be an approved purchase")
	ErrExceedsOriginalAmount       = errors.New("total adjustments exceed original purchase amount")
	ErrNegativeAmount              = errors.New("amount cannot be negative")
	ErrAmountOutOfRange            = errors.New("purchase amount must be between R$1,00 and R$5.000,00")
	ErrCurrencyMismatch            = errors.New("currency mismatch")
	ErrInvalidTransactionType      = errors.New("invalid transaction type")
	ErrDuplicateIdempotencyKey     = errors.New("duplicate idempotency key")
	ErrOriginalTransactionRequired = errors.New("reversal/refund must reference an original transaction")
	ErrDuplicateTransactionID      = errors.New("transaction ID already exists with a different event")
	ErrInvalidInput                = errors.New("invalid input")
)
