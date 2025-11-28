# Тестирование извлечения атрибутов

## Быстрый тест

Для проверки работы системы извлечения атрибутов можно использовать следующие примеры:

### Пример 1: Размеры с материалом
**Входное название:**
```
wbc00z0002 панель isowall box 100x100 120mm сталь белый
```

**Ожидаемые атрибуты:**
- `article_code`: wbc00z0002
- `dimension` (width): 100
- `dimension` (height): 100
- `numeric_value` (thickness): 120 (mm)
- `text_value` (material): сталь
- `text_value` (color): белый
- `text_value` (type): панель

### Пример 2: Технический код с параметрами
**Входное название:**
```
ER-00013004 50kg 220v
```

**Ожидаемые атрибуты:**
- `technical_code`: ER-00013004
- `numeric_value` (weight): 50 (kg)
- `numeric_value` (electrical): 220 (v)

### Пример 3: Артикул с размерами
**Входное название:**
```
wb500z0002 профиль 200x300 5mm
```

**Ожидаемые атрибуты:**
- `article_code`: wb500z0002
- `dimension` (width): 200
- `dimension` (height): 300
- `numeric_value` (thickness): 5 (mm)
- `text_value` (type): профиль

## Проверка через API

### 1. Запустите нормализацию
```bash
curl -X POST http://localhost:8080/api/normalize/start \
  -H "Content-Type: application/json" \
  -d '{"use_ai": false}'
```

### 2. Проверьте статус
```bash
curl http://localhost:8080/api/normalization/status
```

### 3. Получите группу с элементами
```bash
curl "http://localhost:8080/api/normalization/group-items?normalized_name=панель&category=строительные%20материалы&include_ai=true"
```

### 4. Получите атрибуты конкретного элемента
```bash
curl http://localhost:8080/api/normalization/item-attributes/123
```

## Проверка в БД

После нормализации можно проверить сохраненные атрибуты:

```sql
-- Количество атрибутов по типам
SELECT attribute_type, COUNT(*) as count 
FROM normalized_item_attributes 
GROUP BY attribute_type;

-- Атрибуты для конкретного товара
SELECT 
    a.attribute_type,
    a.attribute_name,
    a.attribute_value,
    a.unit,
    a.original_text,
    a.confidence
FROM normalized_item_attributes a
WHERE a.normalized_item_id = 123;

-- Товары с наибольшим количеством атрибутов
SELECT 
    nd.id,
    nd.source_name,
    nd.normalized_name,
    COUNT(a.id) as attr_count
FROM normalized_data nd
LEFT JOIN normalized_item_attributes a ON a.normalized_item_id = nd.id
GROUP BY nd.id
ORDER BY attr_count DESC
LIMIT 10;
```

## Проверка на фронтенде

1. Откройте `/results`
2. Выберите группу с товарами
3. Раскройте элемент (нажмите на стрелку ▼)
4. Проверьте секцию "Извлеченные реквизиты"
5. Убедитесь, что все атрибуты отображаются корректно

## Возможные проблемы

### Атрибуты не сохраняются
- Проверьте логи сервера на наличие ошибок
- Убедитесь, что таблица `normalized_item_attributes` создана
- Проверьте, что `ExtractAttributes` вызывается в процессе нормализации

### Атрибуты не отображаются на фронтенде
- Проверьте, что API возвращает атрибуты в ответе
- Откройте DevTools и проверьте Network tab
- Убедитесь, что интерфейс `GroupItem` включает поле `attributes`

### Дублирование атрибутов
- Система должна автоматически предотвращать дублирование
- Если дубликаты все же появляются, проверьте логику в `ExtractAttributes`

