.PHONY: test test-unit test-integration test-integration-internal test-crash docker-test

test:
	go test ./... -count=1

test-unit:
	go test ./internal/config ./internal/aggregation ./internal/membership -count=1

test-integration:
	go test ./tests/integration -run TestClusterConvergence -count=1

test-integration-internal:
	go test ./internal/gossip -run TestIntegrationGossipConvergence -count=1

test-crash:
	go test ./internal/gossip -run TestCrash -count=1

docker-test:
	docker run --rm -v "$(PWD)":/src -w /src golang:1.22 go test ./... -count=1
