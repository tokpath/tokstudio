#!/usr/bin/env bash
# Read-only verification against the actual brand gate and optional TLS endpoint.
# A local CA handshake is not public DNS or public certificate validation.
set -euo pipefail
: "${TOKENHUB_TLS_API_URL:?set the running API origin}"
: "${TOKENHUB_TLS_SERVER_NAME:?set an existing verified brand hostname}"
known="$(curl --fail-with-body --silent --show-error --get --data-urlencode "domain=$TOKENHUB_TLS_SERVER_NAME" "$TOKENHUB_TLS_API_URL/v1/public/tls-check" -o /dev/null -w '%{http_code}')"
[[ "$known" == 200 ]] || { echo 'registered hostname was refused' >&2; exit 1; }
unknown="$(curl --silent --show-error --get --data-urlencode "domain=tokenhub-unregistered-$RANDOM-$RANDOM.invalid" "$TOKENHUB_TLS_API_URL/v1/public/tls-check" -o /dev/null -w '%{http_code}')"
[[ "$unknown" == 403 || "$unknown" == 404 ]] || { echo 'unregistered hostname gate failed' >&2; exit 1; }
echo 'Actual hostname gate passed: registered allowed, unknown refused.'
if [[ -n "${TOKENHUB_TLS_CONNECT_ADDR:-}" ]]; then
  : "${TOKENHUB_TLS_CA_FILE:?provide the trusted CA file; TLS verification is never disabled}"
  python3 - <<'PY'
import os, socket, ssl
from urllib.parse import urlsplit
endpoint = urlsplit('//'+os.environ['TOKENHUB_TLS_CONNECT_ADDR'])
assert endpoint.hostname and endpoint.port and not endpoint.path
context = ssl.create_default_context(cafile=os.environ['TOKENHUB_TLS_CA_FILE'])
with socket.create_connection((endpoint.hostname, endpoint.port), timeout=10) as connection:
    with context.wrap_socket(connection, server_hostname=os.environ['TOKENHUB_TLS_SERVER_NAME']) as secured:
        print('Verified TLS version:', secured.version())
        print('Certificate expiry:', secured.getpeercert()['notAfter'])
PY
  echo 'TLS chain and hostname verified against the supplied CA.'
fi
