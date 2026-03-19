# Deployment locale con Docker Compose

## Obiettivo
Il cluster locale SDCC viene avviato tramite un'immagine applicativa costruita localmente dal repository, invece di eseguire `go run` dentro un container `golang:1.22`.

## Artefatti coinvolti
- `Dockerfile`: definisce una build Go multi-stage che compila `./cmd/node` in `/out/sdcc-node` e copia il binario nello stage runtime minimale finale.
- `docker-compose.yml`: sorgente canonica del cluster locale multi-nodo; usa `build:` per costruire l'immagine `sdcc-node:local` e avvia i container passando `--config /config/config.yaml`.
- `deploy/docker-compose.yml`: promemoria storico che rimanda al file canonico di root.
- `configs/node*.yaml`: configurazioni montate in sola lettura dentro `/config/config.yaml`.

## Build multi-stage del nodo
La build del container segue questi passaggi:
1. stage `builder` basato su `golang:1.22`;
2. copia del codice necessario alla compilazione (`go.mod`, `cmd/`, `internal/`);
3. compilazione di `./cmd/node` in `/out/sdcc-node` con `CGO_ENABLED=0` per ottenere un binario compatibile con runtime minimale;
4. stage finale `distroless/static-debian12:nonroot`;
5. copia del binario finale in `/usr/local/bin/sdcc-node`;
6. avvio tramite `ENTRYPOINT ["/usr/local/bin/sdcc-node"]` e `CMD ["--config", "/config/config.yaml"]`.

## Avvio del cluster locale
Dalla root della repository:

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f node1
docker compose down
```

## Configurazione runtime
Ogni servizio del Compose:
- usa la stessa immagine applicativa locale (`sdcc-node:local`), costruita dal `Dockerfile` di root;
- monta il proprio file YAML in `/config/config.yaml`;
- riceve eventuali override runtime tramite variabili ambiente (`NODE_ID`, `NODE_PORT`, `SEED_PEERS`, `AGGREGATION`, ecc.).

## Note operative
- La build viene eseguita localmente dal Docker Engine usando il contenuto corrente della repository.
- Il file Compose canonico non monta più l'intero sorgente nel container runtime, perché l'eseguibile è già incluso nell'immagine applicativa.
- Per aggiornare il cluster dopo modifiche al codice è sufficiente rilanciare `docker compose up -d --build`.
