.PHONY: test test-unit test-integration test-crash docker-test

test:
	go test ./... -count=1

test-unit:
	go test ./internal/config ./internal/aggregation ./internal/membership -count=1

test-integration:
	go test ./internal/gossip -run TestIntegration -count=1

test-crash:
	go test ./internal/gossip -run TestCrash -count=1

docker-test:
	docker run --rm -v "$(PWD)":/src -w /src golang:1.22 go test ./... -count=1
