BIN      := plexterboxd
WEB_DIR  := web
DIST_DIR := cmd/server/dist

.PHONY: build build-windows ui docker docker-up docker-down dev clean

## build: compile frontend then Go binary (current OS)
build: ui
	go build -o $(BIN) ./cmd/server/

## build-windows: cross-compile a Windows .exe
build-windows: ui
	GOOS=windows GOARCH=amd64 go build -o $(BIN).exe ./cmd/server/

## ui: build the React frontend into cmd/server/dist/
ui:
	cd $(WEB_DIR) && npm install && npm run build

## docker: build the Docker image
docker:
	docker build -t plexterboxd .

## docker-up: start with docker compose (detached)
docker-up:
	docker compose up -d --build

## docker-down: stop and remove the container
docker-down:
	docker compose down

## dev: run backend + frontend dev servers in parallel (requires tmux or terminal with &)
dev:
	@echo "Start the Go server:  go run ./cmd/server/"
	@echo "Start the Vite dev:   cd web && npm run dev"

## clean: remove binary and frontend build artefacts
clean:
	rm -rf $(BIN) $(BIN).exe $(DIST_DIR)
