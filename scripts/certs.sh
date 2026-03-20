#!/usr/bin/env bash
# scripts/certs.sh — generate the PKI material required by the stack.
#
# Outputs (all in deployments/certs/, names must match deployments/.env):
#   ca.key            Root CA private key      (keep secret, not committed)
#   ca.pem            Root CA certificate
#   server-key.pem    Server private key       (NATS + InfluxDB)
#   server.pem        Server certificate       (SAN: nats, influxdb, localhost, 127.0.0.1)
#   client-key.pem    Client private key       (Go microservices mTLS)
#   client.pem        Client certificate
#   admin-token.json  InfluxDB offline token   (loaded via --admin-token-file at startup)
#
# Side-effects:
#   Updates INFLUX_TOKEN and INFLUX_SESSION_SECRET in deployments/.env
#   to match the newly generated token (override with ENV_FILE=<path>).
#
# Usage:
#   bash scripts/certs.sh          # generate (fails if certs already exist)
#   FORCE=1 bash scripts/certs.sh  # regenerate unconditionally
set -euo pipefail


CERT_DIR="${CERT_DIR:-./deployments/certs}"
CA_DAYS="${CA_DAYS:-3650}"    # Root CA validity: 10 years
LEAF_DAYS="${LEAF_DAYS:-365}" # Leaf cert validity: 1 year
KEY_BITS="${KEY_BITS:-2048}"


if [[ -f "$CERT_DIR/ca.pem" && "${FORCE:-0}" != "1" ]]; then
  echo "ERROR: certificates already exist in $CERT_DIR." >&2
  echo "       Run with FORCE=1 to regenerate." >&2
  exit 1
fi

TEMP_FILES=()

cleanup() {
  local exit_code=$?
  if [[ $exit_code -ne 0 ]]; then
    echo "ERROR: certificate generation failed — removing partial artefacts." >&2
    rm -f "${TEMP_FILES[@]}" 2>/dev/null || true
  fi
  exit "$exit_code"
}
trap cleanup EXIT

mkdir -p "$CERT_DIR"

rm -f "$CERT_DIR/ca.srl"

# 1. Root CA 

echo "==> 1. Generating Root CA"

TEMP_FILES+=("$CERT_DIR/ca.key" "$CERT_DIR/ca.pem")

openssl genrsa -out "$CERT_DIR/ca.key" "$KEY_BITS" 2>/dev/null

openssl req -x509 -new -nodes \
  -key  "$CERT_DIR/ca.key" \
  -sha256 -days "$CA_DAYS" \
  -out  "$CERT_DIR/ca.pem" \
  -subj "/CN=deltaflare-root-ca"

# 2. Server certificate (NATS + InfluxDB) 

echo "==> 2. Generating server certificate (SAN: nats, influxdb, localhost)"

SERVER_CNF="$(mktemp)"
TEMP_FILES+=("$SERVER_CNF" "$CERT_DIR/server-key.pem" "$CERT_DIR/server.pem")

cat > "$SERVER_CNF" <<'EOF'
[req]
distinguished_name = req
prompt             = no

[req_ext]
subjectAltName = DNS:nats,DNS:influxdb,DNS:localhost,IP:127.0.0.1
EOF

openssl genrsa -out "$CERT_DIR/server-key.pem" "$KEY_BITS" 2>/dev/null

SERVER_CSR="$(mktemp)"
TEMP_FILES+=("$SERVER_CSR")

openssl req -new \
  -key    "$CERT_DIR/server-key.pem" \
  -out    "$SERVER_CSR" \
  -subj   "/CN=deltaflare-server"


openssl x509 -req \
  -in         "$SERVER_CSR" \
  -CA         "$CERT_DIR/ca.pem" \
  -CAkey      "$CERT_DIR/ca.key" \
  -set_serial 01 \
  -out        "$CERT_DIR/server.pem" \
  -days       "$LEAF_DAYS" \
  -sha256 \
  -extfile    "$SERVER_CNF" \
  -extensions req_ext \
  2>/dev/null

rm -f "$SERVER_CSR" "$SERVER_CNF"

# 3. Client certificate (Go microservices mTLS) 

echo "==> 3. Generating client certificate (mTLS)"

CLIENT_EXT="$(mktemp)"
TEMP_FILES+=("$CLIENT_EXT" "$CERT_DIR/client-key.pem" "$CERT_DIR/client.pem")

# RFC 5280 §4.2.1.12 — extendedKeyUsage: clientAuth
cat > "$CLIENT_EXT" <<'EOF'
[req_ext]
extendedKeyUsage = clientAuth
EOF

openssl genrsa -out "$CERT_DIR/client-key.pem" "$KEY_BITS" 2>/dev/null

CLIENT_CSR="$(mktemp)"
TEMP_FILES+=("$CLIENT_CSR")

openssl req -new \
  -key  "$CERT_DIR/client-key.pem" \
  -out  "$CLIENT_CSR" \
  -subj "/CN=deltaflare-microservice"

openssl x509 -req \
  -in         "$CLIENT_CSR" \
  -CA         "$CERT_DIR/ca.pem" \
  -CAkey      "$CERT_DIR/ca.key" \
  -set_serial 02 \
  -out        "$CERT_DIR/client.pem" \
  -days       "$LEAF_DAYS" \
  -sha256 \
  -extfile    "$CLIENT_EXT" \
  -extensions req_ext \
  2>/dev/null

rm -f "$CLIENT_CSR" "$CLIENT_EXT"

# 4. InfluxDB admin token 


echo "==> 4. Generating InfluxDB admin token"

INFLUX_TOKEN="apiv3_$(openssl rand -hex 32)"
TEMP_FILES+=("$CERT_DIR/admin-token.json")

cat > "$CERT_DIR/admin-token.json" <<EOF
{
  "token": "$INFLUX_TOKEN",
  "name": "admin-token",
  "description": "Admin token for automated deployment"
}
EOF

ENV_FILE="${ENV_FILE:-./deployments/.env}"
if [[ -f "$ENV_FILE" ]]; then
  INFLUX_SESSION_SECRET="$(openssl rand -hex 32)"
  tmp="$(mktemp)"
  sed \
    -e "s|^INFLUX_TOKEN=.*|INFLUX_TOKEN=$INFLUX_TOKEN|" \
    -e "s|^INFLUX_SESSION_SECRET=.*|INFLUX_SESSION_SECRET=$INFLUX_SESSION_SECRET|" \
    "$ENV_FILE" > "$tmp"
  mv "$tmp" "$ENV_FILE"
  echo "  Synced INFLUX_TOKEN and INFLUX_SESSION_SECRET → $ENV_FILE"
else
  echo "  WARNING: $ENV_FILE not found — set INFLUX_TOKEN=$INFLUX_TOKEN manually." >&2
fi



chmod 600 "$CERT_DIR/ca.key" "$CERT_DIR/server-key.pem" \
          "$CERT_DIR/client-key.pem" "$CERT_DIR/admin-token.json"
chmod 644 "$CERT_DIR/ca.pem" "$CERT_DIR/server.pem" "$CERT_DIR/client.pem"


echo "==> 5. Verifying certificate chain"

openssl verify -CAfile "$CERT_DIR/ca.pem" "$CERT_DIR/server.pem" > /dev/null
openssl verify -CAfile "$CERT_DIR/ca.pem" "$CERT_DIR/client.pem" > /dev/null

echo ""
echo "  ca.pem            Root CA certificate"
echo "  ca.key            Root CA private key     (secret — never commit)"
echo "  server.pem        Server certificate      (SAN: nats, influxdb, localhost)"
echo "  server-key.pem    Server private key      (secret — never commit)"
echo "  client.pem        Client certificate      (mTLS)"
echo "  client-key.pem    Client private key      (secret — never commit)"
echo "  admin-token.json  InfluxDB offline token  (secret — never commit)"
echo ""
echo "  All artefacts generated in: $CERT_DIR"

