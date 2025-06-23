# FANET API - Testing Guide

Руководство по тестированию для проекта FANET API Backend.

## 📋 Обзор системы тестирования

Проект использует comprehensive подход к тестированию с несколькими уровнями тестов:

- **Unit Tests** - тестирование отдельных компонентов
- **Integration Tests** - тестирование взаимодействия компонентов
- **Benchmark Tests** - тестирование производительности
- **End-to-End Tests** - полные сценарии использования

## 🏗 Структура тестов

```
services/api-fanet/
├── internal/
│   ├── auth/auth_test.go           # Unit тесты аутентификации
│   ├── handler/rest_test.go        # Unit тесты HTTP handlers
│   ├── integration/                # Integration тесты
│   │   ├── mqtt_pipeline_test.go   # MQTT → Redis pipeline
│   │   └── api_endpoints_test.go   # REST API endpoints
│   ├── models/                     # Unit тесты моделей
│   │   ├── pilot_test.go
│   │   ├── thermal_test.go
│   │   └── geo_test.go
│   ├── mqtt/parser_test.go         # Unit тесты MQTT парсера
│   ├── repository/redis_test.go    # Unit тесты Redis repository
│   └── service/validation_test.go  # Unit тесты валидации
├── benchmarks/                     # Benchmark тесты
│   ├── parser_benchmark_test.go
│   ├── redis_benchmark_test.go
│   └── websocket_benchmark_test.go
└── testdata/                       # Тестовые данные
    ├── bad-track.geojson
    └── *.geojson
```

## 🚀 Быстрый старт

### Установка зависимостей
```bash
make deps
```

### Запуск всех тестов
```bash
make test
```

### Запуск только unit тестов
```bash
make test-unit
```

### Запуск с анализом покрытия
```bash
make test-coverage
```

## 📊 Команды тестирования

### Основные команды

| Команда | Описание |
|---------|----------|
| `make test` | Запуск всех тестов (unit + integration) |
| `make test-unit` | Только unit тесты |
| `make test-integration` | Только integration тесты |
| `make test-coverage` | Unit тесты с анализом покрытия |
| `make test-verbose` | Verbose вывод всех тестов |
| `make test-short` | Быстрые тесты (без race detection) |

### Дополнительные команды

| Команда | Описание |
|---------|----------|
| `make bench` | Benchmark тесты |
| `make lint` | Линтинг кода |
| `make lint-fix` | Автоматическое исправление линтинга |

### Примеры использования

```bash
# Запуск конкретного теста
go test -v ./internal/mqtt/

# Запуск с фильтром по имени
go test -run TestParser_Parse ./internal/mqtt/

# Запуск benchmarks с профилированием
go test -bench=. -benchmem ./benchmarks/

# Запуск integration тестов
go test -v ./internal/integration/
```

## 🔧 Требования для тестов

### Unit Tests
- Не требуют внешних зависимостей
- Используют mocks и stubs
- Быстрые и изолированные

### Integration Tests
Требуют запущенные сервисы:

**Redis** (порт 6379):
```bash
docker run -d --name redis-test -p 6379:6379 redis:alpine
```

**MQTT** (порт 1883):
```bash
docker run -d --name mosquitto-test -p 1883:1883 eclipse-mosquitto
```

**Или используйте docker-compose:**
```bash
# Из корня flybeeper проекта
make dev-infra
```

## 📈 Покрытие кода

### Цели покрытия
- **Unit tests**: ≥80% покрытия
- **Critical paths**: ≥90% покрытия
- **Integration tests**: полное покрытие основных сценариев

### Анализ покрытия
```bash
# Генерация отчета о покрытии
make test-coverage

# Просмотр HTML отчета
open coverage.html

# Консольный вывод покрытия
go tool cover -func=coverage.out
```

### Исключения из покрытия
- Сгенерированный protobuf код (`*.pb.go`)
- Мертвый код и заглушки
- Некритичные error paths

## 🧪 Типы тестов

### 1. Unit Tests

**Что тестируем:**
- Отдельные функции и методы
- Бизнес-логику
- Validation rules
- Data transformations

**Пример:**
```go
func TestPilot_Validate(t *testing.T) {
    pilot := &models.Pilot{
        DeviceID: "ABC123",
        Position: &models.GeoPoint{Latitude: 46.0, Longitude: 8.0},
        LastUpdate: time.Now(),
    }
    
    err := pilot.Validate()
    assert.NoError(t, err)
}
```

### 2. Integration Tests

**Что тестируем:**
- MQTT → Redis pipeline
- REST API endpoints
- Database operations
- Межкомпонентное взаимодействие

**Пример:**
```go
func (suite *MQTTPipelineTestSuite) TestMQTTToRedisPipeline() {
    // Публикуем MQTT сообщение
    suite.mqttClient.Publish("fb/b/TEST01/f/1", 0, false, payload)
    
    // Проверяем сохранение в Redis
    pilots, err := suite.redisRepo.GetPilotsInRadius(ctx, center, 50, nil)
    assert.NoError(suite.T(), err)
    assert.Len(suite.T(), pilots, 1)
}
```

### 3. Benchmark Tests

**Что тестируем:**
- Производительность парсинга
- Пропускную способность Redis
- Скорость WebSocket broadcasts

**Пример:**
```go
func BenchmarkParser_Parse(b *testing.B) {
    parser := NewParser(logger)
    payload := createTestPayload()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := parser.Parse("fb/b/TEST/f/1", payload)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

## 🎯 Best Practices

### Naming Conventions
- Test files: `*_test.go`
- Test functions: `TestFunctionName`
- Benchmark functions: `BenchmarkFunctionName`
- Test suites: `*TestSuite`

### Test Structure
```go
func TestFunction(t *testing.T) {
    // Arrange - подготовка данных
    input := createTestInput()
    expected := createExpectedOutput()
    
    // Act - выполнение тестируемой функции
    result, err := FunctionUnderTest(input)
    
    // Assert - проверка результатов
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

### Используемые библиотеки
- **testify/assert** - assertions
- **testify/require** - критические проверки
- **testify/mock** - mocking
- **testify/suite** - test suites

### Мокирование
```go
type MockRepository struct {
    mock.Mock
}

func (m *MockRepository) GetPilot(ctx context.Context, id string) (*models.Pilot, error) {
    args := m.Called(ctx, id)
    return args.Get(0).(*models.Pilot), args.Error(1)
}
```

## 🐛 Отладка тестов

### Debug режим
```bash
# Включение debug логирования
export LOG_LEVEL=debug
go test -v ./internal/mqtt/

# Запуск одного теста с отладкой
go test -v -run TestSpecificTest ./internal/mqtt/
```

### Профилирование тестов
```bash
# CPU профилирование
go test -cpuprofile=cpu.prof -bench=.
go tool pprof cpu.prof

# Memory профилирование  
go test -memprofile=mem.prof -bench=.
go tool pprof mem.prof
```

### Race Condition Detection
```bash
# Включение race detector
go test -race ./...

# В CI/CD всегда используется race detection
```

## 🔍 Continuous Integration

### GitHub Actions
Пример конфигурации `.github/workflows/test.yml`:

```yaml
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      redis:
        image: redis:alpine
        ports:
          - 6379:6379
      mosquitto:
        image: eclipse-mosquitto
        ports:
          - 1883:1883
    
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.23'
      
      - name: Run tests
        run: make test-coverage
      
      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          file: ./coverage.out
```

### Pre-commit Hooks
```bash
# Установка pre-commit hooks
go install github.com/pre-commit/pre-commit@latest

# .pre-commit-config.yaml
repos:
  - repo: local
    hooks:
      - id: go-test
        name: go test
        entry: make test-short
        language: system
        pass_filenames: false
      
      - id: go-lint
        name: go lint
        entry: make lint
        language: system
        pass_filenames: false
```

## 📚 Полезные ресурсы

- [Go Testing Package](https://pkg.go.dev/testing)
- [Testify Documentation](https://github.com/stretchr/testify)
- [Go Testing Best Practices](https://go.dev/doc/tutorial/add-a-test)
- [Advanced Go Testing](https://segment.com/blog/5-advanced-testing-techniques-in-go/)

## 🆘 Troubleshooting

### Частые проблемы

**1. Redis connection failed**
```bash
# Проверить доступность Redis
redis-cli ping

# Запустить Redis в Docker
docker run -d -p 6379:6379 redis:alpine
```

**2. MQTT broker not available**
```bash
# Проверить MQTT
mosquitto_pub -h localhost -p 1883 -t test -m "hello"

# Запустить Mosquitto
docker run -d -p 1883:1883 eclipse-mosquitto
```

**3. Tests timeout**
```bash
# Увеличить timeout
go test -timeout=30m ./...

# Или в Makefile изменить TEST_TIMEOUT
```

**4. Race conditions**
```bash
# Запуск без race detection для быстрой отладки
go test -short ./...

# Исправление race conditions требует анализа кода
```

## 📝 Заключение

Система тестирования FANET API обеспечивает:

- ✅ **Высокое покрытие** кода тестами (>80%)
- ✅ **Быстрые unit тесты** для разработки
- ✅ **Comprehensive integration тесты** для CI/CD
- ✅ **Автоматическое качество кода** через линтинг
- ✅ **Performance monitoring** через benchmarks

Следуйте этому руководству для поддержания качества кода и надежности системы.