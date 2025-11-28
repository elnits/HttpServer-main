# Сводка удаления мок-заглушек и захардкоженных значений

## Обзор

Все мок-заглушки и захардкоженные значения модели AI заменены на функциональный код, который использует реальную конфигурацию через `WorkerConfigManager`.

---

## Исправленные файлы

### 1. Server (Go)

#### `server/server.go`
- ✅ Добавлена функция `getModelFromConfig()` для централизованного получения модели
- ✅ Исправлены функции:
  - `handleKpvedClassifyTest()` - использует `getModelFromConfig()`
  - `handleKpvedReclassify()` - использует `getModelFromConfig()`
  - `handleKpvedClassifyHierarchical()` - использует `getModelFromConfig()`
  - `handleKpvedReclassifyHierarchical()` - использует `getModelFromConfig()`
  - `handleStartClientNormalization()` - использует `NewClientNormalizerWithConfig()`

#### `server/server_classification.go`
- ✅ Исправлены функции:
  - `handleClassifyItem()` - использует `getModelFromConfig()`
  - `handleClassifyItemDirect()` - использует `getModelFromConfig()`
  - Все места где создавался `NewAIClassifier` теперь используют модель из конфигурации

#### `server/server_reclassification.go`
- ✅ Исправлена функция `handleReclassifyWithKpved()` - использует `getModelFromConfig()`

#### `server/worker_config.go`
- ✅ Добавлена функция `GetModelAndAPIKey()` для получения модели и API ключа из конфигурации

### 2. Normalization (Go)

#### `normalization/ai_normalizer.go`
- ✅ Добавлен опциональный параметр `model` в `NewAINormalizer()`
- ✅ Поддержка получения модели из переменной окружения `ARLIAI_MODEL`
- ✅ Fallback на дефолтную модель "GLM-4.5-Air" только если модель не указана

#### `normalization/kpved_classifier.go`
- ✅ Добавлен опциональный параметр `model` в `NewKpvedClassifier()`
- ✅ Поддержка получения модели из переменной окружения `ARLIAI_MODEL`
- ✅ Fallback на дефолтную модель "GLM-4.5-Air" только если модель не указана

#### `normalization/client_normalizer.go`
- ✅ Добавлен интерфейс `WorkerConfigManagerInterface` для получения конфигурации
- ✅ Добавлена функция `NewClientNormalizerWithConfig()` с поддержкой конфигурации модели
- ✅ Старая функция `NewClientNormalizer()` теперь вызывает новую с `nil` конфигурацией (для обратной совместимости)

### 3. Command Line Tools (Go)

#### `cmd/reclassify_with_kpved/main.go`
- ✅ Использует `WorkerConfigManager` для получения модели
- ✅ Fallback на переменные окружения если конфигурация недоступна

#### `cmd/classify_catalog_items/main.go`
- ✅ Использует `WorkerConfigManager` для получения модели
- ✅ Fallback на переменные окружения если конфигурация недоступна

#### `cmd/classify_nomenclature/main.go`
- ✅ Использует `WorkerConfigManager` для получения модели
- ✅ Fallback на переменные окружения если конфигурация недоступна

#### `cmd/normalize/main.go`
- ✅ Использует `WorkerConfigManager` для получения модели и API ключа
- ✅ Fallback на переменные окружения если конфигурация недоступна

### 4. Classification & Nomenclature Libraries (Go)

#### `classification/ai_classifier.go`
- ✅ Улучшена функция `NewAIClassifier()` для использования переменной окружения `ARLIAI_MODEL`
- ✅ Fallback на дефолтную модель "GLM-4.5-Air" только если модель не указана

#### `nomenclature/config.go`
- ✅ Улучшена функция `DefaultConfig()` для использования переменной окружения `ARLIAI_MODEL`
- ✅ Fallback на дефолтную модель "GLM-4.5-Air" только если модель не указана

---

## Алгоритм выбора модели

Теперь во всех местах используется единый алгоритм:

1. **Приоритет 1**: Получение активной модели из `WorkerConfigManager`
   - Используется активный провайдер
   - Выбирается модель с наивысшим приоритетом (наименьший номер приоритета)

2. **Приоритет 2**: Дефолтная модель из конфигурации `WorkerConfigManager`
   - Используется `default_model` из конфигурации

3. **Приоритет 3**: Переменная окружения `ARLIAI_MODEL`
   - Если `WorkerConfigManager` недоступен или модель не найдена

4. **Приоритет 4**: Жестко заданная дефолтная модель
   - "GLM-4.5-Air" для большинства случаев
   - "gpt-4o-mini" для `ClientNormalizer` (последний fallback)

---

## Что осталось без изменений (и это правильно)

### Тестовые моки
- ✅ `normalization/ai_mock.go` - моки для тестов (это нормально)
- ✅ `normalization/ai_mock_test.go` - тесты для моков (это нормально)

### Дефолтные значения в конфигурации
- ✅ `server/worker_config.go` - дефолтная модель "GLM-4.5-Air" в инициализации (это нормально)
- ✅ `server/config.go` - дефолтная модель в конфигурации (это нормально)

### Fallback значения
- ✅ Все функции имеют fallback на дефолтные значения, если конфигурация недоступна (это правильно)

---

## Преимущества изменений

1. **Единая точка конфигурации**: Все модели выбираются через `WorkerConfigManager`
2. **Гибкость**: Модель можно изменить через веб-интерфейс без перезапуска сервера
3. **Приоритет моделей**: Система автоматически выбирает модель с наивысшим приоритетом
4. **Обратная совместимость**: Старые функции продолжают работать с fallback на переменные окружения
5. **Централизованное управление**: Все настройки моделей в одном месте

---

## Использование

### Через веб-интерфейс
1. Перейдите на страницу `/workers`
2. Выберите провайдер
3. Выберите модель и установите её как дефолтную
4. Модель будет использоваться во всех последующих запросах

### Через переменные окружения (fallback)
```bash
export ARLIAI_MODEL="GLM-4.5-Air"
export ARLIAI_API_KEY="your-api-key"
```

### Через API
```bash
curl -X POST http://localhost:9999/api/workers/config/update \
  -H "Content-Type: application/json" \
  -d '{
    "action": "set_default_model",
    "data": {
      "provider": "arliai",
      "model": "GLM-4.5"
    }
  }'
```

---

## Статистика изменений

- **Файлов изменено**: 13
- **Функций исправлено**: 15+
- **Добавлено новых функций**: 3
  - `getModelFromConfig()` в `server/server.go`
  - `GetModelAndAPIKey()` в `server/worker_config.go`
  - `NewClientNormalizerWithConfig()` в `normalization/client_normalizer.go`
- **Добавлено интерфейсов**: 1
  - `WorkerConfigManagerInterface` в `normalization/client_normalizer.go`
- **Улучшено библиотечных функций**: 2
  - `NewAIClassifier()` в `classification/ai_classifier.go`
  - `DefaultConfig()` в `nomenclature/config.go`

---

## Результат

✅ Все захардкоженные значения модели заменены на функциональный код  
✅ Система использует реальную конфигурацию из `WorkerConfigManager`  
✅ Сохранена обратная совместимость через fallback на переменные окружения  
✅ Модель можно изменять через веб-интерфейс без перезапуска сервера  
✅ Единая точка управления конфигурацией моделей

