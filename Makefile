.PHONY: dev build test demo reset vendor-stub app docker-dev docker-build build-linux video demo-video demo-final clean

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
	cd tests/e2e && npx playwright test \
		api-endpoints.spec.ts \
		audit-log.spec.ts \
		business-rules.spec.ts \
		deposit-submission.spec.ts \
		empty-states.spec.ts \
		happy-path.spec.ts \
		keyboard.spec.ts \
		ledger.spec.ts \
		navigation.spec.ts \
		operator-review.spec.ts \
		returns.spec.ts \
		settlement.spec.ts \
		transfer-detail.spec.ts \
		vendor-scenarios.spec.ts \
		visual-regression.spec.ts

reset:
	rm -rf data/sqlite/mcd.db data/images/* reports/settlement/*
	@echo "Database and data reset."

docker-build: build-linux
	docker compose build

docker-dev: build-linux
	docker compose up --build

build-linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o bin/app ./cmd/app
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/vendorstub ./cmd/vendorstub

demo:
	@echo "Starting vendor stub..."
	@go run ./cmd/vendorstub & echo $$! > .vendorstub.pid
	@sleep 1
	@echo "Starting app..."
	@go run ./cmd/app & echo $$! >> .vendorstub.pid
	@sleep 2
	@echo "Running all-scenarios demo..."
	@bash scripts/demo_all_scenarios.sh || true
	@echo "Shutting down..."
	@cat .vendorstub.pid | xargs kill 2>/dev/null || true
	@rm -f .vendorstub.pid

DEMO_WEBM := tests/e2e/test-results/demo-short-Professional-Demo-Four-Core-Workflows-chromium/video.webm

## demo-video: 3-minute professional demo (4 workflows, 4K) — recommended
demo-video:
	@echo "Generating short professional demo video (4K)..."
	@cd tests/e2e && npx playwright test demo-short.spec.ts
	@echo ""
	@echo "Video: $(DEMO_WEBM)"
	@mpv --no-resume-playback "$(DEMO_WEBM)" 2>/dev/null || \
	  xdg-open "$(DEMO_WEBM)" 2>/dev/null || \
	  echo "(open the file above manually)"

## demo-final: record demo + assemble voiceover + encode H.264 via NVENC → demo-final.mp4
demo-final:
	@echo "Recording demo..."
	@cd tests/e2e && npx playwright test demo-short.spec.ts
	@echo "Assembling voiceover + encoding H.264..."
	@cd tests/e2e && bash assemble-demo-video.sh
	@echo ""
	@echo "Final video: tests/e2e/demo-final.mp4"
	@mpv --no-resume-playback tests/e2e/demo-final.mp4 2>/dev/null || \
	  xdg-open tests/e2e/demo-final.mp4 2>/dev/null || \
	  echo "(open the file above manually)"

## video: full 10-minute walkthrough with architecture diagrams
video:
	@echo "Restarting app with latest code..."
	@pkill -f "go-build.*cmd/app" 2>/dev/null || true
	@pkill -f "go run.*cmd/app" 2>/dev/null || true
	@sleep 1
	@set -a && . ./.env && set +a && go run ./cmd/app & echo $$! > .app.pid
	@echo "Waiting for app..."
	@for i in $$(seq 1 20); do curl -sf http://localhost:8080/api/v1/deposits > /dev/null 2>&1 && break || sleep 1; done
	@echo "Clearing Playwright cache..."
	@rm -rf /tmp/playwright-transform-cache-$$(id -u)/
	@echo "Generating video tour..."
	@cd tests/e2e && npx playwright test video-tour.spec.ts
	@echo "Playing video..."
	@mpv --no-resume-playback tests/e2e/test-results/video-tour-Video-Tour-Full-Application-Walkthrough-chromium/video.webm

clean:
	rm -rf bin/ data/sqlite/ data/images/ reports/settlement/ .vendorstub.pid
