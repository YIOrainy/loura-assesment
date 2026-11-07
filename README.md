# Support Ticket Classifier

## Run

With Docker only:

```bash
docker compose up --build -d      # or: make up
make samples                      # needs jq
curl localhost:8080/tickets/1
```

Locally, with Go 1.26+:

```bash
make db                                                        # Postgres on localhost:5433
cp cmd/ingestor/config.example.yaml cmd/ingestor/config.yaml
make run                                                       # :8080
make samples                                                   # another terminal
curl localhost:8080/tickets/1
```

## Decisions

**Storage.** One Postgres table holds the ticket, its result, and its place in the queue (`next_attempt_at`). There is no separate broker to keep in sync.

**Async work.** Everything runs in one process: the API, a sweeper and a worker pool. The backlog stays in Postgres. The channel only holds one batch.

**Concurrency.** 8 workers (`workers`), so at most 8 LLM calls run at once. The gateway also limits calls to 5/s (`llm_tokens_per_second`). A circuit breaker opens after 5 failures and stops calls for 30s.

**Restarts.** Nothing is lost. A ticket that was in flight stays `pending` with its lease, and is claimed again when the lease (2m) expires. If a lease expires mid-call, the LLM can be called twice, but a ticket is never classified twice.

**Retries.** A call error or invalid output is retried after `retry_backoff × 2^attempts` (5s, 10s, 20s, 40s). After 5 attempts the ticket becomes `failed`, with the reason in `last_error`. While the breaker is open, no attempt is used. Raw model output is never stored.

**Prompt injection.** Model output is treated as untrusted text. [llm.Parse](cmd/ingestor/internal/llm/llm.go) accepts only one JSON object with exactly `category`, `priority` and `summary`. The category and priority must come from the allowed sets in [llm.json](cmd/ingestor/internal/llm/llm.json). The summary must be one line of at most 300 characters. Anything else is rejected and retried, so an injection can at most pick a wrong label. With a real model, the ticket would also be passed as delimited data and the schema enforced as structured output.

## API

| Request                                            | Responses                                                     |
| -------------------------------------------------- | ------------------------------------------------------------- |
| `POST /tickets` `{"external_id","subject","body"}` | `202` new, `200` duplicate, `400` invalid, `429` rate limited |
| `GET /tickets/{id}` (numeric id)                   | `200`, `404`                                                  |
| `GET /healthz`, `GET /metrics`                     | `200`                                                         |

## Layout

`cmd/ingestor/internal/`:

| Package   | Role                                        |
| --------- | ------------------------------------------- |
| `api`     | HTTP and rate limiting                      |
| `store`   | Postgres and duplicate detection            |
| `sweeper` | Claims due tickets                          |
| `worker`  | Command channel and workers                 |
| `gateway` | Token bucket, breaker and output validation |
| `llm`     | Fake model and `Parse`                      |
| `ticket`  | The ticket model                            |
| `config`  | Configuration                               |
| `webhook` | Completion webhook                          |

Supporting packages: `bloom`, `tokenbucket`, `gateway/breaker`. Configuration lives in [config.example.yaml](cmd/ingestor/config.example.yaml); `POSTGRES_DSN` overrides the DSN.
