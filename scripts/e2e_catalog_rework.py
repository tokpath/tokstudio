#!/usr/bin/env python3
"""Exercise model publish -> route -> channel grant against an isolated API database.

Set TOKENHUB_TEST_API_BASE to the isolated API origin. Bootstrap tokens are read
from the environment or the repository .env file and never printed.
"""

import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


BASE = os.environ["TOKENHUB_TEST_API_BASE"].rstrip("/")
ENV_FILE = Path(__file__).resolve().parents[1] / ".env"


def config(name: str) -> str:
    if os.environ.get(name):
        return os.environ[name]
    for line in ENV_FILE.read_text().splitlines():
        if line.startswith(name + "="):
            return line.split("=", 1)[1].strip().strip('"').strip("'")
    raise RuntimeError(f"missing {name}")


ADMIN = config("TOKENHUB_BOOTSTRAP_ADMIN_TOKEN")
USER = config("TOKENHUB_BOOTSTRAP_USER_TOKEN")


def request(method: str, path: str, payload=None, token=""):
    body = None if payload is None else json.dumps(payload).encode()
    headers = {"Content-Type": "application/json", "X-Tokenhub-Confirm": "1"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(BASE + path, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as error:
        return error.code, json.load(error)


def expect(method, path, status, payload=None, token=ADMIN):
    code, data = request(method, path, payload, token)
    assert code == status, f"{method} {path}: wanted {status}, got {code}: {data}"
    return data


def public_has(model_id: str) -> bool:
    data = expect("GET", "/v1/public/models?id=" + urllib.parse.quote(model_id, safe=""), 200, token="")
    return any(item["id"] == model_id for item in data.get("items", []))


model_id = f"test/rework-{int(time.time() * 1000)}"
item = expect("POST", "/admin/models", 201, {
    "public_id": model_id, "vendor": "test", "display_name": "Catalog Rework",
    "capabilities": {"kind": "text"},
    "initial_price": {"input": "0.000001", "output": "0.000002", "currency": "USD"},
})["item"]
assert item["status"] == "draft"
expect("POST", "/admin/routes", 409, {
    "public_model_id": model_id, "status": "active",
    "candidates": [{"provider_id": "echo-primary", "upstream_model_id": "echo-upstream"}],
})
expect("POST", "/admin/models/publish", 200, {"public_id": model_id})
assert not public_has(model_id), "publish must not expose model"

unpriced = expect("POST", "/admin/providers", 201, {
    "name": "Unpriced Rework", "slug": "unpriced-" + str(int(time.time() * 1000)), "adapter": "test",
})["item"]
expect("POST", "/admin/routes", 409, {
    "public_model_id": model_id, "status": "active",
    "candidates": [{"provider_id": unpriced["id"], "upstream_model_id": "unknown"}],
})

route = expect("POST", "/admin/routes", 201, {
    "public_model_id": model_id, "strategy": "priority",
    "candidates": [{"provider_id": "echo-primary", "upstream_model_id": "echo-upstream"}],
})["item"]
assert route["status"] == "inactive"
route_id = route["id"]
assert expect("GET", "/admin/routes/" + route_id, 200)["item"]["candidates"][0]["upstream_model_id"] == "echo-upstream"
expect("PATCH", "/admin/channels/chn_official_a/models", 409, {
    "items": [{"public_id": model_id, "enabled": True, "wholesale": {"input": "0.0000007", "output": "0.0000014"}}],
})
expect("PATCH", "/admin/routes/" + route_id, 200, {"status": "active"})
assert not public_has(model_id), "route without channel grant must stay hidden"
expect("PATCH", "/admin/channels/chn_official_a/models", 200, {
    "items": [{"public_id": model_id, "enabled": True, "wholesale": {"input": "0.0000007", "output": "0.0000014"}}],
})
assert public_has(model_id), "model with route and grant must be visible"

key = expect("POST", "/v1/me/api-keys", 201, {"name": "catalog-rework"}, USER)["item"]["key"]
models = expect("GET", "/v1/models", 200, token=key).get("data", [])
assert any(model["id"] == model_id for model in models), "API key catalog must include model"
redeem_status, redeem_body = request("POST", "/v1/topups/redeem", {"code": "THE2E"}, USER)
assert redeem_status in (201, 409), f"redeem failed: {redeem_status} {redeem_body}"
# The normal development API deliberately has no live upstream or test adapter;
# its route reaches the adapter layer and returns 503. A disabled route returns 403 earlier.
chat = expect("POST", "/v1/chat/completions", 503, {
    "model": model_id, "messages": [{"role": "user", "content": "catalog-rework"}],
}, key)
assert chat.get("error", {}).get("code") == "provider_unavailable"

expect("PATCH", "/admin/routes/" + route_id, 200, {"status": "inactive"})
assert not public_has(model_id), "disabled route must hide model"
expect("POST", "/v1/chat/completions", 403, {
    "model": model_id, "messages": [{"role": "user", "content": "disabled"}],
}, key)
expect("POST", "/admin/models/deprecate", 200, {"public_id": model_id})
expect("POST", "/admin/models/publish", 200, {"public_id": model_id})
assert expect("GET", "/admin/routes/" + route_id, 200)["item"]["status"] == "inactive"
assert not public_has(model_id), "republish must not enable old route"
print("catalog rework flow passed:", model_id, route_id)
