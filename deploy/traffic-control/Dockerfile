# syntax=docker/dockerfile:1

# Lo stage di build usa lo stesso package cmd/node della modalita normale.
FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/sdcc-node ./cmd/node

# Alpine fornisce una shell, tc/ip e curl senza alterare l'immagine normale.
FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates curl iproute2
COPY --from=builder /out/sdcc-node /usr/local/bin/sdcc-node
COPY deploy/traffic-control/entrypoint.sh /usr/local/bin/sdcc-tc-entrypoint
RUN chmod 0755 /usr/local/bin/sdcc-tc-entrypoint \
    && mkdir -p /var/lib/sdcc

ENTRYPOINT ["/usr/local/bin/sdcc-tc-entrypoint"]
CMD ["--config", "/config/config.yaml"]
