#!/bin/bash

# Генерация Go кода из Protobuf файлов

set -e

PROTO_DIR="ai-spec/api"
OUT_DIR="pkg/pb"

# Создаем директорию для вывода если не существует
mkdir -p ${OUT_DIR}

# Проверяем наличие protoc
if ! command -v protoc &> /dev/null; then
    echo "protoc not found. Please install protocol buffer compiler."
    echo "Visit: https://grpc.io/docs/protoc-installation/"
    exit 1
fi

# Проверяем наличие плагинов
if ! command -v protoc-gen-go &> /dev/null; then
    echo "protoc-gen-go not found. Installing..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "protoc-gen-go-grpc not found. Installing..."
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
fi

echo "Generating protobuf files..."

# Генерируем Go код
protoc \
    --go_out=${OUT_DIR} \
    --go_opt=paths=source_relative \
    --go-grpc_out=${OUT_DIR} \
    --go-grpc_opt=paths=source_relative \
    -I ${PROTO_DIR} \
    ${PROTO_DIR}/fanet.proto

echo "Protobuf generation completed!"
echo "Generated files in: ${OUT_DIR}"