#!/bin/bash

# FANET Backend - Простое развертывание без предустановок
# Требует только Docker

set -e  # Выход при любой ошибке

echo "🚀 FANET Backend - Простое развертывание"
echo "==============================================="

# Проверка Docker
if ! command -v docker &> /dev/null; then
    echo "❌ ERROR: Docker не установлен!"
    echo "Установите Docker: https://docs.docker.com/get-docker/"
    exit 1
fi

if ! docker info &> /dev/null; then
    echo "❌ ERROR: Docker daemon не запущен!"
    echo "Запустите Docker daemon и попробуйте снова"
    exit 1
fi

echo "✅ Docker доступен"

# Создание сети для контейнеров
echo "🔧 Создание Docker сети..."
docker network create fanet-network 2>/dev/null || echo "Сеть fanet-network уже существует"

# Запуск Redis
echo "🔴 Запуск Redis..."
docker run -d \
  --name fanet-redis \
  --network fanet-network \
  -p 6379:6379 \
  redis:7-alpine \
  redis-server --appendonly yes || echo "Redis уже запущен"

# Запуск MQTT Broker
echo "📡 Запуск MQTT Broker..."
docker run -d \
  --name fanet-mqtt \
  --network fanet-network \
  -p 1883:1883 \
  -p 9001:9001 \
  eclipse-mosquitto:2.0 || echo "MQTT уже запущен"

# Запуск MySQL (опционально)
echo "🗄️  Запуск MySQL..."
docker run -d \
  --name fanet-mysql \
  --network fanet-network \
  -p 3306:3306 \
  -e MYSQL_ROOT_PASSWORD=password \
  -e MYSQL_DATABASE=fanet \
  mysql:8.0 || echo "MySQL уже запущен"

# Ожидание готовности сервисов
echo "⏳ Ожидание готовности сервисов..."
sleep 10

# Сборка FANET API образа
echo "🔨 Сборка FANET API..."
docker build -t flybeeper/fanet-api:latest .

# Запуск FANET API
echo "🚀 Запуск FANET API..."
docker run -d \
  --name fanet-api \
  --network fanet-network \
  -p 8090:8090 \
  -e REDIS_URL="redis://fanet-redis:6379" \
  -e MQTT_URL="tcp://fanet-mqtt:1883" \
  -e MYSQL_DSN="root:password@tcp(fanet-mysql:3306)/fanet?parseTime=true" \
  -e AUTH_ENDPOINT="https://api.flybeeper.com/api/v4/user" \
  -e LOG_LEVEL="info" \
  -e ENVIRONMENT="development" \
  flybeeper/fanet-api:latest || {
    echo "❌ Ошибка запуска API. Проверяем логи..."
    docker logs fanet-api
    exit 1
  }

# Проверка здоровья
echo "🔍 Проверка здоровья сервисов..."
sleep 5

# Проверка API
echo "Testing API health..."
if curl -s http://localhost:8090/health > /dev/null; then
    echo "✅ FANET API работает!"
else
    echo "❌ FANET API недоступен"
    echo "Логи API:"
    docker logs fanet-api --tail 20
fi

# Проверка Redis
echo "Testing Redis..."
if docker exec fanet-redis redis-cli ping | grep -q PONG; then
    echo "✅ Redis работает!"
else
    echo "❌ Redis недоступен"
fi

# Проверка MQTT
echo "Testing MQTT..."
if docker exec fanet-mqtt mosquitto_pub -h localhost -t test -m "test" -d; then
    echo "✅ MQTT работает!"
else
    echo "❌ MQTT недоступен"
fi

echo ""
echo "🎉 Развертывание завершено!"
echo "==============================================="
echo "📊 Доступные сервисы:"
echo "  • FANET API:        http://localhost:8090"
echo "  • API Health:       http://localhost:8090/health"
echo "  • API Metrics:      http://localhost:8090/metrics"
echo "  • Redis:            localhost:6379"
echo "  • MQTT:             localhost:1883"
echo "  • MySQL:            localhost:3306"
echo ""
echo "🧪 Тестовые команды:"
echo "  • API Test:         curl http://localhost:8090/health"
echo "  • Snapshot Test:    curl 'http://localhost:8090/api/v1/snapshot?lat=46.0&lon=8.0&radius=50'"
echo "  • WebSocket Test:   wscat -c 'ws://localhost:8090/ws/v1/updates?lat=46&lon=8&radius=50'"
echo "  • MQTT Test:        docker exec fanet-mqtt mosquitto_pub -h localhost -t 'fb/b/test/f/1' -m 'test'"
echo ""
echo "📋 Управление:"
echo "  • Логи API:         docker logs -f fanet-api"
echo "  • Остановка:        docker stop fanet-api fanet-redis fanet-mqtt fanet-mysql"
echo "  • Удаление:         docker rm fanet-api fanet-redis fanet-mqtt fanet-mysql"
echo "  • Очистка сети:     docker network rm fanet-network"
echo ""
echo "✨ FANET Backend готов к использованию!"