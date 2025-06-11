# FANET Backend Deployment Guide

Полное руководство по развертыванию FANET Backend API в различных окружениях.

## 📋 Обзор

FANET Backend - высокопроизводительный Go сервис для real-time отслеживания FANET устройств с поддержкой:
- ⚡ **10k+ concurrent WebSocket connections**
- 🚀 **50k+ MQTT msg/sec processing**
- 🌍 **Геопространственные запросы < 10ms**
- 🔒 **Production-ready безопасность**
- 📊 **Comprehensive мониторинг**

## 🏗️ Архитектура Deployment

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Load Balancer │────│ FANET Backend API │────│ Redis Cluster   │
│   (nginx)       │    │ (Kubernetes)     │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                      │                       │
         │                      │                       │
    ┌─────────┐          ┌────────────┐         ┌──────────────┐
    │ SSL/TLS │          │ MQTT       │         │ MySQL        │
    │ Certs   │          │ Broker     │         │ (Backup)     │
    └─────────┘          └────────────┘         └──────────────┘
```

## 🚀 Способы развертывания

### 0. 🚀 Быстрое развертывание (РЕКОМЕНДУЕТСЯ)

**Для развертывания без установки Go и protoc:**

```bash
# Клонирование репозитория
git clone git@github.com:Genajoin/flybeeper.com.map-fanet-backend.git
cd flybeeper.com.map-fanet-backend

# Одна команда - полное развертывание
./deploy-simple.sh

# Проверка
curl http://localhost:8090/health
```

**Что включает скрипт:**
- ✅ Автоматическая сборка Docker образа с protobuf генерацией
- ✅ Запуск всех необходимых сервисов (Redis, MQTT, MySQL)
- ✅ Настройка сети между контейнерами
- ✅ Проверка здоровья всех сервисов
- ✅ Готовые команды для тестирования

### 1. 📦 Docker Compose (Разработка)

Быстрый старт для локальной разработки:

```bash
# Клонирование и подготовка
git clone git@github.com:Genajoin/flybeeper.com.map-fanet-backend.git
cd flybeeper.com.map-fanet-backend

# ВАРИАНТ 1: С предустановленным Go и protoc
make deps && make proto  # Требует: go, protoc
make dev-env && make dev

# ВАРИАНТ 2: Только Docker (без Go/protoc)
make dev-env  # Запуск Redis, MQTT, MySQL
make docker-build  # Автоматически генерирует protobuf
docker run -p 8090:8090 --network host flybeeper/fanet-api:latest

# Проверка
curl http://localhost:8090/health
```

**Включает:**
- FANET API (localhost:8090)
- Redis (localhost:6379)
- MQTT Mosquitto (localhost:1883)
- MySQL (localhost:3306)
- Redis Commander (localhost:8081)
- Adminer (localhost:8082)

### 2. ☸️ Kubernetes (Production)

Enterprise-grade deployment с автомасштабированием:

```bash
# Development
kubectl apply -k deployments/kubernetes/overlays/dev/

# Production
kubectl apply -k deployments/kubernetes/overlays/production/
```

**Включает:**
- 3+ replicas с auto-scaling
- Rolling updates zero-downtime
- Health checks и probes
- Network policies безопасность
- Prometheus мониторинг
- Persistent Redis cluster

### 3. 🐳 Standalone Docker

Простое развертывание в single container:

```bash
# Клонирование репозитория
git clone git@github.com:Genajoin/flybeeper.com.map-fanet-backend.git
cd flybeeper.com.map-fanet-backend

# Сборка образа (включает автоматическую генерацию protobuf)
make docker-build

# Запуск с external services
docker run -d \
  -p 8090:8090 \
  -e REDIS_URL="redis://your-redis:6379" \
  -e MQTT_URL="tcp://your-mqtt:1883" \
  -e AUTH_ENDPOINT="https://api.flybeeper.com/api/v4/user" \
  flybeeper/fanet-api:latest
```

**⚠️ Важно**: Dockerfile теперь автоматически генерирует protobuf файлы во время сборки. Больше не требуется предустановка Go или protoc на хост-системе.

## 🔧 Конфигурация

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SERVER_PORT` | ❌ | 8090 | HTTP server port |
| `REDIS_URL` | ✅ | - | Redis connection string |
| `MQTT_URL` | ✅ | - | MQTT broker URL |
| `MYSQL_DSN` | ❌ | - | MySQL connection (backup) |
| `AUTH_ENDPOINT` | ✅ | - | Laravel auth API |
| `LOG_LEVEL` | ❌ | info | debug/info/warn/error |
| `ENVIRONMENT` | ❌ | production | Environment mode |

### 📝 Примеры конфигурации

#### Development
```env
ENVIRONMENT=development
LOG_LEVEL=debug
REDIS_URL=redis://localhost:6379
MQTT_URL=tcp://localhost:1883
AUTH_ENDPOINT=https://dev-api.flybeeper.com/api/v4/user
```

#### Production
```env
ENVIRONMENT=production
LOG_LEVEL=info
REDIS_URL=redis://prod-redis-cluster.flybeeper.com:6379
MQTT_URL=tcp://prod-mqtt.flybeeper.com:1883
AUTH_ENDPOINT=https://api.flybeeper.com/api/v4/user
MYSQL_DSN=user:pass@tcp(mysql.prod:3306)/fanet?parseTime=true
```

## 🏢 Production Deployment

### Prerequisites

- **Kubernetes** cluster v1.25+
- **nginx-ingress-controller**
- **cert-manager** (Let's Encrypt)
- **prometheus-operator**
- **External services**: Redis, MQTT, MySQL

### Step-by-Step Production Setup

#### 1. 🔐 Подготовка секретов

```bash
# Редактировать production secrets
vim deployments/kubernetes/overlays/production/secrets-production.yaml
```

```yaml
stringData:
  REDIS_URL: "redis://prod-redis.flybeeper.com:6379"
  REDIS_PASSWORD: "SECURE_PASSWORD"
  MQTT_URL: "tcp://prod-mqtt.flybeeper.com:1883"
  MQTT_USERNAME: "fanet-api-prod"
  MQTT_PASSWORD: "SECURE_MQTT_PASSWORD"
  MYSQL_DSN: "user:pass@tcp(mysql.prod:3306)/fanet?parseTime=true"
  AUTH_ENDPOINT: "https://api.flybeeper.com/api/v4/user"
```

#### 2. 🌐 DNS настройка

```bash
# A Record
api.flybeeper.com → Load Balancer IP

# CNAME (alternative)
api.flybeeper.com → k8s-cluster.flybeeper.com
```

#### 3. 🚀 Deployment

```bash
# Применить манифесты
kubectl apply -k deployments/kubernetes/overlays/production/

# Проверить статус
kubectl get pods -n fanet
kubectl get ingress -n fanet
kubectl get hpa -n fanet

# Мониторинг
kubectl logs -f deployment/fanet-api -n fanet
```

#### 4. ✅ Верификация

```bash
# Health check
curl https://api.flybeeper.com/health

# API тест
curl "https://api.flybeeper.com/api/v1/snapshot?lat=46.0&lon=8.0&radius=50"

# WebSocket тест
wscat -c "wss://api.flybeeper.com/ws/v1/updates?lat=46&lon=8&radius=50"

# Metrics
curl https://api.flybeeper.com/metrics
```

## 📊 Мониторинг и Логирование

### Prometheus Metrics

FANET Backend экспортирует 15+ групп метрик:

```
# Core API metrics
http_requests_total{method="GET",endpoint="/api/v1/snapshot"}
http_request_duration_seconds{quantile="0.95"}

# WebSocket metrics
websocket_connections_active
websocket_messages_sent_total

# MQTT metrics
mqtt_messages_received_total{type="air_tracking"}
mqtt_parsing_duration_seconds

# Redis metrics
redis_operations_total{operation="georadius"}
redis_connection_pool_active

# Business metrics
pilots_active_total
thermals_active_total
geohash_regions_subscribed
```

### Grafana Dashboards

Готовые дашборды в `deployments/monitoring/dashboards/`:

1. **📈 API Performance** - HTTP latency, throughput, errors
2. **🌐 WebSocket Realtime** - Connections, messages, bandwidth
3. **📡 MQTT Pipeline** - Processing rate, parsing performance
4. **🖥️ System Overview** - CPU, memory, network utilization

### Алерты

Настроенные алерты через AlertManager:

- ❌ **API Down** - сервис недоступен > 1 мин
- ⚠️ **High Error Rate** - > 5% ошибок > 5 мин
- ⚠️ **High Latency** - p95 > 500ms > 5 мин
- ⚠️ **WebSocket Overload** - > 8000 connections
- ⚠️ **MQTT Processing Lag** - > 1s delay

## 🔒 Безопасность

### Network Security

```yaml
# NetworkPolicy - только необходимые соединения
ingress:
  - from: nginx-ingress
  - from: prometheus
egress:
  - to: redis-cluster
  - to: mqtt-broker
  - to: laravel-api
```

### Container Security

```yaml
securityContext:
  runAsNonRoot: true
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
```

### Аутентификация

- **Laravel Passport** интеграция
- **Bearer token** validation
- **5 минут** кеширование токенов
- **Rate limiting** 100 req/sec

## 📈 Автомасштабирование

### HorizontalPodAutoscaler

```yaml
metrics:
  - CPU: 70% target
  - Memory: 80% target
  - WebSocket connections: 1000/pod
  - HTTP requests: 500/sec/pod
  - MQTT messages: 1000/sec/pod

behavior:
  scaleUp: быстрое (100% за 30s)
  scaleDown: медленное (10% за 60s)
```

### Производительность

- **Min replicas**: 3 (production) / 1 (dev)
- **Max replicas**: 20 (production) / 3 (dev)
- **Target**: < 10ms latency @ 10k connections

## 🛠️ Операции

### Обновление версии

```bash
# Обновить образ
cd deployments/kubernetes/overlays/production/
kustomize edit set image flybeeper/fanet-api:v1.0.1

# Rolling update
kubectl apply -k .
kubectl rollout status deployment/fanet-api -n fanet
```

### Откат версии

```bash
# К предыдущей версии
kubectl rollout undo deployment/fanet-api -n fanet

# К конкретной версии
kubectl rollout undo deployment/fanet-api --to-revision=2 -n fanet
```

### Горизонтальное масштабирование

```bash
# Временное ручное масштабирование
kubectl scale deployment fanet-api --replicas=10 -n fanet

# Обновление HPA
kubectl patch hpa fanet-api-hpa -n fanet \
  -p '{"spec":{"minReplicas":5,"maxReplicas":30}}'
```

### Backup и восстановление

```bash
# Backup конфигурации
kubectl get all,cm,secret,ingress,pdb,hpa,netpol -n fanet -o yaml > backup.yaml

# Backup данных (Redis)
kubectl exec redis-0 -n fanet -- redis-cli --rdb /backup/dump.rdb

# Restore
kubectl apply -f backup.yaml
```

## 🐛 Troubleshooting

### Частые проблемы

#### Pod не запускается

```bash
kubectl describe pod <pod-name> -n fanet
kubectl logs <pod-name> -n fanet

# Проверить:
# - Секреты корректны
# - Redis/MQTT доступны
# - Resource limits
# - Image pull policy
```

#### WebSocket не работает

```bash
# Проверить ingress
kubectl describe ingress fanet-api -n fanet

# Проверить nginx logs
kubectl logs -n nginx-ingress nginx-ingress-controller-xxx

# Тест соединения
curl -H "Upgrade: websocket" -H "Connection: Upgrade" \
  https://api.flybeeper.com/ws/v1/updates
```

#### Высокая латентность

```bash
# Проверить метрики
kubectl top pods -n fanet
kubectl describe hpa fanet-api-hpa -n fanet

# Проверить Redis
kubectl exec redis-0 -n fanet -- redis-cli info replication

# Профилирование
kubectl port-forward deployment/fanet-api 6060:6060 -n fanet
go tool pprof http://localhost:6060/debug/pprof/profile
```

#### Ошибки аутентификации

```bash
# Проверить Laravel API
curl -H "Authorization: Bearer TOKEN" \
  https://api.flybeeper.com/api/v4/user

# Проверить DNS resolution
kubectl exec deployment/fanet-api -n fanet -- nslookup api.flybeeper.com

# Проверить network policy
kubectl describe networkpolicy fanet-api -n fanet
```

## 📊 Производительные метрики

### Benchmark результаты

```
FANET парсинг:     377ns/op (2.6M ops/sec)
Geohash операции:  61ns/op (16.3M ops/sec)
Redis GeoRadius:   2.1ms (реальные данные)
WebSocket бродкаст: 0.5ms (1000 клиентов)
Memory per conn:   ~100KB (оптимизировано)
```

### Capacity Planning

| Метрика | 1 Pod | 3 Pods | 10 Pods |
|---------|-------|--------|---------|
| WebSocket connections | 1000 | 3000 | 10000 |
| HTTP requests/sec | 500 | 1500 | 5000 |
| MQTT messages/sec | 1000 | 3000 | 10000 |
| Memory usage | 256MB | 768MB | 2.5GB |
| CPU usage | 250m | 750m | 2.5 cores |

## 🔗 Полезные ссылки

- 📖 [Development Guide](DEVELOPMENT.md)
- 🏗️ [Architecture Overview](ai-spec/architecture/overview.md)
- 🔐 [Auth Integration](ai-spec/auth-integration.md)
- 🌐 [Frontend Integration](FRONTEND_INTEGRATION.md)
- ☸️ [Kubernetes Manifests](deployments/kubernetes/README.md)
- 📊 [Monitoring Setup](deployments/monitoring/README.md)

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/flybeeper/fanet-backend/issues)
- **Documentation**: Обновляется автоматически при изменениях
- **Monitoring**: Grafana дашборды доступны в production

---

🎯 **Production Ready**: FANET Backend готов к production deployment с enterprise-grade возможностями масштабирования, безопасности и мониторинга.