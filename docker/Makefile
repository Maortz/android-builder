.PHONY: up up-cron down build logs shell

# Start container (no cron)
up:
	docker compose up -d

# Start container with autonomous pipeline cron enabled (PowerShell)
up-cron:
	$env:ENABLE_CRON="1"; docker compose up -d

down:
	docker compose down

build:
	docker compose build

logs:
	docker compose logs -f

# Drop into a bash shell inside the running container
shell:
	docker compose exec claude bash
