# Интеграция переклассификации с фронтендом

## ✅ Реализовано

### 1. API Endpoints для переклассификации

Созданы 4 новых API endpoint в `server/server_reclassification.go`:

1. **POST `/api/reclassification/start`** - Запуск переклассификации
2. **GET `/api/reclassification/events`** - SSE поток событий в реальном времени
3. **GET `/api/reclassification/status`** - Получение текущего статуса
4. **POST `/api/reclassification/stop`** - Остановка процесса

### 2. Real-time обновления через SSE

- События отправляются через Server-Sent Events (SSE)
- Логи обновляются в реальном времени
- Прогресс отслеживается автоматически

### 3. Детальная статистика

Статус включает:
- Прогресс в процентах
- Количество обработанных/успешных/ошибочных записей
- Скорость обработки (записей/сек)
- Прошедшее время
- Текущий шаг
- История логов (до 1000 последних)

## Использование

### Запуск переклассификации

```javascript
// Запуск с фронтенда
const response = await fetch('http://localhost:9999/api/reclassification/start', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    classifier_id: 1,
    strategy_id: 'top_priority',
    limit: 100  // 0 = без лимита
  })
});
```

### Подключение к событиям (SSE)

```javascript
const eventSource = new EventSource('http://localhost:9999/api/reclassification/events');

eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data);
  if (data.type === 'log') {
    console.log(data.message);
    // Обновить UI
  }
};
```

### Получение статуса

```javascript
const response = await fetch('http://localhost:9999/api/reclassification/status');
const status = await response.json();

console.log(`Прогресс: ${status.progress}%`);
console.log(`Обработано: ${status.processed}/${status.total}`);
console.log(`Скорость: ${status.rate} записей/сек`);
```

### Остановка процесса

```javascript
await fetch('http://localhost:9999/api/reclassification/stop', {
  method: 'POST'
});
```

## Пример React компонента

См. `RECLASSIFICATION_API.md` для полного примера React компонента с:
- Кнопками запуска/остановки
- Отображением прогресса
- Логами в реальном времени
- Статистикой

## Особенности

1. **Асинхронное выполнение** - Не блокирует API
2. **Real-time обновления** - SSE события
3. **Детальная статистика** - Полная информация о процессе
4. **Возможность остановки** - Graceful shutdown
5. **Логирование** - Все события сохраняются

## Документация

- `RECLASSIFICATION_API.md` - Полная документация API
- `server/server_reclassification.go` - Реализация endpoints

