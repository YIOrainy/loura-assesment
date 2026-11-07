BIN_INGESTOR = ingestor

DIR_INGESTOR = ./cmd/ingestor

TEST_POSTGRES_DSN ?= postgres://postgres:postgres@localhost:5433/tickets?sslmode=disable

all: $(BIN_INGESTOR)

$(BIN_INGESTOR):
	go build -o $(BIN_INGESTOR) $(DIR_INGESTOR)

up:
	docker compose up --build -d

down:
	docker compose down

db:
	docker compose up -d postgres

run:
	go run $(DIR_INGESTOR)

samples:
	jq -c '.[]' samples/tickets.json | while read -r t; do curl -s -XPOST localhost:8080/tickets -d "$$t"; echo; done

test:
	TEST_POSTGRES_DSN="$(TEST_POSTGRES_DSN)" go test -race ./...

clean:
	rm -f $(BIN_INGESTOR)

.PHONY: all up down db run samples test clean $(BIN_INGESTOR)
