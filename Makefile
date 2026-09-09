.PHONY: test test-unit test-integration test-integration-internal test-crash test-crash-restart test-crash-restart-internal test-m10 test-traffic-control test-traffic-control-integration docker-test

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

# Variante rapida/deterministica del vecchio scenario M10 in-memory.
test-crash-restart-internal:
	go test ./tests/integration -run TestNodeCrashAndRestartInMemory -count=1

test-m10: test-crash-restart

# Verifica validazione, riepiloghi, mediana e apply/clear con tc simulato.
test-traffic-control:
	bash tests/traffic-control/traffic_control_test.sh

# Suite lenta Linux-only che richiede Docker e la capability NET_ADMIN.
test-traffic-control-integration:
	bash tests/integration/traffic_control_compose_test.sh

docker-test:
	docker run --rm -v "$(PWD)":/src -w /src golang:1.22 go test ./... -count=1
