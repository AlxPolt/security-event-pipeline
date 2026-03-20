#!/bin/bash

# Your module name
MODULE_NAME="event_handling"

echo "Starting project initialization..."

# 1. Creating folder structure
SERVICES=("daemon" "writer" "reader" "client")

for svc in "${SERVICES[@]}"; do
    echo " Creating folders for: $svc"
    mkdir -p cmd/$svc
    mkdir -p internal/$svc/domain
    mkdir -p internal/$svc/ports
    mkdir -p internal/$svc/adapters
done

# 2. Creating infrastructure folders
mkdir -p deployments pkg/logger pkg/security scripts

# 3. Creating entry point files
for svc in "${SERVICES[@]}"; do
    touch cmd/$svc/main.go
    echo "package main" > cmd/$svc/main.go
done

# 4. Creating base files in pkg
touch pkg/logger/logger.go
touch pkg/security/tls.go
touch pkg/security/validator.go

# 5. Initializing Go module
if [ ! -f "go.mod" ]; then
    go mod init $MODULE_NAME
    echo "Go module initialized: $MODULE_NAME"
fi

# 6. Creating auxiliary files
touch Makefile .env.example .gitignore README.md
touch deployments/docker-compose.yaml

# Setting permissions
chmod +x Makefile

echo "Project structure is ready!"
echo "Run 'go mod tidy' to fetch dependencies."
