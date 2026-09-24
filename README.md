### traiding_app

Консольный дашборд для расчёта волатильности акций, облигаций и крипто-фьючерсов.

## Установка

1. Склонируйте репозиторий.
2. Скопируйте `.env.example` в `.env` и укажите ваш Finam API-токен.
3. Выполните `go mod download`.
4. Запустите: `go run ./cmd/dashboard`.

## Использование

- F5 — обновить данные.
- Внизу — строка поиска, введите тикер или название.
- Enter — поиск, выбор инструмента в модальном окне.
- Ctrl+C — выход.

## Сборка

Для Linux: `GOOS=linux GOARCH=amd64 go build -o bin/dashboard ./cmd/dashboard`
Для Windows: `GOOS=windows GOARCH=amd64 go build -o bin/dashboard.exe ./cmd/dashboard`
