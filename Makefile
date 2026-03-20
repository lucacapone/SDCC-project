.PHONY: test test-unit test-integration test-integration-internal test-crash test-crash-restart test-m10 docker-test

test:
	go test ./... -count=1

test-unit:
	go test ./tests/config ./tests/aggregation ./tests/membership -count=1

test-integration:
	go test ./tests/integration -run TestClusterConvergence -count=1

test-integration-internal:
	go test ./tests/gossip -run TestIntegrationGossipConvergence -count=1

test-crash:
	go test ./tests/gossip -run TestCrash -count=1

test-crash-restart:
	go test ./tests/integration -run TestNodeCrashAndRestart -count=1

test-m10: test-crash-restart

docker-test:
	docker run --rm -v "$(PWD)":/src -w /src golang:1.22 go test ./... -count=1
