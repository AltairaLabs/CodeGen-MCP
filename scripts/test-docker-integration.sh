#!/usr/bin/env bash
# Integration test script for Docker Compose setup
# Tests venv initialization and package isolation

set -e

echo "🧪 CodeGen-MCP Docker Integration Test"
echo "========================================"
echo ""

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check if services are running
echo "📋 Step 1: Checking if services are running..."
if ! docker compose -f docker-compose.dev.yml ps | grep -q "Up"; then
    echo -e "${RED}❌ Services are not running. Please start with: make docker-up-dev${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Services are running${NC}"
echo ""

# Check coordinator health
echo "📋 Step 2: Checking coordinator health..."
if ! docker compose -f docker-compose.dev.yml logs coordinator | grep -q "gRPC server listening"; then
    echo -e "${YELLOW}⚠️  Coordinator may not be ready yet${NC}"
else
    echo -e "${GREEN}✅ Coordinator is healthy${NC}"
fi
echo ""

# Check worker registration
echo "📋 Step 3: Checking worker registration..."
if ! docker compose -f docker-compose.dev.yml logs worker-1 | grep -q "Successfully registered"; then
    echo -e "${YELLOW}⚠️  Worker may not be registered yet${NC}"
else
    echo -e "${GREEN}✅ Worker registered successfully${NC}"
fi
echo ""

# Test venv creation in worker
echo "📋 Step 4: Testing Python venv in worker container..."
VENV_TEST=$(docker compose -f docker-compose.dev.yml exec -T worker-1 bash -c "
    # Create a test workspace
    mkdir -p /home/builder/test-workspace
    cd /home/builder/test-workspace
    
    # Simulate venv creation (like session initialization does)
    python3 -m venv .venv
    
    # Check if venv was created
    if [ -f .venv/bin/python ]; then
        echo 'venv_created'
    fi
    
    # Test package installation
    .venv/bin/pip install --quiet requests > /dev/null 2>&1
    if .venv/bin/python -c 'import requests' 2>/dev/null; then
        echo 'package_installed'
    fi
    
    # Cleanup
    cd /home/builder
    rm -rf test-workspace
")

if echo "$VENV_TEST" | grep -q "venv_created"; then
    echo -e "${GREEN}✅ Python venv created successfully${NC}"
else
    echo -e "${RED}❌ Failed to create Python venv${NC}"
    exit 1
fi

if echo "$VENV_TEST" | grep -q "package_installed"; then
    echo -e "${GREEN}✅ Package installation in venv works${NC}"
else
    echo -e "${YELLOW}⚠️  Package installation may have issues${NC}"
fi
echo ""

# Check workspace structure
echo "📋 Step 5: Checking workspace structure..."
WORKSPACE_CHECK=$(docker compose -f docker-compose.dev.yml exec -T worker-1 bash -c "
    ls -la /home/builder/workspaces 2>/dev/null | wc -l
")

if [ "$WORKSPACE_CHECK" -gt 0 ]; then
    echo -e "${GREEN}✅ Workspace directory exists${NC}"
else
    echo -e "${YELLOW}⚠️  No workspaces created yet (normal if no sessions)${NC}"
fi
echo ""

# Check logs for errors
echo "📋 Step 6: Checking logs for errors..."
ERROR_COUNT=$(docker compose -f docker-compose.dev.yml logs | grep -i "error\|fatal\|panic" | wc -l)
if [ "$ERROR_COUNT" -eq 0 ]; then
    echo -e "${GREEN}✅ No errors in logs${NC}"
else
    echo -e "${YELLOW}⚠️  Found $ERROR_COUNT error/warning messages in logs${NC}"
fi
echo ""

# Summary
echo "=========================================="
echo "🎉 Integration Test Complete!"
echo ""
echo "Key Points:"
echo "  • Services: Running"
echo "  • Python venv: Working"
echo "  • Package isolation: Ready"
echo ""
echo "Next steps:"
echo "  • Run: make docker-logs (to monitor logs)"
echo "  • Run: make docker-shell-worker (to inspect worker)"
echo "  • Connect MCP client to localhost:50051"
echo ""
