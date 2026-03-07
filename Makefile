.PHONY: dev build test demo reset vendor-stub app

build:
	go build -o bin/app ./cmd/app
	go build -o bin/vendorstub ./cmd/vendorstub

app: build
	./bin/app

vendor-stub: build
	./bin/vendorstub

dev:
	@echo "Starting vendor stub..."
	@go run ./cmd/vendorstub & echo $$! > .vendorstub.pid
	@sleep 1
	@echo "Starting app..."
	@go run ./cmd/app
	@kill $$(cat .vendorstub.pid) 2>/dev/null || true
	@rm -f .vendorstub.pid

test:
	go test ./... -v -count=1

test-e2e:
	cd tests/e2e && npx playwright test

reset:
	rm -rf data/sqlite/mcd.db data/images/* reports/settlement/*
	@echo "Database and data reset."

clean:
	rm -rf bin/ data/sqlite/ data/images/ reports/settlement/ .vendorstub.pid
