FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /deckhouse-mcp ./cmd/deckhouse-harness

FROM gcr.io/distroless/static-debian12
COPY --from=builder /deckhouse-mcp /deckhouse-mcp
USER nonroot:nonroot
ENTRYPOINT ["/deckhouse-mcp"]
