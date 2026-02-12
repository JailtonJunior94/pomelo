package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

const protocolVersion = "2024-11-05"

// JSON-RPC 2.0 types

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP tool definitions

type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolCallResult struct {
	Content []contentItem `json:"content"`
}

// Server runs the MCP JSON-RPC 2.0 server over stdin/stdout.
type Server struct {
	baseURL string
	logger  *slog.Logger
	writer  *bufio.Writer
}

// NewServer creates an MCP server that calls baseURL for all HTTP requests.
func NewServer(baseURL string) *Server {
	return &Server{
		baseURL: baseURL,
		logger:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
		writer:  bufio.NewWriter(os.Stdout),
	}
}

// Run starts reading JSON-RPC requests from stdin and writing responses to stdout.
func (s *Server) Run() {
	s.logger.Info("MCP server started", "baseURL", s.baseURL)
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer for large payloads
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Error("failed to parse request", "err", err)
			s.writeError(nil, -32700, "parse error")
			continue
		}
		s.logger.Info("request received", "method", req.Method, "id", req.ID)
		s.dispatch(req)
	}
	if err := scanner.Err(); err != nil {
		s.logger.Error("scanner error", "err", err)
	}
}

func (s *Server) dispatch(req jsonRPCRequest) {
	switch req.Method {
	case "initialize":
		s.writeResult(req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "pomelo-simulator", "version": "1.0.0"},
		})
	case "notifications/initialized":
		// No-op notification — no response for notifications
	case "tools/list":
		s.writeResult(req.ID, map[string]any{
			"tools": s.toolDefinitions(),
		})
	case "tools/call":
		s.handleToolCall(req)
	default:
		s.writeError(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleToolCall(req jsonRPCRequest) {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(req.ID, -32602, "invalid params")
		return
	}

	var resultText string
	var toolErr error

	switch params.Name {
	case "simulate_purchase":
		resultText, toolErr = s.toolSimulatePurchase(params.Arguments)
	case "simulate_reversal":
		resultText, toolErr = s.toolSimulateReversal(params.Arguments)
	case "simulate_refund":
		resultText, toolErr = s.toolSimulateRefund(params.Arguments)
	case "simulate_scenario":
		resultText, toolErr = s.toolSimulateScenario(params.Arguments)
	default:
		s.writeError(req.ID, -32601, fmt.Sprintf("unknown tool: %s", params.Name))
		return
	}

	if toolErr != nil {
		s.writeError(req.ID, -32603, toolErr.Error())
		return
	}
	s.writeResult(req.ID, toolCallResult{
		Content: []contentItem{{Type: "text", Text: resultText}},
	})
}

// --- Tool implementations ---

func (s *Server) toolSimulatePurchase(args json.RawMessage) (string, error) {
	var p struct {
		TransactionID  string `json:"transaction_id"`
		IdempotencyKey string `json:"idempotency_key"`
		Status         string `json:"status"`
		Amount         int64  `json:"amount"`
		Currency       string `json:"currency"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if p.TransactionID == "" {
		p.TransactionID = generateID("tx")
	}
	if p.IdempotencyKey == "" {
		p.IdempotencyKey = generateID("idem")
	}
	if p.Status == "" {
		p.Status = "APPROVED"
	}
	if p.Amount == 0 {
		p.Amount = 10000
	}
	if p.Currency == "" {
		p.Currency = "BRL"
	}

	r := newRunner(s.baseURL)
	r.post("simulate_purchase", purchasePayload(p.TransactionID, p.IdempotencyKey, p.Status, p.Amount), 200)
	result := r.result("simulate_purchase")
	return marshalResult(result)
}

func (s *Server) toolSimulateReversal(args json.RawMessage) (string, error) {
	var p struct {
		TransactionID         string `json:"transaction_id"`
		IdempotencyKey        string `json:"idempotency_key"`
		OriginalTransactionID string `json:"original_transaction_id"`
		Amount                int64  `json:"amount"`
		Currency              string `json:"currency"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if p.OriginalTransactionID == "" {
		return "", fmt.Errorf("original_transaction_id is required")
	}
	if p.TransactionID == "" {
		p.TransactionID = generateID("rev")
	}
	if p.IdempotencyKey == "" {
		p.IdempotencyKey = generateID("idem")
	}
	if p.Amount == 0 {
		p.Amount = 10000
	}
	if p.Currency == "" {
		p.Currency = "BRL"
	}

	r := newRunner(s.baseURL)
	r.post("simulate_reversal", adjustmentPayload(p.TransactionID, "REVERSAL_PURCHASE", p.IdempotencyKey, p.OriginalTransactionID, "APPROVED", p.Amount), 200)
	result := r.result("simulate_reversal")
	return marshalResult(result)
}

func (s *Server) toolSimulateRefund(args json.RawMessage) (string, error) {
	var p struct {
		TransactionID         string `json:"transaction_id"`
		IdempotencyKey        string `json:"idempotency_key"`
		OriginalTransactionID string `json:"original_transaction_id"`
		Amount                int64  `json:"amount"`
		Currency              string `json:"currency"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if p.OriginalTransactionID == "" {
		return "", fmt.Errorf("original_transaction_id is required")
	}
	if p.TransactionID == "" {
		p.TransactionID = generateID("ref")
	}
	if p.IdempotencyKey == "" {
		p.IdempotencyKey = generateID("idem")
	}
	if p.Amount == 0 {
		p.Amount = 10000
	}
	if p.Currency == "" {
		p.Currency = "BRL"
	}

	r := newRunner(s.baseURL)
	r.post("simulate_refund", adjustmentPayload(p.TransactionID, "REFUND", p.IdempotencyKey, p.OriginalTransactionID, "APPROVED", p.Amount), 200)
	result := r.result("simulate_refund")
	return marshalResult(result)
}

func (s *Server) toolSimulateScenario(args json.RawMessage) (string, error) {
	var p struct {
		Scenario string `json:"scenario"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Scenario == "" {
		return "", fmt.Errorf("scenario is required. available: %v", availableScenarios())
	}
	result, err := runScenario(s.baseURL, p.Scenario)
	if err != nil {
		return "", err
	}
	return marshalResult(result)
}

// --- Tool definitions ---

func (s *Server) toolDefinitions() []toolDefinition {
	return []toolDefinition{
		{
			Name:        "simulate_purchase",
			Description: "Send a PURCHASE webhook to the Pomelo server",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"transaction_id":  map[string]any{"type": "string", "description": "Transaction ID (auto-generated if empty)"},
					"idempotency_key": map[string]any{"type": "string", "description": "Idempotency key (auto-generated if empty)"},
					"status":          map[string]any{"type": "string", "enum": []string{"APPROVED", "REJECTED"}, "description": "Transaction status"},
					"amount":          map[string]any{"type": "integer", "description": "Amount in cents (default: 10000)"},
					"currency":        map[string]any{"type": "string", "description": "Currency code (default: BRL)"},
				},
			},
		},
		{
			Name:        "simulate_reversal",
			Description: "Send a REVERSAL_PURCHASE webhook to the Pomelo server",
			InputSchema: map[string]any{
				"type": "object",
				"required": []string{"original_transaction_id"},
				"properties": map[string]any{
					"transaction_id":          map[string]any{"type": "string"},
					"idempotency_key":         map[string]any{"type": "string"},
					"original_transaction_id": map[string]any{"type": "string", "description": "ID of the original PURCHASE"},
					"amount":                  map[string]any{"type": "integer", "description": "Amount in cents"},
					"currency":                map[string]any{"type": "string"},
				},
			},
		},
		{
			Name:        "simulate_refund",
			Description: "Send a REFUND webhook to the Pomelo server",
			InputSchema: map[string]any{
				"type": "object",
				"required": []string{"original_transaction_id"},
				"properties": map[string]any{
					"transaction_id":          map[string]any{"type": "string"},
					"idempotency_key":         map[string]any{"type": "string"},
					"original_transaction_id": map[string]any{"type": "string", "description": "ID of the original PURCHASE"},
					"amount":                  map[string]any{"type": "integer", "description": "Amount in cents"},
					"currency":                map[string]any{"type": "string"},
				},
			},
		},
		{
			Name:        "simulate_scenario",
			Description: fmt.Sprintf("Run a predefined end-to-end scenario. Available: %v", availableScenarios()),
			InputSchema: map[string]any{
				"type": "object",
				"required": []string{"scenario"},
				"properties": map[string]any{
					"scenario": map[string]any{
						"type":        "string",
						"description": "Scenario name",
						"enum":        availableScenarios(),
					},
				},
			},
		},
	}
}

// --- Helpers ---

func marshalResult(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var idCounter atomic.Int64

func generateID(prefix string) string {
	n := idCounter.Add(1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), n)
}

func (s *Server) writeResult(id any, result any) {
	s.write(jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) writeError(id any, code int, message string) {
	s.write(jsonRPCResponse{JSONRPC: "2.0", ID: id, Error: &jsonRPCError{Code: code, Message: message}})
}

func (s *Server) write(resp jsonRPCResponse) {
	b, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("failed to marshal response", "err", err)
		return
	}
	s.logger.Info("response sent", "id", resp.ID)
	s.writer.Write(b)
	s.writer.WriteByte('\n')
	s.writer.Flush()
}
