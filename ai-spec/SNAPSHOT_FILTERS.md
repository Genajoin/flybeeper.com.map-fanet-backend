# Snapshot API Filters

## Описание

В API endpoint `/api/v1/snapshot` добавлены дополнительные параметры для более гибкой фильтрации данных:

1. **max_age** - фильтрация объектов по времени последнего обновления
2. **Фильтры типов объектов** - выборочное получение только нужных типов данных
3. **Существующие фильтры** - air-types и ground-types для фильтрации по типам воздушных и наземных объектов

## Новые параметры

### max_age

Параметр `max_age` позволяет получить только те объекты, которые были обновлены в течение указанного времени.

- **Тип**: integer
- **Единицы**: секунды
- **По умолчанию**: 86400 (24 часа)
- **Описание**: Максимальный возраст данных в секундах. Объекты старше указанного времени будут отфильтрованы.

### Фильтры типов объектов

Позволяют выбрать, какие типы объектов включать в ответ:

- **pilots** - воздушные объекты (параплан, дельтаплан и т.д.)
- **stations** - метеостанции
- **thermals** - термические потоки
- **ground_objects** - наземные объекты

**По умолчанию**: все параметры `true` (возвращаются все типы объектов)

## Использование

### Базовый запрос (все объекты за последние 24 часа)
```
GET /api/v1/snapshot?lat=46.5&lon=15.6&radius=200
```

### Только свежие данные (последние 5 минут)
```
GET /api/v1/snapshot?lat=46.5&lon=15.6&radius=200&max_age=300
```

### Только пилоты и термики
```
GET /api/v1/snapshot?lat=46.5&lon=15.6&radius=200&pilots=true&thermals=true&stations=false&ground_objects=false
```

### Только парапланы за последний час
```
GET /api/v1/snapshot?lat=46.5&lon=15.6&radius=200&air-types=1&max_age=3600&pilots=true&stations=false&thermals=false&ground_objects=false
```

### Только наземные объекты с сигналами бедствия
```
GET /api/v1/snapshot?lat=46.5&lon=15.6&radius=200&ground-types=14,15&pilots=false&stations=false&thermals=false&ground_objects=true
```

### Комбинированный запрос
```
GET /api/v1/snapshot?lat=46.5&lon=15.6&radius=200&max_age=600&air-types=1,2&pilots=true&thermals=true&stations=false&ground_objects=false
```

## Примеры ответов

### С фильтрацией по типам объектов
Если `stations=false` и `ground_objects=false`, соответствующие поля будут содержать пустые массивы:

```json
{
  "pilots": [...],
  "ground_objects": [],
  "thermals": [...],
  "stations": [],
  "sequence": 1640995200
}
```

### С фильтрацией по max_age
Объекты старше указанного времени не включаются в ответ:

```json
{
  "pilots": [
    {
      "addr": 12345,
      "name": "Pilot 1",
      "last_update": 1640995100  // Обновлен 100 секунд назад
    }
  ],
  // Пилот с last_update старше max_age не включен
  "thermals": [...],
  "stations": [...],
  "ground_objects": [...]
}
```

## Логирование

При использовании фильтров в логи добавляется дополнительная информация:

```json
{
  "level": "info",
  "lat": 46.5,
  "lon": 15.6,
  "radius": 200,
  "pilots": 5,
  "ground_objects": 0,
  "thermals": 3,
  "stations": 2,
  "max_age_seconds": 300,
  "include_types": {
    "pilots": true,
    "stations": false,
    "thermals": true,
    "ground_objects": false
  },
  "filter_air_types": [1, 2],
  "message": "Snapshot request completed"
}
```

## Производительность

- Использование фильтров типов объектов снижает нагрузку на Redis, так как запрашиваются только нужные данные
- Фильтрация по max_age происходит после получения данных из Redis, но перед сериализацией ответа
- При `pilots=false` соответствующий запрос к Redis не выполняется вообще

## Совместимость

- Все новые параметры полностью опциональны
- Без указания параметров API работает как прежде
- Поддерживается комбинирование всех типов фильтров