package http

import (
	"fmt"
	"time"

	"github.com/jailtonjunior/pomelo/internal/application/ports"
)

// WebhookRequestDTO mirrors the exact Pomelo webhook payload structure.
type WebhookRequestDTO struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Amount struct {
		Local struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"local"`
		Transaction struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"transaction"`
		Settlement struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"settlement"`
		Original struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"original"`
	} `json:"amount"`
	Merchant struct {
		ID      string `json:"id"`
		MCC     string `json:"mcc"`
		Address string `json:"address"`
		Name    string `json:"name"`
		City    string `json:"city"`
		State   string `json:"state"`
	} `json:"merchant"`
	Event struct {
		ID             string `json:"id"`
		CreatedAt      string `json:"created_at"`
		IdempotencyKey string `json:"idempotency_key"`
	} `json:"event"`
	OriginalTransactionID string `json:"original_transaction_id"`
	UserID                string `json:"user_id"`
	CardID                string `json:"card_id"`
	Country               string `json:"country"`
	Currency              string `json:"currency"`
	PointOfSale           string `json:"point_of_sale"`
}

// ToCommand converts the DTO to an application command.
func (d *WebhookRequestDTO) ToCommand() (ports.ProcessTransactionCommand, error) {
	if d.ID == "" {
		return ports.ProcessTransactionCommand{}, fmt.Errorf("id is required")
	}
	if d.Type == "" {
		return ports.ProcessTransactionCommand{}, fmt.Errorf("type is required")
	}
	if d.Event.ID == "" {
		return ports.ProcessTransactionCommand{}, fmt.Errorf("event.id is required")
	}
	if d.Event.IdempotencyKey == "" {
		return ports.ProcessTransactionCommand{}, fmt.Errorf("event.idempotency_key is required")
	}
	if d.Status != "APPROVED" && d.Status != "REJECTED" {
		return ports.ProcessTransactionCommand{}, fmt.Errorf("status must be APPROVED or REJECTED, got: %q", d.Status)
	}

	createdAt, err := time.Parse(time.RFC3339, d.Event.CreatedAt)
	if err != nil {
		return ports.ProcessTransactionCommand{}, fmt.Errorf("event.created_at must be RFC3339: %w", err)
	}

	return ports.ProcessTransactionCommand{
		TransactionID:         d.ID,
		TransactionType:       d.Type,
		TransactionStatus:     d.Status,
		OriginalTransactionID: d.OriginalTransactionID,
		LocalAmount:           d.Amount.Local.Total,
		LocalCurrency:         d.Amount.Local.Currency,
		TxAmount:              d.Amount.Transaction.Total,
		TxCurrency:            d.Amount.Transaction.Currency,
		SettlementAmount:      d.Amount.Settlement.Total,
		SettlementCurrency:    d.Amount.Settlement.Currency,
		OriginalAmount:        d.Amount.Original.Total,
		OriginalCurrency:      d.Amount.Original.Currency,
		MerchantID:            d.Merchant.ID,
		MerchantMCC:           d.Merchant.MCC,
		MerchantAddress:       d.Merchant.Address,
		MerchantName:          d.Merchant.Name,
		MerchantCity:          d.Merchant.City,
		MerchantState:         d.Merchant.State,
		EventID:               d.Event.ID,
		EventCreatedAt:        createdAt,
		IdempotencyKey:        d.Event.IdempotencyKey,
		UserID:                d.UserID,
		CardID:                d.CardID,
		Country:               d.Country,
		Currency:              d.Currency,
		PointOfSale:           d.PointOfSale,
	}, nil
}

// WebhookResponseDTO is the success response.
type WebhookResponseDTO struct {
	TransactionID string `json:"transaction_id"`
	Idempotent    bool   `json:"idempotent"`
	Message       string `json:"message,omitempty"`
}

// ErrorResponseDTO is the error response.
type ErrorResponseDTO struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}
