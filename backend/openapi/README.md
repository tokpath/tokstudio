# TokenHub API contract

`tokenhub.json` is OpenAPI 3.1 for the new application. The old `/admin` and fixed-token contracts have been removed.

Direct API routes start with `/v1`; the browser calls `/api/v1` and the reverse proxy removes `/api`. Browser writes require the actual HttpOnly session, `X-CSRF-Token`, same origin and a UUID `Idempotency-Key`. Model requests use a brand-bound Bearer API Key and accept an optional stable request key up to150 characters. All financial amounts are decimal strings.

The contract includes every model protocol, authentication, core customer money/order/key/usage flows, invoice files, refund destination confirmation, and operator read workspaces. Browser response schemas come from `web/lib/contracts.ts`. Regenerate using the bundled or configured Node runtime:

```sh
node scripts/refactor-openapi.cjs
node scripts/refactor-openapi.cjs --check
node --test backend/openapi/contract.test.cjs
```

Operator mutations are expanded as explicit domain request DTOs in the OpenAPI paths, including technical capabilities and price rules. `operator-operations.json` is reviewed input, maintained against the actual portal workflow definitions; there is no database export step. `operator-inputs.cjs` supplies nested input conversion and explicit technical/specialized DTOs. When a workflow or handler changes, update its metadata and reviewed schema, then regenerate. The generator also checks mutation permissions against `permissionFor` and `operationPermission` in the portal sources. The UI may hide concurrency versions, but API clients must retain the current version from the authorized read response. Actor identity, merchant scope and permitted current state are always verified by the domain.

When the same named operation has distinct create and revision fields, metadata uses `variant` to preserve both schemas. Procurement authorization revisions require the original authorization ID and newly confirmed consent, while creation has no existing ID. Ordinary money inputs use `LedgerDecimal` with 18 fractional places; technical prices and response reading retain the separate 24-place `Decimal` contract.

Workflow URLs may contain a selected row query template. Generation separates that template from the OpenAPI path: appearance/Logo and coverage targets are `target_operator_id` query parameters, while domain requests select their target in the JSON body. Model references always return `listing_version`; an authorized direct-child `buyer_id` query also returns its `authorization_version`. These values belong to the selected operator/model relationship, not to a provider credential or a public model revision.

The typed workspace also returns permitted operations and row actions for a particular state. A command documented in OpenAPI is not automatically permitted for every row. Internal readiness, Caddy TLS issuance checks, worker endpoints and authorized metrics are deliberately outside the public business contract.

The model protocol accepts only configured verified capabilities and parameters. OpenAI-shaped response bodies vary by capability; streamed responses use SSE and binary media uses its actual content type. The gateway does not claim all OpenAI features or capabilities are available. Configuration and protocol-fixture success do not establish live provider or merchant acceptance.

`--check` generates in memory, validates local references, path parameters and mutation permissions, compares exact bytes, and exits nonzero for missing or stale output without writing files. The contract tests cover target placement, nullable theme inheritance, multipart Logo upload, real CAS revisions and the operator context DTOs. Provider materials and fixture evidence are explicitly separate from live acceptance.
