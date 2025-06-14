# FANET Backend Operations Guide

Руководство по эксплуатации и поддержке FANET Backend API в production среде.

## 📋 Обзор операций

Этот документ покрывает daily operations, мониторинг, troubleshooting и maintenance процедуры для FANET Backend.

## 🔍 Мониторинг

### Health Checks

#### Базовые проверки

```bash
# API Health
curl https://api.flybeeper.com/health
# Ожидаемый ответ: {"status":"ok","timestamp":"2024-01-01T12:00:00Z"}

# Ready check
curl https://api.flybeeper.com/ready
# Проверяет Redis, MQTT подключения

# Metrics endpoint
curl https://api.flybeeper.com/metrics
# Prometheus metrics
```

#### Kubernetes health

```bash
# Pods status
kubectl get pods -n fanet
kubectl get pods -n fanet-dev

# Services и endpoints
kubectl get svc,ep -n fanet

# Ingress status
kubectl get ingress -n fanet

# HPA status
kubectl get hpa -n fanet
kubectl describe hpa fanet-api-hpa -n fanet
```

### Key Performance Indicators (KPIs)

#### 🎯 SLA Метрики

| Метрика                   | Target  | Critical    |
| ------------------------- | ------- | ----------- |
| **Uptime**                | > 99.9% | < 99.0%     |
| **Response time (p95)**   | < 50ms  | > 500ms     |
| **Error rate**            | < 1%    | > 5%        |
| **WebSocket connections** | Stable  | Drops > 10% |
| **MQTT processing lag**   | < 100ms | > 1s        |

#### 📊 Business Метрики

```bash
# Активные пилоты
curl -s "https://fanet-api.flybeeper.com/api/v1/pilots?lat=46&lon=8&radius=200" | jq '.pilots | length'

# WebSocket подключения
curl -s https://api.flybeeper.com/metrics | grep websocket_connections_active

# MQTT throughput
curl -s https://api.flybeeper.com/metrics | grep mqtt_messages_received_total
```

### Алерты

#### Critical Alerts (PagerDuty)

- ❌ **API Down** - все pods недоступны > 1 мин
- ❌ **Redis Cluster Down** - потеря данных
- ❌ **MQTT Broker Disconnected** - нет новых данных > 2 мин
- ❌ **High Error Rate** - > 10% ошибок > 2 мин

#### Warning Alerts (Slack)

- ⚠️ **High Latency** - p95 > 200ms > 5 мин
- ⚠️ **Memory Usage High** - > 80% > 10 мин
- ⚠️ **WebSocket Overload** - > 8000 connections
- ⚠️ **HPA Scaling** - частое масштабирование

#### Info Alerts (Email)

- ℹ️ **New Deployment** - successful rollout
- ℹ️ **Scaling Event** - HPA изменение replicas
- ℹ️ **Certificate Renewal** - SSL certs updated

## 🚨 Incident Response

### Severity Levels

#### P0 - Critical (15 мин response)
- Полная недоступность API
- Потеря данных
- Безопасность breach

#### P1 - High (1 час response)
- Partial outage
- Высокая latency влияющая на UX
- Degraded performance

#### P2 - Medium (4 часа response)
- Non-critical bugs
- Performance issues
- Minor feature degradation

#### P3 - Low (Next business day)
- Documentation issues
- Minor enhancements
- Non-urgent improvements

### Runbooks

#### API Down (P0)

```bash
# 1. Проверить pods status
kubectl get pods -n fanet
kubectl describe pod <failing-pod> -n fanet

# 2. Проверить logs
kubectl logs -f deployment/fanet-api -n fanet --tail=100

# 3. Проверить dependencies
kubectl exec deployment/fanet-api -n fanet -- nc -zv redis-cluster 6379
kubectl exec deployment/fanet-api -n fanet -- nc -zv mqtt-broker 1883

# 4. Force restart если необходимо
kubectl rollout restart deployment/fanet-api -n fanet

# 5. Escalate если не помогает
# - Проверить infrastructure (K8s cluster)
# - Проверить external dependencies
```

#### High Latency (P1)

```bash
# 1. Проверить current performance
kubectl top pods -n fanet
kubectl describe hpa fanet-api-hpa -n fanet

# 2. Анализ metrics
curl -s https://api.flybeeper.com/metrics | grep http_request_duration

# 3. Проверить Redis performance
kubectl exec redis-0 -n fanet -- redis-cli info stats
kubectl exec redis-0 -n fanet -- redis-cli slowlog get 10

# 4. Scaling если необходимо
kubectl scale deployment fanet-api --replicas=10 -n fanet

# 5. Performance profiling
kubectl port-forward deployment/fanet-api 6060:6060 -n fanet
go tool pprof http://localhost:6060/debug/pprof/profile
```

#### MQTT Disconnected (P0)

```bash
# 1. Проверить MQTT broker
telnet prod-mqtt.flybeeper.com 1883

# 2. Проверить application logs
kubectl logs -f deployment/fanet-api -n fanet | grep mqtt

# 3. Restart application для reconnection
kubectl rollout restart deployment/fanet-api -n fanet

# 4. Проверить network connectivity
kubectl exec deployment/fanet-api -n fanet -- ping prod-mqtt.flybeeper.com

# 5. Contact MQTT broker admin если broker down
```

## 🛠️ Maintenance

### Regular Tasks

#### Daily (Automated)

- ✅ Health checks monitoring
- ✅ Backup verification
- ✅ Certificate expiry checks
- ✅ Log rotation
- ✅ Metrics collection

#### Weekly (Manual)

```bash
# 1. Проверить resource utilization trends
kubectl top nodes
kubectl top pods -n fanet --sort-by=memory

# 2. Review logs для patterns
kubectl logs deployment/fanet-api -n fanet --since=168h | grep ERROR

# 3. Cleanup old resources если необходимо
kubectl delete pod --field-selector=status.phase==Succeeded -n fanet

# 4. Review HPA performance
kubectl describe hpa fanet-api-hpa -n fanet
```

#### Monthly (Planned)

- 📊 Performance review
- 🔐 Security audit
- 📋 Capacity planning
- 📚 Documentation updates
- 🧪 Disaster recovery testing

### Updates и Deployments

#### Application Updates

```bash
# 1. Development testing
kubectl apply -k deployments/kubernetes/overlays/dev/

# 2. Testing validation
./scripts/smoke-tests.sh dev-api.flybeeper.com

# 3. Production deployment
kubectl apply -k deployments/kubernetes/overlays/production/

# 4. Rollout monitoring
kubectl rollout status deployment/fanet-api -n fanet

# 5. Post-deployment validation
./scripts/smoke-tests.sh api.flybeeper.com
```

#### Rollback Process

```bash
# Emergency rollback
kubectl rollout undo deployment/fanet-api -n fanet

# Rollback к specific revision
kubectl rollout history deployment/fanet-api -n fanet
kubectl rollout undo deployment/fanet-api --to-revision=2 -n fanet
```

### Backup и Recovery

#### Redis Data Backup

```bash
# Manual backup
kubectl exec redis-0 -n fanet -- redis-cli BGSAVE
kubectl exec redis-0 -n fanet -- cp /data/dump.rdb /backup/$(date +%Y%m%d_%H%M%S)_dump.rdb

# Automated backup (cron job)
apiVersion: batch/v1
kind: CronJob
metadata:
  name: redis-backup
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: redis:alpine
            command: ["redis-cli", "-h", "redis-cluster", "BGSAVE"]
```

#### Configuration Backup

```bash
# Backup всех K8s resources
kubectl get all,cm,secret,ingress,pdb,hpa,netpol -n fanet -o yaml > fanet-backup-$(date +%Y%m%d).yaml

# Restore
kubectl apply -f fanet-backup-20240101.yaml
```

## 📊 Performance Tuning

### Scaling Decisions

#### Horizontal Scaling

```bash
# Увеличить min replicas для стабильности
kubectl patch hpa fanet-api-hpa -n fanet -p '{"spec":{"minReplicas":5}}'

# Увеличить max replicas для peak traffic
kubectl patch hpa fanet-api-hpa -n fanet -p '{"spec":{"maxReplicas":30}}'
```

#### Vertical Scaling

```bash
# Увеличить resource limits
kubectl patch deployment fanet-api -n fanet -p '
{
  "spec": {
    "template": {
      "spec": {
        "containers": [{
          "name": "fanet-api",
          "resources": {
            "limits": {"memory": "1Gi", "cpu": "1000m"}
          }
        }]
      }
    }
  }
}'
```

### Configuration Tuning

#### Redis Optimization

```bash
# Увеличить connection pool
kubectl patch configmap fanet-config -n fanet -p '
{
  "data": {
    "REDIS_POOL_SIZE": "200",
    "REDIS_MIN_IDLE_CONNS": "20"
  }
}'
```

#### MQTT Optimization

```bash
# Увеличить worker pool
kubectl patch configmap fanet-config -n fanet -p '
{
  "data": {
    "WORKER_POOL_SIZE": "200",
    "MAX_BATCH_SIZE": "200"
  }
}'
```

## 🔐 Security Operations

### Access Management

#### Service Accounts

```bash
# Audit service account permissions
kubectl auth can-i --list --as=system:serviceaccount:fanet:fanet-api

# Review RBAC
kubectl describe clusterrole,role -n fanet
```

#### Network Policies

```bash
# Test network connectivity
kubectl exec deployment/fanet-api -n fanet -- nc -zv redis-cluster 6379
kubectl exec deployment/fanet-api -n fanet -- nc -zv mqtt-broker 1883

# Review network policies
kubectl describe networkpolicy -n fanet
```

### Secret Management

```bash
# Rotate secrets (example for MQTT)
kubectl create secret generic fanet-secrets-new -n fanet \
  --from-literal=MQTT_PASSWORD="NEW_SECURE_PASSWORD"

kubectl patch deployment fanet-api -n fanet -p '
{
  "spec": {
    "template": {
      "spec": {
        "containers": [{
          "name": "fanet-api",
          "env": [{
            "name": "MQTT_PASSWORD",
            "valueFrom": {
              "secretKeyRef": {
                "name": "fanet-secrets-new",
                "key": "MQTT_PASSWORD"
              }
            }
          }]
        }]
      }
    }
  }
}'
```

## 📈 Capacity Planning

### Growth Predictions

#### Current Capacity (3 pods)

- **WebSocket connections**: 3,000
- **HTTP requests/sec**: 1,500
- **MQTT messages/sec**: 3,000
- **Memory usage**: 768MB
- **CPU usage**: 750m

#### Scaling Thresholds

| Load Level           | Pods  | Resources  | Notes             |
| -------------------- | ----- | ---------- | ----------------- |
| **Low** (< 50%)      | 3-5   | 256MB/250m | Normal operations |
| **Medium** (50-80%)  | 5-10  | 512MB/500m | Peak hours        |
| **High** (80-95%)    | 10-15 | 1GB/1000m  | Traffic spikes    |
| **Critical** (> 95%) | 15-20 | 2GB/2000m  | Emergency scaling |

### Resource Planning

```bash
# Monitor trends
kubectl top pods -n fanet --sort-by=cpu
kubectl top pods -n fanet --sort-by=memory

# Horizontal scaling forecast
# Current: 10k connections = 10 pods
# Growth: 50k connections = 50 pods (need cluster expansion)
```

## 📋 Checklists

### Pre-Deployment Checklist

- [ ] Code review completed
- [ ] Tests passing (unit, integration)
- [ ] Security scan passed
- [ ] Performance testing completed
- [ ] Documentation updated
- [ ] Rollback plan prepared
- [ ] Monitoring alerts configured
- [ ] Change notification sent

### Post-Deployment Checklist

- [ ] Health checks passing
- [ ] Metrics baseline established
- [ ] Error rates within SLA
- [ ] Performance within targets
- [ ] Alerts not firing
- [ ] User acceptance testing
- [ ] Documentation verified
- [ ] Team notification sent

### Incident Response Checklist

- [ ] Incident severity assessed
- [ ] Stakeholders notified
- [ ] Initial investigation completed
- [ ] Workaround implemented (if possible)
- [ ] Root cause identified
- [ ] Fix implemented
- [ ] Service restored
- [ ] Post-incident review scheduled

## 📞 Escalation Paths

### L1 Support (Operations)
- Health checks
- Basic troubleshooting
- Restart services
- Monitor dashboards

### L2 Support (Platform Engineering)
- Configuration changes
- Scaling decisions
- Performance optimization
- Infrastructure issues

### L3 Support (Development Team)
- Code bugs
- Architecture changes
- Complex performance issues
- Security vulnerabilities

### External Dependencies
- **Redis**: Cloud provider support
- **MQTT**: Broker vendor support
- **Kubernetes**: Platform team
- **Laravel API**: FlyBeeper backend team

---

🎯 **Operational Excellence**: Этот guide обеспечивает stable, secure и performant operations для FANET Backend в production.