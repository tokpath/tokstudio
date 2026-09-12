#!/usr/bin/env bash
# 本地 / 预览机：检查 Google OAuth 三件套形态，不打印 secret。
# 可选：用假 code 打 Google token 端点，区分 invalid_client（凭证错）与 invalid_grant（凭证对）。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ENV_FILE="${1:-.env}"
if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing $ENV_FILE" >&2
  exit 1
fi

id=""
secret=""
redirect=""

while IFS= read -r line || [[ -n "$line" ]]; do
  line="${line#"${line%%[![:space:]]*}"}"
  line="${line%"${line##*[![:space:]]}"}"
  [[ -z "$line" || "$line" == \#* ]] && continue
  case "$line" in
    TOKENHUB_GOOGLE_CLIENT_ID=*|TOKENHUB_GOOGLE_CLIENT_SECRET=*|TOKENHUB_GOOGLE_REDIRECT_URL=*)
      key="${line%%=*}"
      raw="${line#*=}"
      raw="${raw#"${raw%%[![:space:]]*}"}"
      raw="${raw%"${raw##*[![:space:]]}"}"
      if [[ ${#raw} -ge 2 ]]; then
        first="${raw:0:1}"
        last="${raw: -1}"
        if [[ ( "$first" == '"' && "$last" == '"' ) || ( "$first" == "'" && "$last" == "'" ) ]]; then
          raw="${raw:1:${#raw}-2}"
          raw="${raw#"${raw%%[![:space:]]*}"}"
          raw="${raw%"${raw##*[![:space:]]}"}"
        fi
      fi
      case "$key" in
        TOKENHUB_GOOGLE_CLIENT_ID) id="$raw" ;;
        TOKENHUB_GOOGLE_CLIENT_SECRET) secret="$raw" ;;
        TOKENHUB_GOOGLE_REDIRECT_URL) redirect="$raw" ;;
      esac
      ;;
  esac
done <"$ENV_FILE"

echo "env_file=$ENV_FILE"
echo "client_id_set=$([ -n "$id" ] && echo true || echo false)"
if [[ -n "$id" && ${#id} -gt 32 ]]; then
  echo "client_id_suffix=${id: -32}"
else
  echo "client_id_suffix=${id}"
fi
echo "secret_set=$([ -n "$secret" ] && echo true || echo false)"
echo "secret_len=${#secret}"
if [[ "$secret" == GOCSPX-* ]]; then
  echo "secret_looks_web=true"
else
  echo "secret_looks_web=false"
fi
if [[ "$secret" =~ [[:space:]] ]]; then
  echo "secret_has_space=true"
else
  echo "secret_has_space=false"
fi
echo "redirect=$redirect"

if [[ -z "$id" || -z "$secret" || -z "$redirect" ]]; then
  echo "result=triad_incomplete"
  exit 2
fi

probe="${GOOGLE_OAUTH_PROBE:-1}"
if [[ "$probe" != "1" ]]; then
  echo "result=triad_present_probe_skipped"
  exit 0
fi

body=$(curl -sS -X POST 'https://oauth2.googleapis.com/token' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode "code=4/fake-invalid-code-for-probe" \
  --data-urlencode "client_id=${id}" \
  --data-urlencode "client_secret=${secret}" \
  --data-urlencode "redirect_uri=${redirect}" \
  --data-urlencode 'grant_type=authorization_code' || true)

err=$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("error",""))' <<<"$body" 2>/dev/null || true)
desc=$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("error_description","")[:120])' <<<"$body" 2>/dev/null || true)
echo "google_token_error=${err:-parse_failed}"
echo "google_token_error_description=${desc}"

case "$err" in
  invalid_grant)
    echo "result=credentials_accepted_by_google"
    exit 0
    ;;
  invalid_client)
    echo "result=credentials_rejected_invalid_client"
    exit 3
    ;;
  *)
    echo "result=unexpected_google_error"
    exit 4
    ;;
esac
