.PHONY: api worker migrate test e2e-m0 e2e-m1 e2e-m2 e2e-m3 web tidy

api:
	cd backend && go run ./cmd/api

worker:
	cd backend && go run ./cmd/worker

migrate:
	cd backend && go run ./cmd/migrate

test:
	cd backend && go test ./...
	cd web && npm test

tidy:
	cd backend && go mod tidy

web:
	cd web && npm run dev

e2e-m0:
	bash scripts/e2e_m0.sh

e2e-m1:
	bash scripts/e2e_m1.sh

e2e-m2:
	bash scripts/e2e_m2.sh

e2e-m3:
	bash scripts/e2e_m3.sh
