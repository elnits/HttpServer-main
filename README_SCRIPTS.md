# Скрипты запуска

## Для Windows (CMD)

### Запуск обоих серверов
```cmd
start-all.bat
```

### Запуск только бэкенда
```cmd
start-backend.bat
```

### Запуск только фронтенда
```cmd
start-frontend.bat
```

## Для PowerShell

### Запуск обоих серверов
```powershell
.\start-all.ps1
```

### Проверка статуса серверов
```powershell
.\check-status.ps1
```

## Для npm (в одном терминале)

### Запуск обоих серверов
```bash
cd frontend
npm run dev:with-backend
```

## Устранение проблем

### Ошибка "The term 'start-all.bat' is not recognized"

**В PowerShell:**
- Используйте `.\start-all.bat` вместо `start-all.bat`
- Или используйте PowerShell скрипт: `.\start-all.ps1`

**В CMD:**
- Убедитесь, что вы в правильной директории
- Используйте полный путь или `cd` в директорию проекта

### Ошибка с командами `start` или `timeout`

- В PowerShell используйте `Start-Process` вместо `start`
- В PowerShell используйте `Start-Sleep` вместо `timeout`
- Или используйте `.bat` файлы в CMD, а не в PowerShell

### Рекомендации

1. **Для CMD:** Используйте `.bat` файлы
2. **Для PowerShell:** Используйте `.ps1` файлы или запускайте `.bat` с `.\`
3. **Для автоматизации:** Используйте `npm run dev:with-backend` в директории `frontend`

