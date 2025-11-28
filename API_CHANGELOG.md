# API Changelog

Все значимые изменения в API документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
и этот проект придерживается [Semantic Versioning](https://semver.org/lang/ru/).

## [1.0.0] - 2025-01-15

### Добавлено

#### Документация
- Полная OpenAPI 3.1 спецификация (`openapi.yaml`)
- Человекочитаемая документация (`API_COMPREHENSIVE_DOCUMENTATION.md`)
- Руководство по интеграции (`INTEGRATION_GUIDE.md`)
- Коллекции для Postman и Insomnia
- README для документации (`API_DOCUMENTATION_README.md`)

#### Эндпоинты Health
- `GET /api/v1/health` - проверка состояния сервера

#### Эндпоинты Uploads
- `POST /api/v1/upload/handshake` - создание выгрузки
- `POST /api/v1/upload/metadata` - отправка метаданных
- `POST /api/v1/upload/nomenclature/batch` - пакетная загрузка номенклатуры
- `GET /api/uploads` - список выгрузок (с пагинацией и фильтрацией)
- `GET /api/uploads/{uuid}` - детали выгрузки
- `GET /api/uploads/{uuid}/data` - получение данных (XML)
- `GET /api/uploads/{uuid}/stream` - потоковая передача (SSE)
- `POST /api/uploads/{uuid}/verify` - проверка передачи

#### Эндпоинты Database
- `GET /api/databases/list` - список баз данных
- `GET /api/databases/analytics` - аналитика базы данных
- `GET /api/databases/history/{dbname}` - история изменений
- `GET /api/database/info` - информация о текущей БД
- `POST /api/database/switch` - переключение БД

#### Эндпоинты Normalization
- `POST /api/normalization/start` - запуск нормализации
- `GET /api/normalization/status` - статус нормализации
- `POST /api/normalization/stop` - остановка нормализации
- `GET /api/normalization/stats` - статистика нормализации
- `GET /api/normalization/groups` - группы нормализованных данных
- `GET /api/normalization/group-items` - элементы группы
- `POST /api/normalization/export-group` - экспорт группы

#### Эндпоинты Classification
- `POST /api/classification/classify` - классификация товара
- `POST /api/classification/classify-item` - прямая классификация
- `GET /api/classification/strategies` - список стратегий
- `POST /api/classification/strategies/configure` - настройка стратегии
- `GET /api/classification/classifiers` - список классификаторов

#### Эндпоинты Reclassification
- `POST /api/reclassification/start` - запуск переклассификации
- `GET /api/reclassification/status` - статус переклассификации
- `GET /api/reclassification/events` - события переклассификации
- `POST /api/reclassification/stop` - остановка переклассификации

#### Эндпоинты Quality
- `GET /api/quality/stats` - статистика качества
- `GET /api/quality/violations` - список нарушений (с фильтрацией)
- `POST /api/quality/violations/{id}` - разрешение нарушения
- `GET /api/quality/item/{id}` - детали нарушения
- `GET /api/quality/suggestions` - предложения по улучшению
- `GET /api/quality/duplicates` - дубликаты
- `POST /api/quality/assess` - оценка качества

#### Эндпоинты KPVED
- `GET /api/kpved/hierarchy` - иерархия КПВЭД
- `GET /api/kpved/search` - поиск по КПВЭД
- `GET /api/kpved/stats` - статистика КПВЭД
- `POST /api/kpved/load` - загрузка классификатора
- `POST /api/kpved/classify-test` - тестовая классификация
- `POST /api/kpved/classify-hierarchical` - иерархическая классификация
- `POST /api/kpved/reclassify` - переклассификация
- `POST /api/kpved/reclassify-hierarchical` - иерархическая переклассификация

#### Эндпоинты Workers
- `GET /api/workers/config` - получить конфигурацию
- `POST /api/workers/config/update` - обновить конфигурацию
- `GET /api/workers/providers` - список провайдеров
- `GET /api/workers/arliai/status` - статус подключения Arliai
- `GET /api/workers/models` - список моделей

#### Эндпоинты Monitoring
- `GET /api/monitoring/metrics` - метрики производительности
- `GET /api/monitoring/cache` - статистика кеша
- `GET /api/monitoring/ai` - статистика AI запросов

#### Эндпоинты Patterns
- `POST /api/patterns/detect` - обнаружение паттернов
- `POST /api/patterns/suggest` - предложение исправлений
- `POST /api/patterns/test-batch` - тестирование на выборке

#### Эндпоинты Snapshots
- `GET /api/snapshots` - список срезов
- `POST /api/snapshots` - создание среза
- `GET /api/snapshots/{id}` - детали среза
- `POST /api/snapshots/auto` - автоматический срез

#### Эндпоинты Clients
- `GET /api/clients` - список клиентов
- `POST /api/clients` - создание клиента
- `GET /api/clients/{id}` - детали клиента
- `GET /api/clients/{id}/projects` - проекты клиента

### Изменено

- Улучшена обработка ошибок с детальными сообщениями
- Добавлена поддержка пагинации для всех списковых эндпоинтов
- Добавлена фильтрация и сортировка
- Улучшена структура ответов

### Исправлено

- Исправлены проблемы с кодировкой в XML ответах
- Улучшена производительность при работе с большими объемами данных
- Исправлена обработка ошибок в потоковой передаче

## [Unreleased]

### Планируется

- Аутентификация через API ключи
- OAuth2 поддержка
- Webhook уведомления
- GraphQL эндпоинт
- Версионирование через заголовки
- Расширенная аналитика

---

## Типы изменений

- `Добавлено` - новые функции
- `Изменено` - изменения в существующих функциях
- `Устарело` - функции, которые скоро будут удалены
- `Удалено` - удаленные функции
- `Исправлено` - исправления ошибок
- `Безопасность` - исправления уязвимостей

