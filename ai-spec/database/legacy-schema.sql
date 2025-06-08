-- Legacy FANET Database Schema
-- Сохранено для справки из существующей системы MqttToDb

-- Таблица пилотов/объектов (UFO = Unidentified Flying Object)
CREATE TABLE `ufo` (
  `addr` int NOT NULL,         -- FANET адрес устройства
  `last_position` int NOT NULL, -- ID последней позиции в ufo_track
  PRIMARY KEY (`addr`)
);

-- Треки полетов (история позиций)
CREATE TABLE `ufo_track` (
  `id` int NOT NULL AUTO_INCREMENT,
  `addr` int NOT NULL,              -- FANET адрес
  `ufo_type` smallint NOT NULL,     -- Тип: 1=параплан, 2=дельтаплан, 3=планер и т.д.
  `latitude` float NOT NULL,        -- Широта
  `longitude` float NOT NULL,       -- Долгота
  `altitude_gps` float DEFAULT NULL,-- Высота GPS (м)
  `altitude_bar` smallint DEFAULT NULL, -- Барометрическая высота (м)
  `speed` smallint DEFAULT NULL,    -- Скорость (км/ч)
  `climb` smallint DEFAULT NULL,    -- Вертикальная скорость (м/с * 10)
  `course` smallint DEFAULT NULL,   -- Курс (градусы)
  `track_online` tinyint DEFAULT NULL, -- Онлайн трекинг
  `raw_id` int DEFAULT NULL,        -- Ссылка на сырой пакет
  `datestamp` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `addr` (`addr`)
);

-- Имена пилотов/устройств
CREATE TABLE `name` (
  `addr` int NOT NULL,              -- FANET адрес
  `name` varchar(30) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin DEFAULT NULL,
  PRIMARY KEY (`addr`)
);

-- Термические потоки
CREATE TABLE `thermal` (
  `id` int NOT NULL,
  `addr` int NOT NULL,              -- Кто обнаружил термик
  `latitude` float NOT NULL,
  `longitude` float NOT NULL,
  `altitude` int DEFAULT NULL,      -- Высота термика (м)
  `quality` tinyint DEFAULT NULL,   -- Качество 0-5
  `climb` smallint DEFAULT NULL,    -- Средняя скороподъемность (м/с * 10)
  `wind_speed` smallint DEFAULT NULL,   -- Скорость ветра (м/с * 10)
  `wind_heading` smallint DEFAULT NULL, -- Направление ветра (градусы)
  PRIMARY KEY (`id`)
);

-- Метеостанции
CREATE TABLE `station` (
  `addr` int NOT NULL,
  `datestamp` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `name` varchar(30) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin DEFAULT NULL,
  `latitude` float DEFAULT NULL,
  `longitude` float DEFAULT NULL,
  `hw_count` int DEFAULT NULL,          -- Счетчик оборудования
  `internet_gateway` tinyint DEFAULT NULL,
  `temperature` float DEFAULT NULL,     -- Температура (°C)
  `wind_heading` smallint DEFAULT NULL, -- Направление ветра (градусы)
  `wind_speed` float DEFAULT NULL,      -- Скорость ветра (м/с)
  `wind_gusts` float DEFAULT NULL,      -- Порывы ветра (м/с)
  `humidity` tinyint DEFAULT NULL,      -- Влажность (%)
  `pressure` int DEFAULT NULL,          -- Давление (гПа)
  `remote_control` tinyint DEFAULT NULL,
  `battery` tinyint DEFAULT NULL,       -- Заряд батареи (%)
  `last_pkt_id` int DEFAULT NULL,
  PRIMARY KEY (`addr`)
);

-- Сырые MQTT пакеты
CREATE TABLE `packet` (
  `id` int NOT NULL AUTO_INCREMENT,
  `datestamp` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `chip_id` int NOT NULL,           -- ID базовой станции
  `rssi` smallint NOT NULL,         -- Уровень сигнала
  `raw` varchar(70) NOT NULL,       -- Сырые данные (hex)
  `check_status` tinyint DEFAULT NULL,
  `snr` smallint DEFAULT NULL,      -- Signal-to-Noise Ratio
  `forward` bit(1) DEFAULT NULL,    -- Нужно ли пересылать
  PRIMARY KEY (`id`)
);

-- Сервисная информация (ретрансляторы)
CREATE TABLE `service` (
  `id` int NOT NULL AUTO_INCREMENT,
  `datestamp` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `addr` int NOT NULL,
  `battery` int DEFAULT NULL,
  PRIMARY KEY (`id`)
);

-- Биллинг (опционально)
CREATE TABLE `billing` (
  `addr` int UNSIGNED NOT NULL,
  `device_id` bigint UNSIGNED NOT NULL,
  `expiration_timestamp` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `notes` varchar(256) DEFAULT NULL,
  `addr_hex` text GENERATED ALWAYS AS (hex(`addr`)) VIRTUAL,
  PRIMARY KEY (`addr`),
  UNIQUE KEY `device_id` (`device_id`)
);