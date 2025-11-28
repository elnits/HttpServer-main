#!/bin/bash
for i in $(seq 1 30); do
  echo "=== Проверка $i ==="
  curl -s http://localhost:9999/api/normalization/status | grep -E '"(isRunning|processed|total|currentStep)"'
  
  RUNNING=$(curl -s http://localhost:9999/api/normalization/status | grep -o '"isRunning":true')
  if [ -z "$RUNNING" ]; then
    echo "Завершено!"
    break
  fi
  
  sleep 2
done
