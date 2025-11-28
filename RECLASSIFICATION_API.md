# API для переклассификации с фронтенда

## Обзор

Создана система API endpoints для запуска переклассификации с фронтенда с возможностью отслеживания прогресса в реальном времени через Server-Sent Events (SSE).

## API Endpoints

### 1. Запуск переклассификации

**POST** `/api/reclassification/start`

**Request Body:**
```json
{
  "classifier_id": 1,
  "strategy_id": "top_priority",
  "limit": 100
}
```

**Параметры:**
- `classifier_id` (int, обязательный) - ID классификатора (обычно 1 для КПВЭД)
- `strategy_id` (string, опциональный) - Стратегия свертки категорий (по умолчанию: "top_priority")
- `limit` (int, опциональный) - Лимит записей для обработки (0 = без лимита)

**Response:**
```json
{
  "success": true,
  "message": "Переклассификация запущена",
  "classifier_id": 1,
  "strategy_id": "top_priority",
  "limit": 100
}
```

**Пример запроса:**
```bash
curl -X POST http://localhost:9999/api/reclassification/start \
  -H "Content-Type: application/json" \
  -d '{"classifier_id": 1, "strategy_id": "top_priority", "limit": 100}'
```

### 2. Получение событий в реальном времени (SSE)

**GET** `/api/reclassification/events`

Подключается к потоку событий через Server-Sent Events. События отправляются в формате:

```
data: {"type":"log","message":"🚀 Запуск переклассификации...","timestamp":"2025-01-16T10:00:00Z"}

data: {"type":"log","message":"📊 Обработано: 10/100 (успешно: 8, ошибок: 2)","timestamp":"2025-01-16T10:00:15Z"}
```

**Пример использования (JavaScript):**
```javascript
const eventSource = new EventSource('http://localhost:9999/api/reclassification/events');

eventSource.onmessage = (event) => {
  const data = JSON.parse(event.data);
  if (data.type === 'log') {
    console.log(data.message);
    // Обновить UI с логами
  }
};

eventSource.onerror = (error) => {
  console.error('SSE error:', error);
  eventSource.close();
};
```

### 3. Получение статуса

**GET** `/api/reclassification/status`

**Response:**
```json
{
  "isRunning": true,
  "progress": 45.5,
  "processed": 455,
  "total": 1000,
  "success": 420,
  "errors": 30,
  "skipped": 5,
  "currentStep": "📊 Обработано: 455/1000 (успешно: 420, ошибок: 30)",
  "logs": [
    "🚀 Запуск переклассификации...",
    "📋 Классификатор ID: 1",
    "✅ Классификатор загружен: КПВЭД (глубина: 6)",
    "..."
  ],
  "startTime": "2025-01-16T10:00:00Z",
  "elapsedTime": "2m15s",
  "rate": 3.4
}
```

**Поля:**
- `isRunning` (bool) - Выполняется ли процесс
- `progress` (float) - Прогресс в процентах (0-100)
- `processed` (int) - Количество обработанных записей
- `total` (int) - Общее количество записей
- `success` (int) - Успешно переклассифицировано
- `errors` (int) - Количество ошибок
- `skipped` (int) - Пропущено записей
- `currentStep` (string) - Текущий шаг
- `logs` (array) - Массив логов (последние 1000)
- `startTime` (string) - Время начала (RFC3339)
- `elapsedTime` (string) - Прошедшее время
- `rate` (float) - Скорость обработки (записей/сек)

**Пример запроса:**
```bash
curl http://localhost:9999/api/reclassification/status
```

### 4. Остановка процесса

**POST** `/api/reclassification/stop`

**Response:**
```json
{
  "success": true,
  "message": "Переклассификация остановлена"
}
```

**Пример запроса:**
```bash
curl -X POST http://localhost:9999/api/reclassification/stop
```

## Пример использования на фронтенде

### React компонент

```typescript
import { useState, useEffect, useRef } from 'react';

interface ReclassificationStatus {
  isRunning: boolean;
  progress: number;
  processed: number;
  total: number;
  success: number;
  errors: number;
  skipped: number;
  currentStep: string;
  logs: string[];
  startTime?: string;
  elapsedTime?: string;
  rate: number;
}

export function ReclassificationPage() {
  const [status, setStatus] = useState<ReclassificationStatus>({
    isRunning: false,
    progress: 0,
    processed: 0,
    total: 0,
    success: 0,
    errors: 0,
    skipped: 0,
    currentStep: 'Не запущено',
    logs: [],
    rate: 0,
  });
  const eventSourceRef = useRef<EventSource | null>(null);

  // Запуск переклассификации
  const startReclassification = async (classifierId: number, strategyId: string, limit?: number) => {
    try {
      const response = await fetch('http://localhost:9999/api/reclassification/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          classifier_id: classifierId,
          strategy_id: strategyId,
          limit: limit || 0,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to start reclassification');
      }

      // Подключаемся к SSE
      const eventSource = new EventSource('http://localhost:9999/api/reclassification/events');
      eventSourceRef.current = eventSource;

      eventSource.onmessage = (event) => {
        const data = JSON.parse(event.data);
        if (data.type === 'log') {
          setStatus((prev) => ({
            ...prev,
            logs: [...prev.logs.slice(-99), data.message],
            currentStep: data.message,
          }));
        }
      };

      eventSource.onerror = (error) => {
        console.error('SSE error:', error);
        eventSource.close();
      };

      // Периодически обновляем статус
      const statusInterval = setInterval(async () => {
        const statusResponse = await fetch('http://localhost:9999/api/reclassification/status');
        const statusData = await statusResponse.json();
        setStatus(statusData);
      }, 1000);

      return () => {
        clearInterval(statusInterval);
        eventSource.close();
      };
    } catch (error) {
      console.error('Error starting reclassification:', error);
    }
  };

  // Остановка переклассификации
  const stopReclassification = async () => {
    try {
      await fetch('http://localhost:9999/api/reclassification/stop', {
        method: 'POST',
      });
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
    } catch (error) {
      console.error('Error stopping reclassification:', error);
    }
  };

  // Очистка при размонтировании
  useEffect(() => {
    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
    };
  }, []);

  return (
    <div>
      <h1>Переклассификация с КПВЭД</h1>
      
      <div>
        <button 
          onClick={() => startReclassification(1, 'top_priority', 100)}
          disabled={status.isRunning}
        >
          Запустить (тест, 100 записей)
        </button>
        
        <button 
          onClick={() => startReclassification(1, 'top_priority')}
          disabled={status.isRunning}
        >
          Запустить (все записи)
        </button>
        
        <button 
          onClick={stopReclassification}
          disabled={!status.isRunning}
        >
          Остановить
        </button>
      </div>

      <div>
        <h2>Статус</h2>
        <p>Выполняется: {status.isRunning ? 'Да' : 'Нет'}</p>
        <p>Прогресс: {status.progress.toFixed(1)}%</p>
        <p>Обработано: {status.processed} / {status.total}</p>
        <p>Успешно: {status.success}</p>
        <p>Ошибок: {status.errors}</p>
        <p>Скорость: {status.rate.toFixed(1)} записей/сек</p>
        <p>Текущий шаг: {status.currentStep}</p>
      </div>

      <div>
        <h2>Логи</h2>
        <div style={{ maxHeight: '400px', overflow: 'auto' }}>
          {status.logs.map((log, index) => (
            <div key={index}>{log}</div>
          ))}
        </div>
      </div>
    </div>
  );
}
```

## Особенности

1. **Асинхронное выполнение** - Процесс запускается в отдельной горутине, не блокируя API
2. **Real-time обновления** - События отправляются через SSE в реальном времени
3. **Детальная статистика** - Полная информация о прогрессе, скорости, ошибках
4. **Возможность остановки** - Процесс можно остановить в любой момент
5. **Логирование** - Все события сохраняются в статусе (до 1000 последних)

## Безопасность

- Проверка на одновременный запуск (только один процесс может выполняться)
- Валидация входных параметров
- Graceful shutdown при остановке

## Производительность

- Скорость: ~1-2 записи/сек (зависит от API)
- Для 15973 записей: ~2-4 часа
- Рекомендуется запускать в фоновом режиме или порциями

