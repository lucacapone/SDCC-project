# syntax=docker/dockerfile:1

# Usa la stessa toolchain Go dell'immagine canonica per produrre il nodo.
FROM golang:1.22 AS builder

# Imposta una directory di build isolata e riproducibile.
WORKDIR /src

# Copia il manifest del modulo prima dei sorgenti per favorire la cache Docker.
COPY go.mod ./

# Copia l'entrypoint applicativo compilato anche dall'immagine canonica.
COPY cmd ./cmd

# Copia i package interni richiesti dal nodo.
COPY internal ./internal

# Compila lo stesso binario statico cmd/node usato dal Dockerfile canonico.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/sdcc-node ./cmd/node

# Usa Alpine come runtime sperimentale minimale con package manager e shell.
FROM alpine:3.20 AS runtime

# Installa tc/ip, certificati e su-exec senza conservare l'indice dei package.
RUN apk add --no-cache ca-certificates iproute2 su-exec

# Copia il binario Go statico prodotto nello stage builder.
COPY --from=builder /out/sdcc-node /usr/local/bin/sdcc-node

# Copia lo script che configura NetEm e abbandona i privilegi.
COPY deploy/traffic-control/entrypoint.sh /usr/local/bin/traffic-control-entrypoint

# Rende eseguibile l'entrypoint dedicato.
RUN chmod 0755 /usr/local/bin/traffic-control-entrypoint

# Avvia sempre la configurazione sperimentale prima del nodo.
ENTRYPOINT ["/usr/local/bin/traffic-control-entrypoint"]

# Mantiene gli stessi argomenti predefiniti del runtime canonico.
CMD ["--config", "/config/config.yaml"]
