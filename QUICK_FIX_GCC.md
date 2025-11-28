# Быстрое решение проблемы CGO

## Проблема
```
Binary was compiled with 'CGO_ENABLED=0', go-sqlite3 requires cgo to work
```

## Решение 1: Установить TDM-GCC (самый простой способ)

1. **Скачайте TDM-GCC:**
   - https://jmeubank.github.io/tdm-gcc/
   - Выберите версию для Windows (64-bit)

2. **Установите:**
   - Запустите установщик
   - Выберите "Add to PATH" при установке
   - Установите в стандартное место (обычно `C:\TDM-GCC-64\`)

3. **Перезапустите терминал** и запустите:
   ```bash
   start-backend.bat
   ```

## Решение 2: Использовать Docker (без установки GCC)

```bash
docker-compose up -d
```

Это запустит все сервисы в контейнерах, где GCC уже установлен.

## Решение 3: Установить MinGW-w64 вручную

1. Скачайте: https://sourceforge.net/projects/mingw-w64/
2. Распакуйте в `C:\mingw64\mingw64\bin\`
3. Добавьте в PATH:
   ```powershell
   [Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\mingw64\mingw64\bin", "User")
   ```
4. Перезапустите терминал

## Проверка установки

После установки GCC проверьте:
```bash
gcc --version
```

Должно вывести версию компилятора.

