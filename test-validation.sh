#!/bin/bash

# Create a test directory
TEST_DIR="/tmp/validation-test"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Create a simple test that will fail
echo "This is a test file" > test.txt

# Create a validation script that will fail the first time but succeed after a fix
cat > validate.sh << 'EOF'
#!/bin/bash
if [ -f "fixed.txt" ]; then
    echo "Validation passed!"
    exit 0
else
    echo "Validation failed: fixed.txt not found"
    exit 1
fi
EOF

chmod +x validate.sh

# Create a corrective script that will create the missing file
cat > fix.sh << 'EOF'
#!/bin/bash
echo "Creating fixed.txt to fix the validation"
echo "Fixed!" > fixed.txt
EOF

chmod +x fix.sh

echo "Test environment created in $TEST_DIR"
echo "Run validate.sh to test validation (should fail initially)"
echo "Run fix.sh to apply fix"
echo "Run validate.sh again to test validation (should pass after fix)"