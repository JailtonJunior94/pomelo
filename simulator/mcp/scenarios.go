package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// StepResult captures the outcome of a single HTTP step in a scenario.
type StepResult struct {
	Step           int    `json:"step"`
	Description    string `json:"description"`
	Method         string `json:"method"`
	URL            string `json:"url"`
	RequestBody    any    `json:"request_body,omitempty"`
	ResponseStatus int    `json:"response_status"`
	ResponseBody   any    `json:"response_body,omitempty"`
	ExpectedStatus int    `json:"expected_status"`
	Passed         bool   `json:"passed"`
}

// ScenarioResult aggregates all steps and the overall outcome.
type ScenarioResult struct {
	Scenario string       `json:"scenario"`
	Steps    []StepResult `json:"steps"`
	Success  bool         `json:"success"`
	Summary  string       `json:"summary"`
}

type scenarioRunner struct {
	baseURL string
	client  *http.Client
	steps   []StepResult
}

func newRunner(baseURL string) *scenarioRunner {
	return &scenarioRunner{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (r *scenarioRunner) post(desc string, body map[string]any, expectedStatus int) (map[string]any, error) {
	return r.request(http.MethodPost, r.baseURL+"/webhook/transactions", desc, body, expectedStatus)
}

func (r *scenarioRunner) request(method, url, desc string, body any, expectedStatus int) (map[string]any, error) {
	step := StepResult{
		Step:           len(r.steps) + 1,
		Description:    desc,
		Method:         method,
		URL:            url,
		RequestBody:    body,
		ExpectedStatus: expectedStatus,
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	step.ResponseStatus = resp.StatusCode
	step.Passed = resp.StatusCode == expectedStatus

	var respBody map[string]any
	json.NewDecoder(resp.Body).Decode(&respBody)
	step.ResponseBody = respBody

	r.steps = append(r.steps, step)
	return respBody, nil
}

func (r *scenarioRunner) result(scenario string) ScenarioResult {
	success := true
	for _, s := range r.steps {
		if !s.Passed {
			success = false
			break
		}
	}
	summary := fmt.Sprintf("%d/%d steps passed", countPassed(r.steps), len(r.steps))
	return ScenarioResult{Scenario: scenario, Steps: r.steps, Success: success, Summary: summary}
}

func countPassed(steps []StepResult) int {
	n := 0
	for _, s := range steps {
		if s.Passed {
			n++
		}
	}
	return n
}

// --- Payload builders ---

func purchasePayload(txID, idemKey, status string, amount int64) map[string]any {
	return map[string]any{
		"id":     txID,
		"type":   "PURCHASE",
		"status": status,
		"amount": amountBlock(amount, "BRL"),
		"merchant": map[string]any{
			"id": "merchant-001", "mcc": "5411",
			"name": "Test Store", "city": "São Paulo", "state": "SP",
		},
		"event": map[string]any{
			"id":              "evt-" + txID,
			"created_at":      time.Now().UTC().Format(time.RFC3339),
			"idempotency_key": idemKey,
		},
		"user_id":       "user-001",
		"card_id":       "card-001",
		"country":       "BR",
		"currency":      "BRL",
		"point_of_sale": "ONLINE",
	}
}

func adjustmentPayload(txID, txType, idemKey, originalTxID, status string, amount int64) map[string]any {
	p := purchasePayload(txID, idemKey, status, amount)
	p["type"] = txType
	p["original_transaction_id"] = originalTxID
	return p
}

func amountBlock(amount int64, currency string) map[string]any {
	newBlock := func() map[string]any {
		return map[string]any{"total": amount, "currency": currency}
	}
	return map[string]any{
		"local":       newBlock(),
		"transaction": newBlock(),
		"settlement":  newBlock(),
		"original":    newBlock(),
	}
}

// --- Scenario implementations ---

func runScenario(baseURL, scenario string) (ScenarioResult, error) {
	switch scenario {
	// ── Basic purchase flows ──────────────────────────────────────────────
	case "purchase_approved":
		return scenarioPurchaseApproved(baseURL)
	case "purchase_rejected":
		return scenarioPurchaseRejected(baseURL)
	case "purchase_at_min_amount":
		return scenarioPurchaseAtMinAmount(baseURL)
	case "purchase_at_max_amount":
		return scenarioPurchaseAtMaxAmount(baseURL)
	case "purchase_amount_too_low":
		return scenarioPurchaseAmountTooLow(baseURL)
	case "purchase_amount_too_high":
		return scenarioPurchaseAmountTooHigh(baseURL)
	case "purchase_negative_amount":
		return scenarioPurchaseNegativeAmount(baseURL)
	// ── Reversal flows ────────────────────────────────────────────────────
	case "reversal_total":
		return scenarioReversalTotal(baseURL)
	case "reversal_partial":
		return scenarioReversalPartial(baseURL)
	case "reversal_exceeds_amount":
		return scenarioReversalExceedsAmount(baseURL)
	case "reversal_on_rejected_purchase":
		return scenarioReversalOnRejectedPurchase(baseURL)
	// ── Refund flows ──────────────────────────────────────────────────────
	case "refund_total":
		return scenarioRefundTotal(baseURL)
	case "refund_partial_single":
		return scenarioRefundPartialSingle(baseURL)
	case "refund_partial_multiple":
		return scenarioRefundPartialMultiple(baseURL)
	case "refund_exceeds_amount":
		return scenarioRefundExceedsAmount(baseURL)
	case "refund_on_rejected_purchase":
		return scenarioRefundOnRejectedPurchase(baseURL)
	case "multiple_adjustments_exceed":
		return scenarioMultipleAdjustmentsExceed(baseURL)
	// ── Mixed adjustment flows ────────────────────────────────────────────
	case "reversal_after_partial_refund":
		return scenarioReversalAfterPartialRefund(baseURL)
	// ── Idempotency & delivery flows ─────────────────────────────────────
	case "duplicate_event":
		return scenarioDuplicateEvent(baseURL)
	case "out_of_order":
		return scenarioOutOfOrder(baseURL)
	case "webhook_retry":
		return scenarioWebhookRetry(baseURL)
	// ── Validation error flows ────────────────────────────────────────────
	case "missing_original_transaction_id":
		return scenarioMissingOriginalTransactionID(baseURL)
	default:
		return ScenarioResult{}, fmt.Errorf("unknown scenario: %s", scenario)
	}
}

// ── Basic purchase flows ──────────────────────────────────────────────────────

func scenarioPurchaseApproved(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE APPROVED (R$100,00)", purchasePayload("tx-pa-001", "idem-pa-001", "APPROVED", 10000), 200)
	return r.result("purchase_approved"), nil
}

func scenarioPurchaseRejected(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE REJECTED (R$100,00)", purchasePayload("tx-pr-001", "idem-pr-001", "REJECTED", 10000), 200)
	return r.result("purchase_rejected"), nil
}

// scenarioPurchaseAtMinAmount validates boundary: minimum allowed amount R$1,00 (100 cents).
func scenarioPurchaseAtMinAmount(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE at minimum amount R$1,00 (100 cents) → expect 200",
		purchasePayload("tx-pmin-001", "idem-pmin-001", "APPROVED", 100), 200)
	return r.result("purchase_at_min_amount"), nil
}

// scenarioPurchaseAtMaxAmount validates boundary: maximum allowed amount R$5.000,00 (500000 cents).
func scenarioPurchaseAtMaxAmount(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE at maximum amount R$5.000,00 (500000 cents) → expect 200",
		purchasePayload("tx-pmax-001", "idem-pmax-001", "APPROVED", 500_000), 200)
	return r.result("purchase_at_max_amount"), nil
}

// scenarioPurchaseAmountTooLow validates that amounts below R$1,00 are rejected with 422.
func scenarioPurchaseAmountTooLow(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE with amount R$0,50 (50 cents, below minimum) → expect 422",
		purchasePayload("tx-plow-001", "idem-plow-001", "APPROVED", 50), 422)
	return r.result("purchase_amount_too_low"), nil
}

// scenarioPurchaseAmountTooHigh validates that amounts above R$5.000,00 are rejected with 422.
func scenarioPurchaseAmountTooHigh(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE with amount R$6.000,00 (600000 cents, above maximum) → expect 422",
		purchasePayload("tx-phigh-001", "idem-phigh-001", "APPROVED", 600_000), 422)
	return r.result("purchase_amount_too_high"), nil
}

// scenarioPurchaseNegativeAmount validates that negative amounts are rejected with 400.
func scenarioPurchaseNegativeAmount(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE with negative amount → expect 400",
		purchasePayload("tx-pneg-001", "idem-pneg-001", "APPROVED", -100), 400)
	return r.result("purchase_negative_amount"), nil
}

// ── Reversal flows ────────────────────────────────────────────────────────────

func scenarioReversalTotal(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE APPROVED (R$100,00)", purchasePayload("tx-rt-001", "idem-rt-001", "APPROVED", 10000), 200)
	r.post("POST REVERSAL_PURCHASE (total amount R$100,00) → expect 200",
		adjustmentPayload("tx-rt-002", "REVERSAL_PURCHASE", "idem-rt-002", "tx-rt-001", "APPROVED", 10000), 200)
	return r.result("reversal_total"), nil
}

func scenarioReversalPartial(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE APPROVED (R$100,00)", purchasePayload("tx-rp-001", "idem-rp-001", "APPROVED", 10000), 200)
	r.post("POST REVERSAL_PURCHASE (partial R$50,00) → expect 200",
		adjustmentPayload("tx-rp-002", "REVERSAL_PURCHASE", "idem-rp-002", "tx-rp-001", "APPROVED", 5000), 200)
	return r.result("reversal_partial"), nil
}

// scenarioReversalExceedsAmount validates that a reversal exceeding the original amount is rejected with 409.
func scenarioReversalExceedsAmount(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE APPROVED (R$100,00)", purchasePayload("tx-rea-001", "idem-rea-001", "APPROVED", 10000), 200)
	r.post("POST REVERSAL_PURCHASE exceeding original amount (R$150,00) → expect 409",
		adjustmentPayload("tx-rea-002", "REVERSAL_PURCHASE", "idem-rea-002", "tx-rea-001", "APPROVED", 15000), 409)
	return r.result("reversal_exceeds_amount"), nil
}

// scenarioReversalOnRejectedPurchase validates that reversals on rejected purchases are rejected with 409.
func scenarioReversalOnRejectedPurchase(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE REJECTED (R$100,00)", purchasePayload("tx-rorp-001", "idem-rorp-001", "REJECTED", 10000), 200)
	r.post("POST REVERSAL_PURCHASE on rejected purchase → expect 409",
		adjustmentPayload("tx-rorp-002", "REVERSAL_PURCHASE", "idem-rorp-002", "tx-rorp-001", "APPROVED", 10000), 409)
	return r.result("reversal_on_rejected_purchase"), nil
}

// ── Refund flows ──────────────────────────────────────────────────────────────

func scenarioRefundTotal(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE APPROVED (R$100,00)", purchasePayload("tx-rft-001", "idem-rft-001", "APPROVED", 10000), 200)
	r.post("POST REFUND total amount (R$100,00) → expect 200",
		adjustmentPayload("tx-rft-002", "REFUND", "idem-rft-002", "tx-rft-001", "APPROVED", 10000), 200)
	return r.result("refund_total"), nil
}

func scenarioRefundPartialSingle(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE APPROVED (R$100,00)", purchasePayload("tx-rfps-001", "idem-rfps-001", "APPROVED", 10000), 200)
	r.post("POST REFUND partial (R$40,00) → expect 200",
		adjustmentPayload("tx-rfps-002", "REFUND", "idem-rfps-002", "tx-rfps-001", "APPROVED", 4000), 200)
	return r.result("refund_partial_single"), nil
}

// scenarioRefundPartialMultiple validates multiple partial refunds that sum exactly to the total purchase amount.
func scenarioRefundPartialMultiple(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	// Purchase: R$300,00 (30000 cents)
	// REFUND #1: R$150,00 + REFUND #2: R$150,00 = R$300,00 (total)
	r.post("POST PURCHASE APPROVED (R$300,00)", purchasePayload("tx-rfpm-001", "idem-rfpm-001", "APPROVED", 30000), 200)
	r.post("POST REFUND #1 partial (R$150,00) → expect 200",
		adjustmentPayload("tx-rfpm-002", "REFUND", "idem-rfpm-002", "tx-rfpm-001", "APPROVED", 15000), 200)
	r.post("POST REFUND #2 partial (R$150,00) completing full refund → expect 200",
		adjustmentPayload("tx-rfpm-003", "REFUND", "idem-rfpm-003", "tx-rfpm-001", "APPROVED", 15000), 200)
	return r.result("refund_partial_multiple"), nil
}

func scenarioRefundExceedsAmount(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE APPROVED (R$100,00)", purchasePayload("tx-rfea-001", "idem-rfea-001", "APPROVED", 10000), 200)
	r.post("POST REFUND exceeding original amount (R$150,00) → expect 409",
		adjustmentPayload("tx-rfea-002", "REFUND", "idem-rfea-002", "tx-rfea-001", "APPROVED", 15000), 409)
	return r.result("refund_exceeds_amount"), nil
}

// scenarioRefundOnRejectedPurchase validates that refunds on rejected purchases are rejected with 409.
func scenarioRefundOnRejectedPurchase(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE REJECTED (R$100,00)", purchasePayload("tx-rforp-001", "idem-rforp-001", "REJECTED", 10000), 200)
	r.post("POST REFUND on rejected purchase → expect 409",
		adjustmentPayload("tx-rforp-002", "REFUND", "idem-rforp-002", "tx-rforp-001", "APPROVED", 10000), 409)
	return r.result("refund_on_rejected_purchase"), nil
}

// scenarioMultipleAdjustmentsExceed validates that the cumulative sum of adjustments cannot exceed the original amount.
func scenarioMultipleAdjustmentsExceed(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	// Purchase: R$100,00. Two refunds each R$60,00 → second one exceeds remaining R$40,00.
	r.post("POST PURCHASE APPROVED (R$100,00)", purchasePayload("tx-mae-001", "idem-mae-001", "APPROVED", 10000), 200)
	r.post("POST REFUND #1 (R$60,00) → expect 200",
		adjustmentPayload("tx-mae-002", "REFUND", "idem-mae-002", "tx-mae-001", "APPROVED", 6000), 200)
	r.post("POST REFUND #2 (R$60,00, cumulative R$120,00 > R$100,00) → expect 409",
		adjustmentPayload("tx-mae-003", "REFUND", "idem-mae-003", "tx-mae-001", "APPROVED", 6000), 409)
	return r.result("multiple_adjustments_exceed"), nil
}

// ── Mixed adjustment flows ────────────────────────────────────────────────────

func scenarioReversalAfterPartialRefund(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE APPROVED (R$100,00)", purchasePayload("tx-rapf-001", "idem-rapf-001", "APPROVED", 10000), 200)
	r.post("POST REFUND partial (R$60,00) → expect 200",
		adjustmentPayload("tx-rapf-002", "REFUND", "idem-rapf-002", "tx-rapf-001", "APPROVED", 6000), 200)
	r.post("POST REVERSAL_PURCHASE total (R$100,00, cumulative R$160,00 > R$100,00) → expect 409",
		adjustmentPayload("tx-rapf-003", "REVERSAL_PURCHASE", "idem-rapf-003", "tx-rapf-001", "APPROVED", 10000), 409)
	return r.result("reversal_after_partial_refund"), nil
}

// ── Idempotency & delivery flows ──────────────────────────────────────────────

func scenarioDuplicateEvent(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST PURCHASE APPROVED (first delivery)", purchasePayload("tx-de-001", "idem-de-001", "APPROVED", 10000), 200)
	r.post("POST PURCHASE APPROVED (duplicate idempotency_key, same event) → expect 200 with idempotent=true",
		purchasePayload("tx-de-001", "idem-de-001", "APPROVED", 10000), 200)
	return r.result("duplicate_event"), nil
}

func scenarioOutOfOrder(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	r.post("POST REFUND before original PURCHASE (out-of-order) → expect 404",
		adjustmentPayload("tx-ooo-002", "REFUND", "idem-ooo-002", "tx-ooo-001", "APPROVED", 5000), 404)
	r.post("POST PURCHASE APPROVED (original arrives late) → expect 200",
		purchasePayload("tx-ooo-001", "idem-ooo-001", "APPROVED", 10000), 200)
	r.post("POST REFUND retry (now original exists) → expect 200",
		adjustmentPayload("tx-ooo-002", "REFUND", "idem-ooo-003", "tx-ooo-001", "APPROVED", 5000), 200)
	return r.result("out_of_order"), nil
}

func scenarioWebhookRetry(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	// Step 1: first delivery — success.
	r.post("POST PURCHASE APPROVED (first delivery) → expect 200",
		purchasePayload("tx-wr-001", "idem-wr-001", "APPROVED", 10000), 200)
	// Step 2: Pomelo retries the same event with the same idempotency_key (network retry, no ACK received).
	// The system must recognize it as a duplicate and return 200 with idempotent=true.
	r.post("POST PURCHASE retry (same idempotency_key, Pomelo network retry) → expect 200 idempotent=true",
		purchasePayload("tx-wr-001", "idem-wr-001", "APPROVED", 10000), 200)
	return r.result("webhook_retry"), nil
}

// ── Validation error flows ────────────────────────────────────────────────────

// scenarioMissingOriginalTransactionID validates that adjustments without original_transaction_id are rejected with 400.
func scenarioMissingOriginalTransactionID(baseURL string) (ScenarioResult, error) {
	r := newRunner(baseURL)
	// Build a REFUND payload with empty original_transaction_id.
	payload := adjustmentPayload("tx-moti-001", "REFUND", "idem-moti-001", "", "APPROVED", 5000)
	r.post("POST REFUND without original_transaction_id → expect 400", payload, 400)
	return r.result("missing_original_transaction_id"), nil
}

// availableScenarios returns all scenario names.
func availableScenarios() []string {
	return []string{
		// Basic purchase
		"purchase_approved",
		"purchase_rejected",
		"purchase_at_min_amount",
		"purchase_at_max_amount",
		"purchase_amount_too_low",
		"purchase_amount_too_high",
		"purchase_negative_amount",
		// Reversal
		"reversal_total",
		"reversal_partial",
		"reversal_exceeds_amount",
		"reversal_on_rejected_purchase",
		// Refund
		"refund_total",
		"refund_partial_single",
		"refund_partial_multiple",
		"refund_exceeds_amount",
		"refund_on_rejected_purchase",
		"multiple_adjustments_exceed",
		// Mixed
		"reversal_after_partial_refund",
		// Idempotency & delivery
		"duplicate_event",
		"out_of_order",
		"webhook_retry",
		// Validation errors
		"missing_original_transaction_id",
	}
}
