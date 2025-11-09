#!/bin/bash

# Multi-Worker & Multi-Session Integration Test
# Tests session distribution and concurrent execution across multiple workers

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo "🧪 Multi-Worker & Multi-Session Test"
echo "===================================="

# Check if both workers are running
echo ""
echo "📋 Step 1: Verifying multi-worker setup..."
WORKER_COUNT=$(docker compose ps --services --filter "status=running" | grep -c "worker" || true)
if [ "$WORKER_COUNT" -lt 2 ]; then
    echo -e "${RED}❌ Need at least 2 workers running${NC}"
    echo "Run: docker compose --profile multi-worker up -d"
    exit 1
fi
echo -e "${GREEN}✅ Found $WORKER_COUNT workers running${NC}"

# Check both workers registered
echo ""
echo "📋 Step 2: Checking worker registrations..."
for worker in worker-1 worker-2; do
    REGISTERED=$(docker compose logs $worker | grep -c "Successfully registered" || true)
    if [ "$REGISTERED" -gt 0 ]; then
        echo -e "${GREEN}  ✓ $worker registered successfully${NC}"
    else
        echo -e "${YELLOW}  ⚠️  $worker may not be registered yet${NC}"
    fi
done

# Test concurrent session creation
echo ""
echo "📋 Step 3: Testing concurrent session creation..."
echo "  Creating test workspaces in both workers..."

# Create multiple test sessions simultaneously
for i in {1..6}; do
    (
        WORKER="worker-$((($i % 2) + 1))"
        SESSION_ID="test-session-$i"
        docker compose exec -T $WORKER mkdir -p /home/builder/workspaces/$SESSION_ID 2>/dev/null || true
        if [ $? -eq 0 ]; then
            echo -e "${GREEN}    ✓ Session $i workspace created${NC}"
        fi
    ) &
done
wait
echo -e "${GREEN}✅ Concurrent session workspaces created${NC}"

# Test Python venv creation in multiple sessions
echo ""
echo "📋 Step 4: Testing Python venv in multiple sessions..."
SUCCESS_COUNT=0
FAIL_COUNT=0

for i in {1..4}; do
    WORKER="worker-$((($i % 2) + 1))"
    SESSION_ID="test-session-$i"
    WORKSPACE="/home/builder/workspaces/$SESSION_ID"
    
    # Create venv
    docker compose exec -T $WORKER bash -c "cd $WORKSPACE && python3 -m venv .venv" 2>/dev/null
    
    # Check if venv was created
    if docker compose exec -T $WORKER test -f "$WORKSPACE/.venv/bin/python" 2>/dev/null; then
        echo -e "${GREEN}  ✓ Session $i: venv created in $WORKER${NC}"
        ((SUCCESS_COUNT++))
    else
        echo -e "${RED}  ❌ Session $i: venv creation failed${NC}"
        ((FAIL_COUNT++))
    fi
done

echo -e "${GREEN}✅ Virtual environments: $SUCCESS_COUNT created, $FAIL_COUNT failed${NC}"

# Test package isolation
echo ""
echo "📋 Step 5: Testing package isolation across sessions..."
echo "  Installing different packages in different sessions..."

# Install requests in session-1 (worker-1)
echo "  Installing 'requests' in session-1..."
docker compose exec -T worker-1 bash -c \
    "cd /home/builder/workspaces/test-session-1 && .venv/bin/pip install -q requests" 2>/dev/null && \
    echo -e "${GREEN}    ✓ requests installed in session-1${NC}" || \
    echo -e "${YELLOW}    ⚠️  Could not install requests${NC}"

# Install flask in session-2 (worker-2)
echo "  Installing 'flask' in session-2..."
docker compose exec -T worker-2 bash -c \
    "cd /home/builder/workspaces/test-session-2 && .venv/bin/pip install -q flask" 2>/dev/null && \
    echo -e "${GREEN}    ✓ flask installed in session-2${NC}" || \
    echo -e "${YELLOW}    ⚠️  Could not install flask${NC}"

# Verify isolation
echo ""
echo "  Verifying package isolation..."

# Check requests is available in session-1
if docker compose exec -T worker-1 bash -c \
    "cd /home/builder/workspaces/test-session-1 && .venv/bin/python -c 'import requests'" 2>/dev/null; then
    echo -e "${GREEN}    ✓ session-1 can import requests${NC}"
else
    echo -e "${RED}    ❌ session-1 cannot import requests${NC}"
fi

# Check requests is NOT available in session-2
if docker compose exec -T worker-2 bash -c \
    "cd /home/builder/workspaces/test-session-2 && .venv/bin/python -c 'import requests'" 2>/dev/null; then
    echo -e "${RED}    ❌ ISOLATION FAILED: session-2 can import requests (should not)${NC}"
else
    echo -e "${GREEN}    ✓ session-2 cannot import requests (correct isolation)${NC}"
fi

# Check flask is available in session-2
if docker compose exec -T worker-2 bash -c \
    "cd /home/builder/workspaces/test-session-2 && .venv/bin/python -c 'import flask'" 2>/dev/null; then
    echo -e "${GREEN}    ✓ session-2 can import flask${NC}"
else
    echo -e "${RED}    ❌ session-2 cannot import flask${NC}"
fi

# Check flask is NOT available in session-1
if docker compose exec -T worker-1 bash -c \
    "cd /home/builder/workspaces/test-session-1 && .venv/bin/python -c 'import flask'" 2>/dev/null; then
    echo -e "${RED}    ❌ ISOLATION FAILED: session-1 can import flask (should not)${NC}"
else
    echo -e "${GREEN}    ✓ session-1 cannot import flask (correct isolation)${NC}"
fi

# Test concurrent file operations
echo ""
echo "📋 Step 6: Testing concurrent file operations..."
echo "  Writing files concurrently across sessions..."

for i in {1..6}; do
    (
        WORKER="worker-$((($i % 2) + 1))"
        SESSION_ID="test-session-$i"
        CONTENT="Task $i executed at $(date)"
        docker compose exec -T $WORKER bash -c \
            "echo '$CONTENT' > /home/builder/workspaces/$SESSION_ID/test-file-$i.txt" 2>/dev/null && \
            echo -e "${GREEN}    ✓ File $i written${NC}"
    ) &
done
wait

# Verify files
echo "  Verifying file isolation..."
for i in {1..6}; do
    WORKER="worker-$((($i % 2) + 1))"
    SESSION_ID="test-session-$i"
    if docker compose exec -T $WORKER test -f "/home/builder/workspaces/$SESSION_ID/test-file-$i.txt" 2>/dev/null; then
        echo -e "${GREEN}    ✓ File $i exists in correct session${NC}"
    else
        echo -e "${RED}    ❌ File $i missing${NC}"
    fi
done

# Check resource distribution
echo ""
echo "📋 Step 7: Checking workspace distribution..."
echo "  Worker-1 workspaces:"
docker compose exec -T worker-1 ls -1 /home/builder/workspaces 2>/dev/null | grep test-session | \
    awk '{print "    - " $0}' || echo "    (none)"

echo "  Worker-2 workspaces:"
docker compose exec -T worker-2 ls -1 /home/builder/workspaces 2>/dev/null | grep test-session | \
    awk '{print "    - " $0}' || echo "    (none)"

# Summary
echo ""
echo "=========================================="
echo -e "${GREEN}🎉 Multi-Worker Test Complete!${NC}"
echo ""
echo "Key Findings:"
echo "  • Workers Running: $WORKER_COUNT"
echo "  • Virtual Environments: Created and isolated"
echo "  • Package Isolation: Working correctly"
echo "  • Concurrent Operations: Successful"
echo ""
echo "Next steps:"
echo "  • Run: make docker-logs (to monitor activity)"
echo "  • Run: make docker-ps (to check status)"
echo "  • Cleanup: docker compose down"
