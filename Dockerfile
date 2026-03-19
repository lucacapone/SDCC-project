# syntax=docker/dockerfile:1

FROM golang:1.22 AS builder

WORKDIR /src

# Copia i manifest per sfruttare la cache del layer delle dipendenze.
COPY go.mod ./

# Copia il sorgente applicativo necessario alla build del nodo.
COPY cmd ./cmd
COPY internal ./internal

# Compila un binario statico adatto a un runtime minimale.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags='-s -w' -o /out/sdcc-node ./cmd/node

FROM gcr.io/distroless/static-debian12:nonroot AS runtime

WORKDIR /

# Copia il binario compilato nello stage runtime minimale.
COPY --from=builder /out/sdcc-node /usr/local/bin/sdcc-node

ENTRYPOINT ["/usr/local/bin/sdcc-node"]
CMD ["--config", "/config/config.yaml"]
