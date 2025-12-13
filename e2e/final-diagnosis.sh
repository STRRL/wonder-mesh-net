#!/bin/bash

echo "=== Final Tailscale Diagnosis ==="

cd /Users/strrl/playground/GitHub/wonder-mesh-net/e2e

echo "[1] Starting services..."
docker compose up -d
sleep 10
docker restart coordinator
sleep 5

echo -e "\n[2] Getting authkey (full flow)..."
COOKIE_JAR=/tmp/cookies-final.txt
rm -f "$COOKIE_JAR"

LOGIN_REDIRECT=$(curl -s -I -c "$COOKIE_JAR" "http://localhost:9080/auth/login?provider=oidc" | grep -i "^location:" | sed 's/location: //i' | tr -d '\r')
LOGIN_PAGE=$(curl -s -c "$COOKIE_JAR" -b "$COOKIE_JAR" -L "$LOGIN_REDIRECT")
FORM_ACTION=$(echo "$LOGIN_PAGE" | sed -n 's/.*action="\([^"]*\)".*/\1/p' | head -1 | sed 's/&amp;/\&/g')
CALLBACK_RESPONSE=$(curl -s -c "$COOKIE_JAR" -b "$COOKIE_JAR" -L -d "username=testuser" -d "password=testpass" -w "\n%{url_effective}" "$FORM_ACTION")
SESSION=$(echo "$CALLBACK_RESPONSE" | tail -1 | sed -n 's/.*session=\([^&]*\).*/\1/p')
JOIN_TOKEN=$(curl -s -X POST -H "X-Session-Token: $SESSION" -H "Content-Type: application/json" -d '{"ttl": "1h"}' "http://localhost:9080/api/v1/join-token" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')

echo "Session: ${SESSION:0:40}..."
echo "Join token: ${JOIN_TOKEN:0:80}..."

echo -e "\n[3] Starting tailscaled in worker-1..."
docker compose exec -T -d worker-1 tailscaled --state=/data/tailscaled.state --socket=/var/run/tailscale/tailscaled.sock
sleep 3

echo -e "\n[4] Getting worker authkey..."
WORKER_API_RESPONSE=$(docker compose exec -T worker-1 sh -c "curl -s -X POST -H 'Content-Type: application/json' -d '{\"token\": \"$JOIN_TOKEN\"}' 'http://coordinator:9080/api/v1/worker/join'")
echo "API Response: $WORKER_API_RESPONSE"

AUTHKEY=$(echo "$WORKER_API_RESPONSE" | sed -n 's/.*"authkey":"\([^"]*\)".*/\1/p')
LOGIN_SERVER=$(echo "$WORKER_API_RESPONSE" | sed -n 's/.*"headscale_url":"\([^"]*\)".*/\1/p' | sed 's/localhost/coordinator/g')

echo "Authkey: ${AUTHKEY:0:40}..."
echo "Login server (rewritten): $LOGIN_SERVER"

echo -e "\n[5] Running tailscale up (no timeout, capturing full output)..."
docker compose exec -T worker-1 sh -c "tailscale up --reset --authkey='$AUTHKEY' --login-server='$LOGIN_SERVER' --accept-routes --accept-dns=false 2>&1" &
TAILSCALE_PID=$!

echo "Tailscale up started in background (PID: $TAILSCALE_PID)"
echo "Waiting 20 seconds while monitoring logs..."

for i in {1..20}; do
  echo -n "."
  sleep 1
done
echo ""

echo -e "\n[6] Checking tailscale status..."
docker compose exec -T worker-1 tailscale status 2>&1 || true

echo -e "\n[7] Headscale logs (looking for connection attempts)..."
docker logs coordinator 2>&1 | grep -E "(machine|node|auth|register|key|poll|endpoint)" | tail -40

echo -e "\n[8] Worker-1 container logs..."
docker logs worker-1 2>&1 | tail -30

kill $TAILSCALE_PID 2>/dev/null || true
wait $TAILSCALE_PID 2>/dev/null || true

echo -e "\n=== Diagnosis complete. Containers still running for manual inspection. ==="
echo "To clean up: cd /Users/strrl/playground/GitHub/wonder-mesh-net/e2e && docker compose down"
