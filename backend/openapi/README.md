# TokenHub API contract

`tokenhub.json` is OpenAPI 3.1 for the new application. The old `/admin` and fixed-token contracts have been removed.

Direct API routes start with `/v1`; the browser calls `/api/v1` and the reverse proxy removes `/api`. Browser writes require the actual HttpOnly session, `X-CSRF-Token`, same origin and a UUID `Idempotency-Key`. Model requests use a brand-bound Bearer API Key and accept an optional stable request key up to150 characters. All financial amounts are decimal strings.

The contract includes every model protocol, authentication, core customer money/order/key/usage flows, invoice files, refund destination confirmation, and operator read workspaces. Browser response schemas come from `web/lib/contracts.ts`. Regenerate using the bundled or configured Node runtime:

```sh
node scripts/refactor-openapi.cjs
node scripts/refactor-openapi.cjs --check
```

Operator mutations are expanded as explicit domain request DTOs in the OpenAPI paths, including technical capabilities and price rules. `operator-operations.json` records the reviewed typed workflow metadata; `operator-inputs.cjs` supplies nested input conversion and explicit technical/specialized DTOs. Regeneration combines these inputs with browser output schemas. When a workflow or handler changes, update its metadata and reviewed schema, then regenerate. The UI may hide concurrency versions, but API clients must retain the current version from the authorized read response. Actor identity, merchant scope and permitted current state are always verified by the domain.

The typed workspace also returns permitted operations and row actions for a particular state. A command documented in OpenAPI is not automatically permitted for every row. Internal readiness, Caddy TLS issuance checks, worker endpoints and authorized metrics are deliberately outside the public business contract.

The model protocol accepts only configured verified capabilities and parameters. OpenAI-shaped response bodies vary by capability; streamed responses use SSE and binary media uses its actual content type. The gateway does not claim all OpenAI features or capabilities are available. Configuration and protocol-fixture success do not establish live provider or merchant acceptance.

The final generated contract contains198 paths,177 schemas and111 reviewed typed operator command definitions. `--check` generates in memory, validates every local reference, compares exact bytes, and exits nonzero for missing or stale output without writing files. Provider materials and fixture evidence are explicitly separate from live acceptance.
