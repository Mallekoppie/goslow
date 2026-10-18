.PHONY: test test-unit test-race vet build-examples

# Default target: run the unit test suite.
test: test-unit

# Run every unit test in the root module (integration-tagged tests excluded).
test-unit:
	go test ./...

# Run the unit test suite under the race detector.
test-race:
	go test -race ./...

# Static checks used by CI.
vet:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt required:"; echo "$$out"; exit 1; fi
	go vet ./...

# Build every example module, which uses the local platform via replace directives.
build-examples:
	@out=$$(mktemp -d); trap 'rm -rf "$$out"' EXIT; \
	for dir in $$(find example -name go.mod -exec dirname {} \;); do \
		echo "building $$dir"; \
		(cd $$dir && go build -o "$$out/" ./...) || exit 1; \
	done
