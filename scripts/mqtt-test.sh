#!/bin/bash

# MQTT Test Publisher Script
# Скрипт для публикации тестовых FANET данных в MQTT

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Значения по умолчанию
BROKER_URL="${MQTT_URL:-tcp://localhost:1883}"
CHIP_IDS="8896672,7048812,2462966788"
PACKET_TYPES="1,2,4,7,9"
RATE="2s"
MAX_MESSAGES="0"
CLIENT_ID="fanet-test-publisher"
LAT="46.0"
LON="8.0"
SPEED="50.0"

# Путь к исполняемому файлу
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EXECUTABLE="$SCRIPT_DIR/mqtt-test-publisher"
SOURCE_FILE="$SCRIPT_DIR/mqtt-test-publisher.go"

# Функция для вывода помощи
show_help() {
    echo -e "${BLUE}MQTT Test Publisher для FANET протокола${NC}"
    echo ""
    echo "Использование: $0 [OPTIONS]"
    echo ""
    echo "Опции:"
    echo "  -b, --broker URL        MQTT broker URL (default: $BROKER_URL)"
    echo "  -c, --chips IDs         Chip IDs через запятую (default: $CHIP_IDS)"
    echo "  -t, --types TYPES       Типы пакетов через запятую (default: $PACKET_TYPES)"
    echo "  -r, --rate DURATION     Частота публикации (default: $RATE)"
    echo "  -m, --max NUMBER        Максимум сообщений, 0=бесконечно (default: $MAX_MESSAGES)"
    echo "  -i, --client-id ID      MQTT Client ID (default: $CLIENT_ID)"
    echo "  --lat LATITUDE          Стартовая широта (default: $LAT)"
    echo "  --lon LONGITUDE         Стартовая долгота (default: $LON)"
    echo "  --speed SPEED           Скорость движения км/ч (default: $SPEED)"
    echo "  --build                 Пересобрать исполняемый файл"
    echo "  --clean                 Удалить исполняемый файл"
    echo "  -h, --help             Показать эту справку"
    echo ""
    echo "Примеры:"
    echo "  $0                                          # Базовый запуск"
    echo "  $0 -r 1s -m 100                           # Быстро, 100 сообщений"
    echo "  $0 -b tcp://192.168.1.100:1883            # Удаленный брокер"
    echo "  $0 -t 1,2 --lat 47.5 --lon 9.0            # Только tracking и name"
    echo "  $0 --build                                 # Пересборка"
    echo ""
    echo "Типы пакетов FANET:"
    echo "  1 - Air Tracking (воздушное судно)"
    echo "  2 - Name (имя пилота)"
    echo "  4 - Service/Weather (метеостанция)"
    echo "  7 - Ground Tracking (наземный объект)"
    echo "  9 - Thermal (термик)"
}

# Функция для сборки
build_publisher() {
    echo -e "${YELLOW}🔨 Сборка MQTT Test Publisher...${NC}"
    
    if ! command -v go &> /dev/null; then
        echo -e "${RED}❌ Go не найден. Установите Go для сборки.${NC}"
        exit 1
    fi

    cd "$SCRIPT_DIR"
    
    # Проверяем go.mod в корне проекта
    if [ ! -f "../go.mod" ]; then
        echo -e "${RED}❌ go.mod не найден в корне проекта${NC}"
        exit 1
    fi

    # Сборка с учетом модуля
    if go build -o "$EXECUTABLE" "$SOURCE_FILE"; then
        echo -e "${GREEN}✅ Сборка завершена: $EXECUTABLE${NC}"
    else
        echo -e "${RED}❌ Ошибка сборки${NC}"
        exit 1
    fi
}

# Функция для очистки
clean_publisher() {
    echo -e "${YELLOW}🧹 Удаление исполняемого файла...${NC}"
    if [ -f "$EXECUTABLE" ]; then
        rm "$EXECUTABLE"
        echo -e "${GREEN}✅ Файл удален${NC}"
    else
        echo -e "${YELLOW}⚠️  Исполняемый файл не найден${NC}"
    fi
}

# Функция для проверки исполняемого файла
check_executable() {
    if [ ! -f "$EXECUTABLE" ]; then
        echo -e "${YELLOW}⚠️  Исполняемый файл не найден. Собираем...${NC}"
        build_publisher
    elif [ "$SOURCE_FILE" -nt "$EXECUTABLE" ]; then
        echo -e "${YELLOW}⚠️  Исходный код новее исполняемого файла. Пересобираем...${NC}"
        build_publisher
    fi
}

# Парсинг аргументов командной строки
while [[ $# -gt 0 ]]; do
    case $1 in
        -b|--broker)
            BROKER_URL="$2"
            shift 2
            ;;
        -c|--chips)
            CHIP_IDS="$2"
            shift 2
            ;;
        -t|--types)
            PACKET_TYPES="$2"
            shift 2
            ;;
        -r|--rate)
            RATE="$2"
            shift 2
            ;;
        -m|--max)
            MAX_MESSAGES="$2"
            shift 2
            ;;
        -i|--client-id)
            CLIENT_ID="$2"
            shift 2
            ;;
        --lat)
            LAT="$2"
            shift 2
            ;;
        --lon)
            LON="$2"
            shift 2
            ;;
        --speed)
            SPEED="$2"
            shift 2
            ;;
        --build)
            build_publisher
            exit 0
            ;;
        --clean)
            clean_publisher
            exit 0
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo -e "${RED}❌ Неизвестная опция: $1${NC}"
            echo "Используйте -h для справки"
            exit 1
            ;;
    esac
done

# Проверка и сборка если необходимо
check_executable

# Проверка доступности MQTT брокера
echo -e "${BLUE}🔍 Проверка подключения к MQTT брокеру...${NC}"
if command -v mosquitto_pub &> /dev/null; then
    # Тестовое сообщение для проверки
    if timeout 5 mosquitto_pub -h "${BROKER_URL#tcp://}" -h "${BROKER_URL%:*}" -p "${BROKER_URL##*:}" -t "test/connection" -m "test" -q 0 >/dev/null 2>&1; then
        echo -e "${GREEN}✅ MQTT брокер доступен${NC}"
    else
        echo -e "${YELLOW}⚠️  Не удается подключиться к MQTT брокеру. Проверьте, что брокер запущен.${NC}"
        echo -e "${YELLOW}   Для запуска локального брокера: make dev-env${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  mosquitto_pub не найден. Пропускаем проверку подключения.${NC}"
fi

echo ""
echo -e "${GREEN}🚀 Запуск MQTT Test Publisher...${NC}"
echo -e "${BLUE}📡 Брокер:${NC} $BROKER_URL"
echo -e "${BLUE}📟 Базовые станции:${NC} $CHIP_IDS"
echo -e "${BLUE}📦 Типы пакетов:${NC} $PACKET_TYPES"
echo -e "${BLUE}⏱️  Частота:${NC} $RATE"
echo -e "${BLUE}🌍 Позиция:${NC} $LAT, $LON"
if [ "$MAX_MESSAGES" != "0" ]; then
    echo -e "${BLUE}🔢 Максимум сообщений:${NC} $MAX_MESSAGES"
fi
echo ""
echo -e "${YELLOW}Нажмите Ctrl+C для остановки${NC}"
echo ""

# Запуск издателя
exec "$EXECUTABLE" \
    -broker "$BROKER_URL" \
    -chips "$CHIP_IDS" \
    -types "$PACKET_TYPES" \
    -rate "$RATE" \
    -max "$MAX_MESSAGES" \
    -client "$CLIENT_ID" \
    -lat "$LAT" \
    -lon "$LON" \
    -speed "$SPEED"