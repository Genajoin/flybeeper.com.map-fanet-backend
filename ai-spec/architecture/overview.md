# Архитектура FANET Backend

## Обзор системы

FANET Backend - высокопроизводительная система для обработки real-time данных от FANET устройств с минимальным энергопотреблением и максимальной эффективностью.

## Архитектурная диаграмма

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  FANET Devices  │     │  Base Stations  │     │  Web Clients    │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                        │
         │ LoRa                  │                        │ HTTPS
         ▼                       ▼                        ▼
┌─────────────────────────────────────────┐     ┌─────────────────┐
│           MQTT Broker                    │     │   Nginx Proxy   │
│         (Mosquitto/EMQX)                │     │   (HTTP/2+TLS)  │
└────────────────┬─────────────────────────┘     └────────┬────────┘
                 │                                         │
                 │ MQTT                                    │ HTTP/2
                 ▼                                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Go FANET Backend                            │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────┐   │
│  │ MQTT Handler │  │ HTTP Handler │  │ WebSocket Handler  │   │
│  └──────┬───────┘  └──────┬───────┘  └─────────┬──────────┘   │
│         │                  │                     │              │
│         ▼                  ▼                     ▼              │
│  ┌──────────────────────────────────────────────────────┐      │
│  │                  Service Layer                        │      │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌───────────┐ │      │
│  │  │ Pilot   │ │ Thermal │ │ Station │ │ Auth      │ │      │
│  │  │ Service │ │ Service │ │ Service │ │ Service   │ │      │
│  │  └─────────┘ └─────────┘ └─────────┘ └───────────┘ │      │
│  └──────────────────────┬───────────────────────────────┘      │
│                         │                                       │
│         ┌───────────────┴────────────────┐                     │
│         ▼                                ▼                     │
│  ┌─────────────┐                 ┌──────────────┐             │
│  │ Redis Cache │                 │ MySQL Backup │             │
│  │  (Primary)  │                 │  (Fallback)  │             │
│  └─────────────┘                 └──────────────┘             │
└─────────────────────────────────────────────────────────────────┘
```

## Компоненты системы

### 1. Data Sources

#### FANET Devices
- Параплан вариометры с GPS и LoRa
- Метеостанции
- Наземные трекеры
- Передача через LoRa на частоте 868 МГц

#### Base Stations
- Raspberry Pi или ESP32 с LoRa модулем
- Прием FANET пакетов
- Пересылка в MQTT broker
- Покрытие радиусом до 50км

### 2. Message Layer

#### MQTT Broker
- **Продакшн**: EMQX для масштабируемости
- **Разработка**: Mosquitto для простоты
- Топики: `fb/b/{chip_id}/f` для FANET данных
- QoS 0 для tracking, QoS 1 для критических данных

### 3. Application Layer

#### Go FANET Backend
Основной сервис обработки данных:

**MQTT Handler**
- Подписка на FANET топики
- Парсинг бинарных пакетов
- Валидация и дедупликация
- Запись в Redis

**HTTP Handler**
- REST API endpoints
- HTTP/2 + Protobuf
- Аутентификация Bearer token
- Rate limiting

**WebSocket Handler**
- Real-time обновления
- Дифференциальная синхронизация
- Региональная подписка
- Heartbeat monitoring

### 4. Service Layer

#### Pilot Service
- Управление позициями пилотов
- Построение треков
- Детекция аномалий

#### Thermal Service
- Агрегация термических данных
- Расчет качества термиков
- Временная фильтрация

#### Station Service
- Обработка метеоданных
- Расчет трендов
- Интерполяция данных

#### Auth Service
- Валидация Bearer токенов
- Интеграция с Laravel API
- Кэширование разрешений

### 5. Storage Layer

#### Redis (Primary)
- **Геоданные**: GEOADD/GEORADIUS для позиций
- **Хэши**: HSET для атрибутов объектов
- **Списки**: LPUSH для треков
- **TTL**: автоматическая очистка старых данных
- **Pub/Sub**: координация между инстансами

#### MySQL (Backup)
- Холодный старт: загрузка initial state
- Периодический backup из Redis
- Аналитические запросы
- Долгосрочное хранение

## Потоки данных

### 1. Ingestion Flow

```
FANET Device → LoRa → Base Station → MQTT → Go Backend → Redis
```

1. Устройство передает позицию каждые 1-10 секунд
2. Базовая станция принимает и добавляет метаданные (RSSI, SNR)
3. MQTT broker распределяет по подписчикам
4. Go backend парсит, валидирует и сохраняет

### 2. Query Flow

```
Client → HTTP/2 → Go Backend → Redis → Protobuf → Client
```

1. Клиент запрашивает snapshot региона
2. Backend выполняет GEORADIUS в Redis
3. Данные сериализуются в Protobuf
4. HTTP/2 отправляет компактный ответ

### 3. Real-time Flow

```
Redis Update → Go Backend → WebSocket → Client
```

1. Обновление в Redis триггерит событие
2. Backend определяет затронутые регионы
3. Дифференциальное обновление по WebSocket
4. Клиент применяет изменения к локальному состоянию

## Масштабирование

### Горизонтальное масштабирование

```
┌─────────┐  ┌─────────┐  ┌─────────┐
│Backend 1│  │Backend 2│  │Backend 3│
└────┬────┘  └────┬────┘  └────┬────┘
     │            │            │
     └────────────┼────────────┘
                  │
           ┌──────┴──────┐
           │Redis Cluster│
           └─────────────┘
```

- Stateless backend инстансы
- Redis Cluster для шардирования данных
- Load balancer для распределения нагрузки
- Sticky sessions для WebSocket

### Вертикальное масштабирование

- CPU: больше ядер для goroutines
- RAM: больше памяти для кэша
- Network: 10Gbps для high traffic

## Безопасность

### Network Security
- TLS 1.3 для всех соединений
- HTTP/2 с обязательным шифрованием
- VPN для MQTT (опционально)

### Application Security
- Bearer token аутентификация
- Rate limiting по IP и токену
- Input validation на всех уровнях
- Защита от replay attacks

### Data Security
- Анонимизация приватных данных
- Шифрование sensitive полей
- Audit logging всех изменений
- GDPR compliance

## Мониторинг

### Метрики (Prometheus)
- Количество активных соединений
- Скорость обработки сообщений
- Latency percentiles (p50, p95, p99)
- Размер очередей
- Использование памяти/CPU

### Логирование (структурированное)
- JSON формат для парсинга
- Correlation ID для трассировки
- Уровни: ERROR, WARN, INFO, DEBUG
- Ротация и архивирование

### Алерты
- Высокая latency (> 100ms)
- Потеря MQTT соединения
- Redis memory > 80%
- Error rate > 1%

## Отказоустойчивость

### Уровень приложения
- Graceful shutdown
- Circuit breakers
- Retry с exponential backoff
- Timeout на всех операциях

### Уровень инфраструктуры
- Redis Sentinel для failover
- MQTT кластер
- Multi-AZ deployment
- Автоматическое восстановление

## Производительность

### Целевые показатели
- Latency: p95 < 50ms
- Throughput: 10k msg/sec
- Concurrent connections: 10k+
- Memory per connection: < 100KB

### Оптимизации
- Zero-copy где возможно
- Object pooling
- Batch операции
- Efficient serialization