# FANET API - Frontend Integration Guide

Руководство по интеграции FANET API с frontend приложением maps.flybeeper.com

## 📋 Обзор

FANET API предоставляет high-performance real-time данные о полетах парапланов и дельтапланов через:
- **REST API** - для получения снимков данных и отправки позиций
- **WebSocket** - для real-time обновлений на карте
- **SSO аутентификация** - через Laravel Passport API

## 🔗 Endpoints

### Base URL
- **Production**: `https://fanet-api.flybeeper.com`
- **Development**: `http://localhost:8090`

### API Версия
Все endpoints используют префикс `/api/v1/`

## 🔐 Аутентификация

### Workflow SSO
1. **Пользователь логинится через Laravel API**
2. **Frontend получает Bearer token**  
3. **Использует токен для всех запросов к FANET API**

### Подробная спецификация
Полная документация аутентификации: **[ai-spec/auth-integration.md](ai-spec/auth-integration.md)**

### Получение токена
```javascript
// Логин через Laravel API
const loginResponse = await fetch('https://api.flybeeper.com/api/v4/login', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    email: 'pilot@example.com',
    password: 'password'
  })
});

const { token } = await loginResponse.json();
```

### Использование токена
```javascript
// Все запросы к FANET API с токеном
const response = await fetch('https://fanet-api.flybeeper.com/api/v1/snapshot?lat=46&lon=8&radius=50', {
  headers: {
    'Authorization': `Bearer ${token}`,
    'Accept': 'application/json'
  }
});
```

## 📡 REST API Endpoints

### GET /api/v1/snapshot
Получить начальный снимок всех объектов в регионе

**Параметры:**
- `lat` (required) - широта центра (-90 до 90)
- `lon` (required) - долгота центра (-180 до 180)  
- `radius` (required) - радиус в км (1-200)

**Пример:**
```javascript
const snapshot = await fetch('/api/v1/snapshot?lat=46.0&lon=8.0&radius=50', {
  headers: { 'Accept': 'application/json' }
});

const data = await snapshot.json();
// data.pilots - массив пилотов
// data.thermals - массив термиков  
// data.stations - массив метеостанций
// data.sequence - номер последовательности для WebSocket
```

### GET /api/v1/pilots
Получить пилотов в указанных границах

**Параметры:**
- `bounds` (required) - границы: "sw_lat,sw_lon,ne_lat,ne_lon"

**Пример:**
```javascript
const pilots = await fetch('/api/v1/pilots?bounds=45.5,15.0,47.5,16.2');
```

### GET /api/v1/thermals  
Получить термики в указанных границах

**Параметры:**
- `bounds` (required) - границы: "sw_lat,sw_lon,ne_lat,ne_lon"
- `min_quality` (optional) - минимальное качество (0-5)

### GET /api/v1/stations
Получить метеостанции в указанных границах

**Параметры:**
- `bounds` (required) - границы: "sw_lat,sw_lon,ne_lat,ne_lon"

### POST /api/v1/position 🔒 (Authentication Required)
Отправить свою позицию (требует аутентификации)

**Headers:**
- `Authorization: Bearer {token}` (required)
- `Content-Type: application/json`

**Body:**
```json
{
  "position": {
    "latitude": 46.0,
    "longitude": 8.0
  },
  "altitude": 1000,
  "speed": 25.5,
  "climb": 2.1,
  "course": 180.0,
  "timestamp": 1640995200
}
```

**Пример:**
```javascript
const response = await fetch('/api/v1/position', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    position: { latitude: 46.0, longitude: 8.0 },
    altitude: 1000,
    timestamp: Math.floor(Date.now() / 1000)
  })
});
```

## 🔄 WebSocket Real-time Updates

### Подключение
```javascript
const ws = new WebSocket('wss://fanet-api.flybeeper.com/ws/v1/updates?lat=46&lon=8&radius=50');

ws.onopen = () => {
  console.log('WebSocket connected');
};

ws.onmessage = (event) => {
  // Данные приходят в бинарном формате (Protobuf)
  // Необходимо использовать protobuf.js для декодирования
  const update = decodeProtobufUpdate(event.data);
  
  switch(update.type) {
    case 'PILOT':
      updatePilotOnMap(update.pilot);
      break;
    case 'THERMAL':
      addThermalToMap(update.thermal);
      break;
    case 'STATION':
      updateStationOnMap(update.station);
      break;
  }
};

ws.onclose = () => {
  console.log('WebSocket disconnected');
  // Реализовать автоматическое переподключение
};
```

### Heartbeat
WebSocket отправляет ping каждые 30 секунд. Клиент должен отвечать pong для поддержания соединения.

```javascript
ws.onmessage = (event) => {
  if (event.data === 'ping') {
    ws.send('pong');
    return;
  }
  // Обработка обычных сообщений
};
```

## 📊 Форматы данных

### Pilot Object
```json
{
  "addr": 123456,
  "name": "Pilot Name",
  "type": "paraglider",
  "position": {
    "latitude": 46.0,
    "longitude": 8.0
  },
  "altitude": 1000,
  "speed": 25.5,
  "climb": 2.1,
  "course": 180.0,
  "last_update": 1640995200,
  "track_online": true,
  "battery": 85
}
```

### Thermal Object
```json
{
  "id": 789,
  "addr": 123456,
  "position": {
    "latitude": 46.0,
    "longitude": 8.0
  },
  "altitude": 1200,
  "quality": 4,
  "climb": 3.2,
  "wind_speed": 5.5,
  "wind_heading": 270.0,
  "timestamp": 1640995200
}
```

### Station Object
```json
{
  "addr": 456789,
  "name": "Mountain Station",
  "position": {
    "latitude": 46.0,
    "longitude": 8.0
  },
  "temperature": 15.5,
  "wind_speed": 8.2,
  "wind_heading": 270.0,
  "wind_gusts": 12.1,
  "humidity": 65,
  "pressure": 1013.25,
  "battery": 78,
  "last_update": 1640995200
}
```

## 🎯 Рекомендации по интеграции

### 1. Получение начальных данных
```javascript
async function initializeMap(lat, lon, radius = 50) {
  try {
    // 1. Получаем снимок данных
    const snapshot = await fetchSnapshot(lat, lon, radius);
    
    // 2. Отображаем пилотов на карте
    snapshot.pilots.forEach(pilot => addPilotToMap(pilot));
    
    // 3. Отображаем термики
    snapshot.thermals.forEach(thermal => addThermalToMap(thermal));
    
    // 4. Отображаем станции
    snapshot.stations.forEach(station => addStationToMap(station));
    
    // 5. Подключаемся к WebSocket для real-time обновлений
    connectWebSocket(lat, lon, radius, snapshot.sequence);
    
  } catch (error) {
    console.error('Failed to initialize map:', error);
  }
}
```

### 2. Real-time обновления
```javascript
function handleWebSocketUpdate(update) {
  switch(update.action) {
    case 'ADD':
      addObjectToMap(update.data);
      break;
    case 'UPDATE':
      updateObjectOnMap(update.data);
      break;
    case 'REMOVE':
      removeObjectFromMap(update.data.addr);
      break;
  }
}
```

### 3. Отправка позиции пользователя
```javascript
async function sendPosition(position) {
  if (!authToken) {
    console.warn('User not authenticated, cannot send position');
    return;
  }
  
  try {
    await fetch('/api/v1/position', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${authToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        position: {
          latitude: position.lat,
          longitude: position.lng
        },
        altitude: position.altitude,
        timestamp: Math.floor(Date.now() / 1000)
      })
    });
  } catch (error) {
    console.error('Failed to send position:', error);
  }
}
```

### 4. Обработка ошибок аутентификации
```javascript
async function apiRequest(url, options = {}) {
  const response = await fetch(url, {
    ...options,
    headers: {
      'Authorization': `Bearer ${authToken}`,
      ...options.headers
    }
  });
  
  if (response.status === 401) {
    // Токен истек или недействителен
    authToken = null;
    showLoginModal();
    throw new Error('Authentication required');
  }
  
  return response;
}
```

## ⚡ Производительность

### Рекомендации:
1. **Кеширование**: Используйте localStorage для кеширования токенов
2. **Батчинг**: Группируйте обновления позиции (не чаще 1 раз в секунду)
3. **Геофильтрация**: Запрашивайте только видимую область карты
4. **Debouncing**: Используйте debounce для изменений viewport
5. **WebSocket**: Предпочитайте WebSocket для real-time данных

### Пример оптимизированного запроса:
```javascript
// Debounce для изменений карты
const debouncedMapUpdate = debounce((bounds) => {
  updateMapData(bounds);
}, 500);

map.on('moveend', () => {
  const bounds = map.getBounds();
  debouncedMapUpdate(bounds);
});
```

## 🛠️ Отладка и тестирование

### Development Environment
```bash
# Запуск локального FANET API
git clone https://github.com/flybeeper/fanet-backend
cd fanet-backend
make dev-env && make dev

# API доступен на http://localhost:8090
```

### Тестовые данные
```bash
# Генерация тестовых MQTT данных
make mqtt-test-quick
```

### Health Check
```javascript
const health = await fetch('/health');
// Должен вернуть: {"status":"ok","timestamp":...,"version":"1.0.0"}
```

## 📚 Дополнительные ресурсы

- **[OpenAPI спецификация](ai-spec/api/rest-api.yaml)** - полная REST API документация
- **[WebSocket протокол](ai-spec/api/websocket-protocol.md)** - детали WebSocket сообщений  
- **[Аутентификация](ai-spec/auth-integration.md)** - полная спецификация SSO
- **[FANET протокол](ai-spec/mqtt/)** - спецификация FANET сообщений

## 🆘 Поддержка

При возникновении проблем:
1. Проверьте health endpoint: `/health`
2. Убедитесь в валидности токена аутентификации
3. Проверьте CORS настройки для cross-origin запросов
4. Обратитесь к [troubleshooting guide](TROUBLESHOOTING.md)

---

**Последнее обновление**: Январь 2025  
**API версия**: v1.0.0  
**Совместимость**: Laravel Passport v10.1+