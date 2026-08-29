.PHONY: build up up-debug down clean logs ps migrate seed smoke reset go-build go-test go-test-db tidy deps-check load load-idem load-capacity

DB_URL ?= postgres://admin:password@localhost:5433/event_ticketing?sslmode=disable

## build: build all service images
build:
	docker compose build

## up: start the full stack (Postgres, migrations, four services)
up:
	docker compose up -d --build

## up-debug: same, but republish the internal service ports on 8081-8083
up-debug:
	docker compose -f docker-compose.yml -f docker-compose.debug.yml up -d --build

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

## load: reseed, race 100 VUs for 50 tickets, then verify the database state
## Runs k6 in Docker so no local install is needed.
load: seed
	docker run --rm -i -v "$(PWD)/tests/load:/scripts:ro" \
		-e GATEWAY=http://host.docker.internal:8000 \
		--add-host=host.docker.internal:host-gateway \
		grafana/k6:latest run /scripts/purchase_contention.js
	./tests/load/verify.sh

## load-idem: fire many concurrent purchases sharing one idempotency key
load-idem: seed
	docker run --rm -i -v "$(PWD)/tests/load:/scripts:ro" \
		-e GATEWAY=http://host.docker.internal:8000 \
		--add-host=host.docker.internal:host-gateway \
		grafana/k6:latest run /scripts/idempotency.js

## load-capacity: ramp arrival rate to find where latency degrades
load-capacity: seed
	docker run --rm -i -v "$(PWD)/tests/load:/scripts:ro" \
		-e GATEWAY=http://host.docker.internal:8000 \
		--add-host=host.docker.internal:host-gateway \
		grafana/k6:latest run /scripts/capacity.js

## reset: tear everything down and rebuild from scratch, seeded and verified
reset: clean up
	@echo "waiting for services to settle..."
	@sleep 5
	@$(MAKE) seed
	@$(MAKE) smoke

# --- Go workspace -----------------------------------------------------------
# The workspace root is not itself a module, so a bare ./... does not resolve.
# Every target below names the modules explicitly.
GO_MODULES := ./pkg/... ./api-gateway/... ./user-service/... ./event-service/... ./ticket-service/...

## go-build: compile every module
go-build:
	cd services && go build $(GO_MODULES)

## go-test: test every module
go-test:
	cd services && go test $(GO_MODULES)

## go-test-db: also run the store integration tests against the running Postgres
go-test-db:
	cd services && TEST_DATABASE_URL="$(DB_URL)" go test $(GO_MODULES)

## tidy: run go mod tidy in each module
tidy:
	@for m in pkg api-gateway user-service event-service ticket-service; do \
		echo "tidy $$m"; (cd services/$$m && go mod tidy) || exit 1; \
	done

## deps-check: fail if a shared dependency is pinned to different versions
deps-check:
	@echo "checking shared dependency versions agree across modules..."
	@fail=0; for dep in github.com/jackc/pgx/v5 github.com/golang-jwt/jwt/v5 github.com/google/uuid golang.org/x/crypto; do \
		vers=$$(grep -h "$$dep " services/*/go.mod | grep -v "^\s*//" | awk -v d="$$dep" '{for(i=1;i<=NF;i++) if($$i==d) print $$(i+1)}' | sort -u); \
		count=$$(echo "$$vers" | grep -c .); \
		if [ "$$count" -gt 1 ]; then \
			echo "  MISMATCH $$dep: $$(echo $$vers | tr '\n' ' ')"; fail=1; \
		else \
			[ -n "$$vers" ] && echo "  ok $$dep $$vers"; \
		fi; \
	done; \
	if [ "$$fail" = 1 ]; then echo "shared dependencies diverge"; exit 1; fi
