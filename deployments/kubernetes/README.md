# FANET Backend Kubernetes Deployment

Полный набор Kubernetes манифестов для production deployment FANET Backend API.

## 📁 Структура

```
deployments/kubernetes/
├── README.md                    # Этот файл
├── kustomization.yaml          # Base configuration
├── namespace.yaml              # Namespace + ResourceQuota + LimitRange
├── configmap.yaml              # Конфигурация приложения
├── secret.yaml                 # Секреты (templates)
├── deployment.yaml             # API Deployment с production настройками
├── service.yaml                # Services (API, metrics, headless)
├── ingress.yaml                # Ingress с WebSocket + SSL
├── hpa.yaml                    # HorizontalPodAutoscaler + custom metrics
├── networkpolicy.yaml          # Сетевые политики безопасности
├── pdb.yaml                    # PodDisruptionBudget для HA
├── redis/                      # Redis Cluster
│   ├── statefulset.yaml        # Redis StatefulSet + init job
│   └── service.yaml            # Redis Services + config
├── monitoring/                 # Prometheus интеграция
│   └── servicemonitor.yaml     # ServiceMonitor + PodMonitor + Rules
└── overlays/                   # Environment-specific configs
    ├── dev/                    # Development overlay
    ├── staging/                # Staging overlay (TODO)
    └── production/             # Production overlay
```

## 🚀 Быстрый старт

### 1. Prerequisites

```bash
# Проверить наличие kubectl
kubectl version --client

# Проверить наличие kustomize
kustomize version

# Убедиться что подключены к правильному кластеру
kubectl config current-context
```

### 2. Development Deployment

```bash
# Применить dev конфигурацию
kubectl apply -k overlays/dev/

# Проверить статус
kubectl get pods -n fanet-dev
kubectl get services -n fanet-dev
kubectl get ingress -n fanet-dev

# Logs
kubectl logs -f deployment/fanet-api -n fanet-dev
```

### 3. Production Deployment

```bash
# ВАЖНО: Сначала обновить секреты в overlays/production/secrets-production.yaml

# Применить production конфигурацию
kubectl apply -k overlays/production/

# Проверить статус
kubectl get pods -n fanet
kubectl get hpa -n fanet
kubectl get servicemonitor -n fanet

# Мониторинг
kubectl top pods -n fanet
kubectl describe hpa fanet-api-hpa -n fanet
```

## 🔧 Конфигурация

### Environment Variables

Все настройки в `configmap.yaml` соответствуют `internal/config/config.go`:

| Variable | Default | Description |
|----------|---------|-------------|
| `ENVIRONMENT` | production | Режим работы |
| `SERVER_PORT` | 8090 | HTTP порт |
| `REDIS_URL` | from secret | Redis connection string |
| `MQTT_URL` | from secret | MQTT broker URL |
| `MYSQL_DSN` | from secret | MySQL connection (optional) |
| `AUTH_ENDPOINT` | from secret | Laravel API URL |
| `LOG_LEVEL` | info | Уровень логирования |

### Secrets

Обновите секреты для каждого environment:

```yaml
# overlays/production/secrets-production.yaml
stringData:
  REDIS_URL: "redis://prod-redis-cluster.flybeeper.com:6379"
  REDIS_PASSWORD: "SECURE_PRODUCTION_PASSWORD"
  MQTT_URL: "tcp://prod-mqtt.flybeeper.com:1883"
  MQTT_USERNAME: "fanet-api-prod"
  MQTT_PASSWORD: "SECURE_MQTT_PASSWORD"
  MYSQL_DSN: "user:pass@tcp(mysql.prod:3306)/fanet_prod?parseTime=true"
  AUTH_ENDPOINT: "https://api.flybeeper.com/api/v4/user"
```

## 📊 Мониторинг

### Prometheus Metrics

ServiceMonitor автоматически настроен для сбора метрик:

- **API metrics**: `http://fanet-api:9090/metrics`
- **Redis metrics**: через redis-exporter
- **Custom metrics**: WebSocket connections, MQTT rate, geo operations

### Grafana Dashboards

Используйте существующие дашборды из `deployments/monitoring/dashboards/`:

1. **API Performance** - HTTP метрики, latency, throughput
2. **WebSocket Realtime** - WebSocket connections, message rates
3. **MQTT Pipeline** - MQTT processing, queue sizes
4. **System Overview** - CPU, memory, network

### Алерты

Настроены алерты для:

- ❌ **API Down** - сервис недоступен > 1 мин
- ⚠️ **High Error Rate** - > 5% ошибок > 5 мин  
- ⚠️ **High Latency** - p95 > 500ms > 5 мин
- ⚠️ **WebSocket Overload** - > 8000 connections
- ⚠️ **MQTT Lag** - processing > 1s
- ❌ **Redis Down** - кластер недоступен
- ⚠️ **Redis Memory** - > 80% использования

## 🔐 Безопасность

### Network Policies

Настроены ограничения сетевого трафика:

- ✅ Ingress: только от nginx-ingress, Prometheus
- ✅ Egress: только к Redis, MQTT, MySQL, Laravel API
- ❌ Все остальные соединения запрещены

### Security Context

- ✅ `runAsNonRoot: true`
- ✅ `readOnlyRootFilesystem: true`
- ✅ `allowPrivilegeEscalation: false`
- ✅ `capabilities: drop ALL`

### RBAC

ServiceAccount с минимальными правами (только чтение собственных pods).

## 📈 Автомасштабирование

### HorizontalPodAutoscaler

Настроены два HPA:

1. **Основной** (CPU/Memory/WebSocket):
   - Min: 3 pods (prod) / 1 pod (dev)
   - Max: 20 pods (prod) / 3 pods (dev)
   - Targets: CPU 70%, Memory 80%, WebSocket < 1000/pod

2. **Кастомный** (FANET метрики):
   - Redis ops, Geo queries, Batch writer queue
   - Более консервативное масштабирование

### Поведение

- **Scale Up**: быстрое (100% за 30s)
- **Scale Down**: медленное (10% за 60s) для stability

## 🛠 Операции

### Обновление приложения

```bash
# Обновить образ
cd overlays/production/
kustomize edit set image flybeeper/fanet-api:v1.0.1

# Применить изменения
kubectl apply -k .

# Проверить rollout
kubectl rollout status deployment/fanet-api -n fanet
kubectl rollout history deployment/fanet-api -n fanet
```

### Откат

```bash
# Откатить к предыдущей версии
kubectl rollout undo deployment/fanet-api -n fanet

# Откатить к конкретной версии
kubectl rollout undo deployment/fanet-api --to-revision=2 -n fanet
```

### Масштабирование

```bash
# Временное ручное масштабирование
kubectl scale deployment fanet-api --replicas=10 -n fanet

# Обновить HPA
kubectl patch hpa fanet-api-hpa -n fanet -p '{"spec":{"minReplicas":5}}'
```

### Дебаггинг

```bash
# Логи
kubectl logs -f deployment/fanet-api -n fanet
kubectl logs --previous deployment/fanet-api -n fanet

# Exec в под
kubectl exec -it deployment/fanet-api -n fanet -- /bin/sh

# Проверка health
kubectl get pods -n fanet
kubectl describe pod <pod-name> -n fanet

# События
kubectl get events -n fanet --sort-by='.lastTimestamp'

# Metrics
kubectl top pods -n fanet
kubectl top nodes
```

### Backup / Restore

```bash
# Backup конфигурации
kubectl get all,configmap,secret,ingress,pdb,hpa,networkpolicy -n fanet -o yaml > fanet-backup.yaml

# Restore
kubectl apply -f fanet-backup.yaml
```

## 📋 Checklist перед Production

### Infrastructure

- [ ] Kubernetes кластер v1.25+
- [ ] nginx-ingress-controller установлен
- [ ] cert-manager настроен (Let's Encrypt)
- [ ] prometheus-operator установлен
- [ ] StorageClass `fast-ssd` доступен

### Configuration

- [ ] Обновлены production секреты
- [ ] Настроены DNS записи (api.flybeeper.com)
- [ ] Настроен external Redis кластер
- [ ] Настроен external MQTT broker
- [ ] Laravel API доступен для auth

### Monitoring

- [ ] Prometheus scraping работает
- [ ] Grafana дашборды импортированы
- [ ] AlertManager настроен
- [ ] Notification channels (Slack/Email) работают

### Security

- [ ] Network policies протестированы
- [ ] TLS сертификаты валидны
- [ ] RBAC права минимальны
- [ ] Секреты не commit в git

### Performance

- [ ] Resource limits настроены
- [ ] HPA протестирован
- [ ] Load testing выполнен
- [ ] Backup/restore процедуры проверены

## 🔗 Полезные ссылки

- [FANET API Документация](../../DEVELOPMENT.md)
- [Мониторинг Дашборды](../monitoring/dashboards/)
- [Архитектурный обзор](../../ai-spec/architecture/overview.md)
- [Frontend Integration](../../FRONTEND_INTEGRATION.md)

## 🐛 Troubleshooting

### Pod не запускается

```bash
kubectl describe pod <pod-name> -n fanet
kubectl logs <pod-name> -n fanet
```

Частые причины:
- Неправильные секреты
- Недоступен Redis/MQTT
- Insufficient resources
- Image pull errors

### WebSocket не работает

Проверить:
- Ingress annotations для WebSocket
- Network policy разрешения
- Application logs

### Медленная работа

Проверить:
- HPA metrics и scaling
- Resource utilization
- Redis cluster health
- MQTT queue размеры

### Ошибки аутентификации

Проверить:
- AUTH_ENDPOINT доступность
- Laravel API статус
- Network connectivity

---

🎯 **Production Ready**: Все манифесты настроены для production использования с высокой доступностью, безопасностью и мониторингом.