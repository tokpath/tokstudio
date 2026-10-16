.PHONY: api worker migrate test test-integration test-external web tidy assert-deploy

api:
	cd backend && go run ./cmd/api

worker:
	cd backend && go run ./cmd/worker

migrate:
	cd backend && go run ./cmd/migrate

test:
	cd backend && go test ./...
	cd web && npm test

# Supply isolated PostgreSQL and explicit external lab/S3/Redis endpoints.
test-integration:
	@test -n "$(TOKENHUB_DATABASE_URL)"
	@test -n "$(TOKENHUB_TEST_DATABASE_URL)"
	cd backend && go test -race -p 1 -count=1 ./...

# The standalone protocol service must be started separately. Fail visibly if
# the operator intended an external drill but forgot one of its real services.
test-external:
	@test -n "$$TOKENHUB_DATABASE_URL" && test -n "$$TOKENHUB_TEST_DATABASE_URL"
	@test -n "$$TOKENHUB_MODEL_MOCK_URL" && test -n "$$TOKENHUB_PAYMENT_MOCK_URL"
	@test -n "$$TOKENHUB_SMTP_MOCK_ADDR" && test -n "$$TOKENHUB_TEST_REDIS_URL"
	@test -n "$$TOKENHUB_TEST_S3_ENDPOINT" && test -n "$$TOKENHUB_TEST_S3_BUCKET"
	@test -n "$$TOKENHUB_TEST_S3_ACCESS_KEY" && test -n "$$TOKENHUB_TEST_S3_SECRET_KEY"
	cd backend && go test -race -p 1 -count=1 -timeout 15m ./...

tidy:
	cd backend && go mod tidy

web:
	cd web && npm run dev

assert-deploy:
	bash scripts/assert_deploy_isolation.sh
	bash scripts/refactor-assert-release.sh
	bash scripts/assert_minio_recovery.sh
