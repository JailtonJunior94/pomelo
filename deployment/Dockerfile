# ─── Stage 1: build ───────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Cache de dependências separado do código
COPY go.mod ./
RUN go mod download

COPY . .

# CGO desabilitado → binário estático
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ─── Stage 2: runtime ─────────────────────────────────────────────────────────
FROM scratch

COPY --from=builder /out/server /server

EXPOSE 8080

ENTRYPOINT ["/server"]
