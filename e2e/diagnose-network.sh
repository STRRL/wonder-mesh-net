#!/bin/bash
set -e

echo "=== Network Diagnosis ==="

echo -e "\n[1] Starting services..."
docker compose up -d
sleep 10
docker restart coordinator
sleep 5

echo -e "\n[2] Checking coordinator network mode..."
docker inspect coordinator --format='Network Mode: {{.HostConfig.NetworkMode}}'

echo -e "\n[3] Checking worker-1 /etc/hosts..."
docker exec worker-1 cat /etc/hosts | grep -E "(coordinator|host-gateway|192\.168)"

echo -e "\n[4] Testing DNS resolution from worker-1..."
docker exec worker-1 getent hosts coordinator || echo "DNS resolution failed"

echo -e "\n[5] Testing connectivity: worker-1 -> coordinator:9080..."
docker exec worker-1 curl -v --connect-timeout 5 http://coordinator:9080/hs/health 2>&1 | grep -E "(Trying|Connected|HTTP|failed|refused)" || echo "Connection test failed"

echo -e "\n[6] Testing connectivity: worker-1 -> host.docker.internal:9080..."
docker exec worker-1 curl -v --connect-timeout 5 http://host.docker.internal:9080/hs/health 2>&1 | grep -E "(Trying|Connected|HTTP|failed|refused)" || echo "Connection test failed"

echo -e "\n[7] Getting host gateway IP..."
docker exec worker-1 cat /etc/hosts | grep host-gateway | awk '{print $1}'

echo -e "\n[8] Checking if coordinator is listening on 9080..."
docker exec coordinator netstat -tuln | grep 9080 || echo "Port 9080 not found"

echo -e "\n[9] Checking Headscale server_url config..."
docker exec coordinator grep "^server_url:" /etc/headscale/config.yaml

echo -e "\n=== Diagnosis Complete ==="
echo "Based on the results above, the issue is likely:"
echo "- If coordinator DNS fails: extra_hosts not working correctly"
echo "- If connection refused: coordinator not accessible from worker network"
echo "- If timeout: network routing issue between containers"
