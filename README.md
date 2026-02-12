# Pomelo Webhook Simulator

Servidor Go que recebe webhooks de transações de cartão débito da Pomelo e um simulador MCP para disparar todos os cenários possíveis end-to-end.

## Tecnologias

| | |
|---|---|
| **Linguagem** | Go 1.25 |
| **Arquitetura** | Hexagonal (Ports & Adapters) |
| **HTTP** | `net/http` stdlib — ServeMux com method+path routing (Go 1.22) |
| **Logging** | `log/slog` structured logging (Go 1.21) |
| **Concorrência** | `sync.RWMutex`, `sync.WaitGroup.Go` (Go 1.25), `sync/atomic.Int64` |
| **Coleções** | `slices`, `maps` stdlib (Go 1.21/1.23) |
| **Simulador** | MCP JSON-RPC 2.0 sobre stdin/stdout |
| **Testes** | `testing` stdlib — sem frameworks externos |
| **Race detector** | `go test -race` |
| **Container** | Docker multi-stage (`golang:1.25-alpine` → `scratch`) |
| **Orquestração** | Docker Compose com healthcheck |
| **Dependências externas** | **zero** |

---

## Arquitetura

O projeto segue arquitetura hexagonal estrita. Cada camada depende apenas da camada interior:

```
┌──────────────────────────────────────────────────────────────┐
│                        cmd/server                            │
│                    (composition root)                        │
└──────────────┬───────────────────────────┬───────────────────┘
               │                           │
               ▼                           ▼
┌──────────────────────┐     ┌─────────────────────────────┐
│  adapters/input/http │     │  adapters/output/memory     │
│  DTO → Command       │     │  Repository (RWMutex)       │
│  HTTP status mapping │     │  idempotency atômica        │
└──────────┬───────────┘     └──────────────┬──────────────┘
           │                                │
           ▼                                ▼
┌──────────────────────────────────────────────────────────────┐
│                    application/service                       │
│   ProcessTransaction  ·  GetTransaction  ·  ListTransactions │
└──────────────────────────────┬───────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────┐
│                         domain                               │
│  Transaction (PURCHASE)  ·  Adjustment (REVERSAL / REFUND)  │
│  Money (int64 centavos)  ·  Erros sentinela                  │
└──────────────────────────────────────────────────────────────┘
```

### Estrutura de diretórios

```
pomelo/
├── cmd/
│   ├── server/main.go          # HTTP server — composition root
│   └── simulator/main.go       # MCP simulator binary
├── deployment/
│   ├── Dockerfile              # imagem do servidor
│   ├── Dockerfile.simulator    # imagem do simulador
│   └── docker-compose.yml      # orquestra os dois serviços
├── internal/
│   ├── domain/
│   │   ├── money.go            # Money value object (int64 centavos)
│   │   ├── errors.go           # erros sentinela do domínio
│   │   ├── transaction.go      # agregado Transaction (PURCHASE)
│   │   └── adjustment.go       # entidade Adjustment (REVERSAL / REFUND)
│   ├── application/
│   │   ├── ports/
│   │   │   ├── input.go        # interface WebhookUseCase + Command/Result
│   │   │   └── output.go       # interface TransactionRepository
│   │   └── service.go          # orquestração dos use cases
│   └── adapters/
│       ├── input/http/
│       │   ├── dto.go          # WebhookRequestDTO + ToCommand()
│       │   └── handler.go      # handlers net/http
│       └── output/memory/
│           └── repository.go   # repositório in-memory thread-safe
└── simulator/
    └── mcp/
        ├── server.go           # servidor MCP JSON-RPC 2.0 stdin/stdout
        └── scenarios.go        # 4 tools + 29 cenários pré-definidos
```

---

## Fluxo de uma transação

### PURCHASE

```
Pomelo                  HTTP Adapter              Application             Domain
  │                         │                         │                     │
  │── POST /webhook/... ───▶│                         │                     │
  │                         │── ToCommand() ─────────▶│                     │
  │                         │                         │── GetByIdempKey() ──▶│
  │                         │                         │   (idempotência)     │
  │                         │                         │── NewPurchase() ────▶│
  │                         │                         │                     │── valida campos
  │                         │                         │◀── Transaction ──────│
  │                         │                         │── SaveTransaction() ─▶ Repository
  │                         │◀── Result ──────────────│                     │
  │◀── 200 OK ─────────────│                         │                     │
```

### REVERSAL / REFUND

```
Pomelo                  HTTP Adapter              Application             Domain
  │                         │                         │                     │
  │── POST /webhook/... ───▶│                         │                     │
  │                         │── ToCommand() ─────────▶│                     │
  │                         │                         │── GetByIdempKey()   │
  │                         │                         │── GetTransactionByID() ──▶ Repository
  │                         │                         │── NewAdjustment() ─▶│
  │                         │                         │── GetAdjustments() ──▶ Repository
  │                         │                         │── sumApproved()     │
  │                         │                         │── ValidateAgainst() ▶│
  │                         │                         │                     │── existingTotal + adj > original?
  │                         │                         │                     │── purchase.IsApproved?
  │                         │                         │── SaveAdjustment() ──▶ Repository
  │                         │◀── Result ──────────────│                     │
  │◀── 200 OK ─────────────│                         │                     │
```

### Mapeamento de erros domínio → HTTP

| Erro de domínio | HTTP | Code |
|---|---|---|
| `ErrDuplicateIdempotencyKey` | `200` | — (`idempotent: true`) |
| `ErrTransactionNotFound` | `404` | `NOT_FOUND` |
| `ErrExceedsOriginalAmount` | `409` | `EXCEEDS_ORIGINAL_AMOUNT` |
| `ErrPurchaseNotApproved` | `409` | `PURCHASE_NOT_APPROVED` |
| `ErrAmountOutOfRange` | `422` | `AMOUNT_OUT_OF_RANGE` |
| `ErrNegativeAmount` | `400` | `NEGATIVE_AMOUNT` |
| `ErrOriginalTransactionRequired` | `400` | `ORIGINAL_TRANSACTION_REQUIRED` |
| `ErrCurrencyMismatch` | `400` | `CURRENCY_MISMATCH` |
| outros | `500` | `INTERNAL_ERROR` |

---

## API

### `POST /webhook/transactions`

Recebe qualquer tipo de transação Pomelo.

**Body:**
```json
{
  "id": "tx-001",
  "type": "PURCHASE",
  "status": "APPROVED",
  "original_transaction_id": "",
  "amount": {
    "local":       { "total": 10000, "currency": "BRL" },
    "transaction": { "total": 10000, "currency": "BRL" },
    "settlement":  { "total": 10000, "currency": "BRL" },
    "original":    { "total": 10000, "currency": "BRL" }
  },
  "merchant": {
    "id": "MERCH-001", "mcc": "5411",
    "name": "Supermercado", "city": "SP", "state": "SP"
  },
  "event": {
    "id": "evt-001",
    "created_at": "2024-06-01T10:00:00Z",
    "idempotency_key": "ctx-unique-key-001"
  },
  "user_id": "user-001",
  "card_id":  "card-001",
  "country":  "BR",
  "currency": "BRL",
  "point_of_sale": "ONLINE"
}
```

**Resposta 200:**
```json
{ "transaction_id": "tx-001", "idempotent": false, "message": "transaction processed" }
```

**Resposta 200 (evento duplicado):**
```json
{ "transaction_id": "tx-001", "idempotent": true, "message": "duplicate event, already processed" }
```

**Resposta de erro:**
```json
{ "error": "total adjustments exceed original purchase amount", "code": "EXCEEDS_ORIGINAL_AMOUNT" }
```

---

### `GET /transactions/{id}`

Retorna uma transação pelo ID.

```bash
curl http://localhost:8080/transactions/tx-001
```

### `GET /transactions`

Lista todas as transações armazenadas.

```bash
curl http://localhost:8080/transactions
```

### `GET /health`

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

---

## Tipos de transação

| `type` | Descrição | `original_transaction_id` |
|---|---|---|
| `PURCHASE` | Compra no débito | vazio |
| `REVERSAL_PURCHASE` | Estorno total ou parcial | ID da PURCHASE original |
| `REFUND` | Devolução total ou parcial | ID da PURCHASE original |

**Regras de negócio:**
- O valor da PURCHASE deve estar entre **R$1,00** (100 centavos) e **R$5.000,00** (500.000 centavos)
- Valores negativos são rejeitados com `400`; valores fora do intervalo com `422`
- REVERSAL e REFUND só podem ser aplicados a uma PURCHASE com `status = APPROVED`
- A soma de todos os ajustes `APPROVED` não pode exceder o `amount.local.total` original
- REVERSAL e REFUND requerem `original_transaction_id` preenchido
- Idempotência garantida por `event.idempotency_key`
- Out-of-order: retorna `404` se a PURCHASE ainda não chegou; cliente faz retry

---

## Simulador MCP

O simulador é um servidor **MCP (Model Context Protocol) JSON-RPC 2.0** que roda sobre stdin/stdout. Ele expõe 4 tools e **29 cenários pré-definidos**.

### Tools disponíveis

| Tool | Descrição |
|---|---|
| `simulate_purchase` | Dispara uma PURCHASE com parâmetros customizáveis |
| `simulate_reversal` | Dispara um REVERSAL_PURCHASE |
| `simulate_refund` | Dispara um REFUND |
| `simulate_scenario` | Executa um cenário completo pré-definido |

### Como usar com Claude Desktop / VS Code

Adicione ao `claude_desktop_config.json` (ou equivalente do cliente MCP):

```json
{
  "mcpServers": {
    "pomelo-simulator": {
      "command": "go",
      "args": ["run", "./cmd/simulator"],
      "cwd": "/caminho/para/pomelo",
      "env": { "WEBHOOK_URL": "http://localhost:8080" }
    }
  }
}
```

Depois basta pedir ao assistente: _"execute o cenário `refund_partial_multiple`"_ ou _"rode todos os cenários de reversal"_.

### Cenários pré-definidos (`simulate_scenario`)

#### Compras básicas

| Cenário | Passos | HTTP esperado |
|---|---|---|
| `purchase_approved` | PURCHASE APPROVED R$100,00 | `200` |
| `purchase_rejected` | PURCHASE REJECTED R$100,00 | `200` |
| `purchase_at_min_amount` | PURCHASE no valor mínimo R$1,00 (100 cents) | `200` |
| `purchase_at_max_amount` | PURCHASE no valor máximo R$5.000,00 (500.000 cents) | `200` |
| `purchase_amount_too_low` | PURCHASE R$0,50 — abaixo do mínimo | `422` |
| `purchase_amount_too_high` | PURCHASE R$6.000,00 — acima do máximo | `422` |
| `purchase_negative_amount` | PURCHASE com valor negativo | `400` |

#### Reversais (REVERSAL_PURCHASE)

| Cenário | Passos | HTTP esperado |
|---|---|---|
| `reversal_total` | PURCHASE + REVERSAL total R$100,00 | `200` → `200` |
| `reversal_partial` | PURCHASE + REVERSAL parcial R$50,00 | `200` → `200` |
| `reversal_exceeds_amount` | PURCHASE + REVERSAL R$150 (> R$100 original) | `200` → `409` |
| `reversal_on_rejected_purchase` | PURCHASE REJECTED + REVERSAL | `200` → `409` |

#### Reembolsos (REFUND)

| Cenário | Passos | HTTP esperado |
|---|---|---|
| `refund_total` | PURCHASE + REFUND total R$100,00 | `200` → `200` |
| `refund_partial_single` | PURCHASE + REFUND parcial R$40,00 | `200` → `200` |
| `refund_partial_multiple` | PURCHASE R$300 + REFUND₁ R$150 + REFUND₂ R$150 (soma = total) | `200` → `200` → `200` |
| `refund_exceeds_amount` | PURCHASE + REFUND R$150 (> R$100 original) | `200` → `409` |
| `refund_on_rejected_purchase` | PURCHASE REJECTED + REFUND | `200` → `409` |
| `multiple_adjustments_exceed` | PURCHASE R$100 + REFUND₁ R$60 + REFUND₂ R$60 (acumulado R$120 > R$100) | `200` → `200` → `409` |

#### Ajustes mistos

| Cenário | Passos | HTTP esperado |
|---|---|---|
| `reversal_after_partial_refund` | PURCHASE R$100 + REFUND R$60 + REVERSAL R$100 (acumulado > original) | `200` → `200` → `409` |

#### Idempotência e entrega

| Cenário | Passos | HTTP esperado |
|---|---|---|
| `duplicate_event` | PURCHASE + reenvio com a **mesma** `idempotency_key` | `200` → `200` (`idempotent=true`) |
| `out_of_order` | REFUND sem PURCHASE → PURCHASE → REFUND retry | `404` → `200` → `200` |
| `webhook_retry` | PURCHASE + retry de rede (mesma `idempotency_key`, mesmo `tx_id`) | `200` → `200` (`idempotent=true`) |

#### Erros de validação

| Cenário | Passos | HTTP esperado |
|---|---|---|
| `missing_original_transaction_id` | REFUND sem `original_transaction_id` | `400` (`ORIGINAL_TRANSACTION_REQUIRED`) |
| `missing_id` | POST sem campo `id` | `400` |
| `missing_idempotency_key` | POST com `idempotency_key` vazia | `400` |
| `invalid_created_at` | POST com `created_at` em formato inválido (non-RFC3339) | `400` |
| `invalid_json_body` | POST com corpo que não é JSON | `400` |

#### Consultas

| Cenário | Passos | HTTP esperado |
|---|---|---|
| `list_transactions` | PURCHASE seed → GET `/transactions` | `200` → `200` (array) |
| `get_transaction_existing` | PURCHASE seed → GET `/transactions/:id` | `200` → `200` |
| `get_transaction_not_found` | GET `/transactions/id-inexistente` | `404` (`NOT_FOUND`) |

---

## Como executar

### Pré-requisitos

- Go 1.25+ — [download](https://go.dev/dl/)
- Docker + Docker Compose (opcional)

### Localmente

```bash
# Terminal 1 — servidor HTTP
go run ./cmd/server
# time=... level=INFO msg="pomelo webhook server listening" addr=:8080

# Terminal 2 — simulador MCP
WEBHOOK_URL=http://localhost:8080 go run ./cmd/simulator
# time=... level=INFO msg="MCP server started" baseURL=http://localhost:8080
```

**Enviar um cenário via stdin (JSON-RPC 2.0):**
```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"simulate_scenario","arguments":{"scenario":"refund_partial_multiple"}}}' \
  | WEBHOOK_URL=http://localhost:8080 go run ./cmd/simulator
```

### Docker Compose

```bash
# A partir da raiz do projeto
docker compose -f deployment/docker-compose.yml up --build

# Só o servidor
docker compose -f deployment/docker-compose.yml up server --build
```

O Compose aguarda o healthcheck do `server` passar antes de subir o `simulator`.

### Testes

```bash
# Todos os testes com race detector
go test -race ./...

# Verbose
go test -v -race ./...

# Pacote específico
go test -race ./internal/domain/...
go test -race ./internal/application/...
go test -race ./internal/adapters/...
```

**Cobertura:**

| Pacote | Testes |
|---|---|
| `domain` | `NewMoney`, `NewPurchase` (inclui limites R$1–R$5.000), `NewAdjustment`, `ValidateAgainstPurchase` |
| `application` | 17 casos com mock inline: aprovado, rejeitado, reversais, reembolsos, idempotência, valor fora do intervalo, valor negativo, tipo inválido |
| `adapters/http` | 11 casos de status HTTP via `httptest` (inclui 422 `AMOUNT_OUT_OF_RANGE`) |
| `adapters/memory` | CRUD + 200 goroutines paralelas (`-race`) |

---

## Postman

A collection e o environment estão prontos para importar:

```
docs/pomelo-webhook.postman_collection.json   # 70+ requests, 19 pastas, testes automáticos
docs/pomelo-local.postman_environment.json    # base_url + variáveis de runtime
```

**Importar:**
1. Postman → **Import** → selecione os dois arquivos de `docs/`
2. Selecione o environment **Pomelo Local** no canto superior direito
3. Inicie o servidor (`go run ./cmd/server`) → **Run collection**

As pastas cobrem os mesmos 29 cenários do simulador MCP, organizadas por categoria:

| Pasta | Cenários |
|---|---|
| 01–02 | PURCHASE aprovada e rejeitada |
| 03–04 | REVERSAL total e parcial |
| 05–07 | REFUND total, parcial único, parcial múltiplo |
| 08–09 | REFUND e REVERSAL excedendo o valor original |
| 10–12 | Idempotência, out-of-order, webhook retry |
| 13 | Ajuste em PURCHASE rejeitada |
| 14 | Validação de campos obrigatórios |
| 15 | Limites de valor (mínimo, máximo, negativo) |
| 16–18 | REVERSAL/REFUND com erros de conflito |
| 19 | Consultas (listar, buscar por ID, 404) |

---

## Domínio

### Money

Valor objeto imutável em centavos (`int64`). Sem conversão de float.

```
NewMoney(amount int64, currency string) → rejeita amount < 0
Add(other Money) → erro se moedas diferentes
GreaterThan(other Money) bool
```

### Transaction (PURCHASE)

Agregado raiz. Imutável após construção.

```
NewPurchase(...) → valida id, event.id, idempotency_key
                → rejeita amount < R$1,00 (100 centavos) ou > R$5.000,00 (500.000 centavos)
IsApprovedPurchase() bool
CanReceiveAdjustment() bool
```

### Adjustment (REVERSAL_PURCHASE / REFUND)

Entidade imutável após construção.

```
NewAdjustment(...) → valida type ∈ {REVERSAL_PURCHASE, REFUND}, original_transaction_id obrigatório
ValidateAgainstPurchase(original, existingTotal) → ErrPurchaseNotApproved | ErrExceedsOriginalAmount
```

### Invariantes

1. Valor da PURCHASE entre R$1,00 e R$5.000,00 (centavos: 100–500.000)
2. Nenhum ajuste pode exceder o valor original da PURCHASE (verificação acumulada)
3. PURCHASE não é mutada — ajustes são entidades separadas
4. Verificação de idempotência + gravação são atômicas sob o mesmo mutex
5. REVERSAL e REFUND exigem PURCHASE com `status = APPROVED`
6. REVERSAL e REFUND exigem `original_transaction_id` não-vazio
7. Out-of-order falha com `404` — sem buffering interno

---

## Decisões de design

**Por que `int64` para valores monetários?**
Evita erros de arredondamento de ponto flutuante. Todos os valores trafegam em centavos.

**Por que repositório in-memory?**
O projeto é um simulador/sandbox. O repositório implementa a interface `TransactionRepository` — trocar por Postgres, Redis ou DynamoDB é uma mudança apenas no adapter de saída, sem tocar em domínio ou application.

**Por que MCP sobre stdin/stdout?**
O simulador é projetado para ser plugado diretamente em clientes MCP (Claude Desktop, VS Code, etc.) sem nenhuma configuração de rede adicional.

**Por que `FROM scratch` no Dockerfile?**
Binário estático compilado com `CGO_ENABLED=0` — imagem final sem shell, sem libc, sem surface de ataque. Tamanho típico: ~6 MB.
