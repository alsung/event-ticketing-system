.PHONY: build up down clean logs ps migrate seed smoke reset

DB_URL ?= postgres://admin:password@localhost:5433/event_ticketing?sslmode=disable

## build: build all service images
build:
	docker compose build

## up: start the full stack (Postgres, migrations, four services)
up:
	docker compose up -d --build

## down: stop all services, preserving the database volume
down:
	docker compose down

## clean: stop all services and delete the database volume
clean:
	docker compose down -v

## logs: tail logs from all services
logs:
	docker compose logs -f

## ps: show service status
ps:
	docker compose ps

## migrate: re-run migrations (they also run automatically on `up`)
migrate:
	docker compose run --rm migrate

## seed: reset event/ticket data to a known fixture (1 event, 50 tickets)
seed:
	docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U admin -d event_ticketing < scripts/seed.sql

## smoke: end-to-end check of the full user journey through the gateway
smoke:
	./tests/smoke/smoke.sh

## reset: tear everything down and rebuild from scratch, seeded and verified
reset: clean up
	@echo "waiting for services to settle..."
	@sleep 5
	@$(MAKE) seed
	@$(MAKE) smoke
