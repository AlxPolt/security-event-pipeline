.PHONY: build test security setup docs up down certs

setup:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install github.com/swaggo/swag/cmd/swag@latest

docs:
	swag init -g cmd/reader/main.go -o docs --parseDependency --parseInternal

build:
	go build -trimpath -ldflags="-w -s" ./cmd/...

test:
	go test -count=1 -race -coverprofile=coverage.out ./...

security:
	govulncheck ./...
	gosec -severity medium -confidence medium -quiet ./...

up:
	docker compose -f deployments/docker-compose.yaml up --build -d

down:
	docker compose -f deployments/docker-compose.yaml down -v

certs:
	bash scripts/certs.sh
