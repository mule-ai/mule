#!/bin/bash

# Test script for workflow step reordering functionality

echo "Testing workflow step reordering..."

# Test the health endpoint first
echo "Checking service health..."
if curl -s http://localhost:8140/health | grep -q "OK"; then
    echo "Service is healthy"
else
    echo "Service is not responding"
    exit 1
fi

echo "Setup complete. You can now test the reordering functionality in the UI."
echo "1. Create a workflow with multiple steps"
echo "2. Try reordering the steps using drag and drop"
echo "3. Check the logs for any errors: docker logs mule-api"