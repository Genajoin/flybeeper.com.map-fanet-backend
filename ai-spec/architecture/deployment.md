# Deployment Guide

## Обзор развертывания

FANET Backend разработан для развертывания в контейнерной среде с поддержкой автомасштабирования и высокой доступности.

## Окружения

### Development
- Docker Compose для локальной разработки
- Redis и MQTT в контейнерах
- Hot reload для быстрой итерации

### Staging
- Kubernetes в single-node
- Полная копия production с меньшими ресурсами
- Интеграционное тестирование

### Production
- Kubernetes multi-node cluster
- Auto-scaling based on load
- Multi-region deployment

## Docker

### Dockerfile (Multi-stage build)

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o fanet-api cmd/fanet-api/main.go

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy binary
COPY --from=builder /app/fanet-api .

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8090/health || exit 1

EXPOSE 8090

CMD ["./fanet-api"]
```

### docker-compose.yml (Development)

```yaml
version: '3.8'

services:
  fanet-api:
    build: .
    ports:
      - "8090:8090"
    environment:
      - REDIS_URL=redis://redis:6379
      - MQTT_URL=tcp://mqtt:1883
      - LOG_LEVEL=debug
    depends_on:
      - redis
      - mqtt
    volumes:
      - ./:/app
    command: air # hot reload

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    command: redis-server --appendonly yes

  mqtt:
    image: eclipse-mosquitto:2
    ports:
      - "1883:1883"
      - "9001:9001"
    volumes:
      - ./deployments/mosquitto/mosquitto.conf:/mosquitto/config/mosquitto.conf
      - mqtt-data:/mosquitto/data
      - mqtt-logs:/mosquitto/log

  redis-commander:
    image: rediscommander/redis-commander:latest
    environment:
      - REDIS_HOSTS=local:redis:6379
    ports:
      - "8081:8081"

volumes:
  redis-data:
  mqtt-data:
  mqtt-logs:
```

## Kubernetes

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fanet-api
  namespace: fanet
spec:
  replicas: 3
  selector:
    matchLabels:
      app: fanet-api
  template:
    metadata:
      labels:
        app: fanet-api
    spec:
      containers:
      - name: fanet-api
        image: flybeeper/fanet-api:latest
        ports:
        - containerPort: 8090
          name: http
        - containerPort: 9090
          name: metrics
        env:
        - name: REDIS_URL
          valueFrom:
            secretKeyRef:
              name: fanet-secrets
              key: redis-url
        - name: MQTT_URL
          valueFrom:
            secretKeyRef:
              name: fanet-secrets
              key: mqtt-url
        - name: AUTH_URL
          value: "https://api.flybeeper.com/api/v3/auth/verify"
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8090
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8090
          initialDelaySeconds: 5
          periodSeconds: 10
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: fanet-api
  namespace: fanet
spec:
  selector:
    app: fanet-api
  ports:
  - name: http
    port: 80
    targetPort: 8090
  - name: metrics
    port: 9090
    targetPort: 9090
  type: ClusterIP
```

### Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: fanet-api
  namespace: fanet
  annotations:
    nginx.ingress.kubernetes.io/enable-cors: "true"
    nginx.ingress.kubernetes.io/cors-allow-methods: "GET, POST, OPTIONS"
    nginx.ingress.kubernetes.io/cors-allow-headers: "DNT,X-CustomHeader,Keep-Alive,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Authorization"
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
    nginx.ingress.kubernetes.io/proxy-body-size: "1m"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
spec:
  tls:
  - hosts:
    - fanet-api.flybeeper.com
    secretName: fanet-api-tls
  rules:
  - host: fanet-api.flybeeper.com
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: fanet-api
            port:
              number: 80
      - path: /ws
        pathType: Prefix
        backend:
          service:
            name: fanet-api
            port:
              number: 80
```

### HorizontalPodAutoscaler

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: fanet-api
  namespace: fanet
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: fanet-api
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  - type: Pods
    pods:
      metric:
        name: websocket_connections_per_pod
      target:
        type: AverageValue
        averageValue: "1000"
```

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fanet-config
  namespace: fanet
data:
  config.yaml: |
    server:
      port: 8090
      read_timeout: 10s
      write_timeout: 10s
      max_header_bytes: 1048576
    
    redis:
      max_idle: 10
      max_active: 100
      idle_timeout: 240s
      
    mqtt:
      client_id: fanet-api
      clean_session: false
      order_matters: false
      
    auth:
      cache_ttl: 300s
      
    performance:
      worker_pool_size: 100
      max_batch_size: 100
      batch_timeout: 5s
```

## Redis Deployment

### Redis Cluster (Production)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: redis-cluster
  namespace: fanet
spec:
  type: ClusterIP
  ports:
  - port: 6379
    targetPort: 6379
  selector:
    app: redis-cluster
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis-cluster
  namespace: fanet
spec:
  serviceName: redis-cluster
  replicas: 6
  selector:
    matchLabels:
      app: redis-cluster
  template:
    metadata:
      labels:
        app: redis-cluster
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        command:
          - redis-server
          - /conf/redis.conf
        ports:
        - containerPort: 6379
        volumeMounts:
        - name: conf
          mountPath: /conf
        - name: data
          mountPath: /data
        resources:
          requests:
            cpu: 100m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 1Gi
      volumes:
      - name: conf
        configMap:
          name: redis-cluster-config
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
```

## Monitoring Stack

### Prometheus

```yaml
apiVersion: v1
kind: Service
metadata:
  name: prometheus
  namespace: monitoring
spec:
  type: NodePort
  ports:
  - port: 9090
    targetPort: 9090
    nodePort: 30090
  selector:
    app: prometheus
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      containers:
      - name: prometheus
        image: prom/prometheus:latest
        ports:
        - containerPort: 9090
        volumeMounts:
        - name: config
          mountPath: /etc/prometheus
        - name: data
          mountPath: /prometheus
      volumes:
      - name: config
        configMap:
          name: prometheus-config
      - name: data
        emptyDir: {}
```

### Grafana

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grafana
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grafana
  template:
    metadata:
      labels:
        app: grafana
    spec:
      containers:
      - name: grafana
        image: grafana/grafana:latest
        ports:
        - containerPort: 3000
        env:
        - name: GF_SECURITY_ADMIN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: grafana-secrets
              key: admin-password
```

## CI/CD Pipeline

### GitHub Actions

```yaml
name: Build and Deploy

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    
    - name: Test
      run: |
        go test -v ./...
        go test -race -coverprofile=coverage.out ./...
    
    - name: Lint
      uses: golangci/golangci-lint-action@v3

  build:
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Log in to the Container registry
      uses: docker/login-action@v2
      with:
        registry: ${{ env.REGISTRY }}
        username: ${{ github.actor }}
        password: ${{ secrets.GITHUB_TOKEN }}
    
    - name: Extract metadata
      id: meta
      uses: docker/metadata-action@v4
      with:
        images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
    
    - name: Build and push Docker image
      uses: docker/build-push-action@v4
      with:
        context: .
        push: true
        tags: ${{ steps.meta.outputs.tags }}
        labels: ${{ steps.meta.outputs.labels }}

  deploy:
    needs: build
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'
    
    steps:
    - name: Deploy to Kubernetes
      uses: azure/k8s-deploy@v4
      with:
        manifests: |
          deployments/kubernetes/deployment.yaml
          deployments/kubernetes/service.yaml
        images: |
          ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ github.sha }}
```

## Production Checklist

### Pre-deployment
- [ ] All tests passing
- [ ] Security scan completed
- [ ] Performance benchmarks met
- [ ] Documentation updated
- [ ] Database migrations tested

### Deployment
- [ ] Blue-green deployment strategy
- [ ] Health checks passing
- [ ] Metrics flowing to Prometheus
- [ ] Logs aggregated in ELK
- [ ] Alerts configured

### Post-deployment
- [ ] Smoke tests passing
- [ ] Performance monitored
- [ ] Error rates normal
- [ ] Rollback plan ready
- [ ] Team notified