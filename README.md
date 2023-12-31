# EscarBot
Telegram bot for EarthBound Café!

## Instructions

First of all, you should set up some environment variables:
```
cp .env.example .env
nano .env
```

The following variables are required:
* `BOT_TOKEN`
* `CHANNEL_ID`
* `GROUP_ID`
* `ADMIN_ID`

### Run with Docker
Just run:
```
docker-compose -f docker-compose.yaml up -d
```

## Test and debug locally
```
go test -v ./...
go run .
```
