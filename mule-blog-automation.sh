#!/usr/bin/env bash
#
# Mule AI Blog & Documentation Automation Script
# 
# This script runs the ralph-sh methodology to automate:
# 1. Blog content creation for https://muleai.io/blog
# 2. Documentation updates for https://muleai.io/docs
#
# It:
# 1. Sets up the muleai.io working directory
# 2. Copies spec.md and plan.md from the mule project
# 3. Runs ralph-sh to execute blog tasks sequentially
# 4. Then runs documentation improvement tasks
# 5. The agent identifies as "Mule" - an AI agent focused on AI dev and Golang
#
# Usage:
#   ./mule-blog-automation.sh
#
# Arguments:
#   (none) - Runs with unlimited loops (recommended)
#   -1      - Explicitly unlimited loops
#   N       - Run max N loops (not recommended, use unlimited)
#
# Requirements:
#   - pi CLI installed
#   - ralph-sh at /usr/local/bin/ralph-sh
#   - Network access for GitHub API and blog publishing
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MULE_PROJECT_DIR="$(dirname "$SCRIPT_DIR")/mule"
TARGET_DIR="$(dirname "$SCRIPT_DIR")/muleai.io"
LOCK_FILE="/tmp/mule-blog-automation.lock"

# Files to copy
SPEC_FILE="spec.md"
PLAN_FILE="plan.md"
PROGRESS_FILE="progress.md"
SUMMARY_FILE="SUMMARY.md"

# Documentation files
DOCS_SPEC_FILE="docs-spec.md"
DOCS_PLAN_FILE="docs-plan.md"

# pi model config
PROVIDER="proxy-anthropic"
MODEL="minimax-anthropic/minimax-m2.7-highspeed"

# Trap for cleanup
trap 'echo "Script interrupted"; exit 130' INT

# Check for existing lock file to prevent duplicate runs
if [[ -f "$LOCK_FILE" ]]; then
    echo "Error: Another instance of this script is already running (lock file exists: $LOCK_FILE)"
    echo "If you're sure no other instance is running, delete the lock file and try again:"
    echo "  rm $LOCK_FILE"
    exit 1
fi

# Create lock file
echo "$$" > "$LOCK_FILE"
trap 'rm -f "$LOCK_FILE"' EXIT

echo "=========================================="
echo "  Mule AI Blog & Documentation Automation"
echo "=========================================="
echo ""

# Check prerequisites
echo "📋 Checking prerequisites..."

# Check pi is available
if ! command -v pi &> /dev/null; then
    echo "Error: pi CLI not found. Please install pi first."
    exit 1
fi
echo "  ✓ pi CLI found"

# Check ralph-sh is available
if [[ ! -x "/usr/local/bin/ralph-sh" ]]; then
    echo "Error: ralph-sh not found at /usr/local/bin/ralph-sh"
    exit 1
fi
echo "  ✓ ralph-sh found"

# Check source files exist
if [[ ! -f "$MULE_PROJECT_DIR/$SPEC_FILE" ]]; then
    echo "Error: $SPEC_FILE not found in $MULE_PROJECT_DIR"
    exit 1
fi

if [[ ! -f "$MULE_PROJECT_DIR/$PLAN_FILE" ]]; then
    echo "Error: $PLAN_FILE not found in $MULE_PROJECT_DIR"
    exit 1
fi

# Check documentation spec and plan exist
if [[ ! -f "$MULE_PROJECT_DIR/$DOCS_SPEC_FILE" ]]; then
    echo "Warning: $DOCS_SPEC_FILE not found - documentation updates will be skipped"
    DOCS_ENABLED=false
else
    echo "  ✓ Documentation spec found"
    DOCS_ENABLED=true
fi

if [[ ! -f "$MULE_PROJECT_DIR/$DOCS_PLAN_FILE" ]]; then
    echo "Warning: $DOCS_PLAN_FILE not found - documentation updates will be skipped"
    DOCS_ENABLED=false
else
    echo "  ✓ Documentation plan found"
fi

echo "  ✓ Source files found"

echo ""

# Setup target directory
echo "📁 Setting up $TARGET_DIR..."

mkdir -p "$TARGET_DIR"

# Copy spec and plan files
echo "  Copying $SPEC_FILE..."
cp "$MULE_PROJECT_DIR/$SPEC_FILE" "$TARGET_DIR/$SPEC_FILE"

echo "  Copying $PLAN_FILE..."
cp "$MULE_PROJECT_DIR/$PLAN_FILE" "$TARGET_DIR/$PLAN_FILE"

# Copy documentation spec and plan if they exist
if [[ "$DOCS_ENABLED" == "true" ]]; then
    echo "  Copying $DOCS_SPEC_FILE..."
    cp "$MULE_PROJECT_DIR/$DOCS_SPEC_FILE" "$TARGET_DIR/$DOCS_SPEC_FILE"
    
    echo "  Copying $DOCS_PLAN_FILE..."
    cp "$MULE_PROJECT_DIR/$DOCS_PLAN_FILE" "$TARGET_DIR/$DOCS_PLAN_FILE"
fi

# Initialize progress.md if it doesn't exist
if [[ ! -f "$TARGET_DIR/$PROGRESS_FILE" ]]; then
    cat > "$TARGET_DIR/$PROGRESS_FILE" << 'EOF'
# Progress Report

## Mule AI Blog & Documentation Automation Run

**Started:** $(date)

---

### Notes

EOF
    echo "  Created $PROGRESS_FILE"
else
    echo "  Using existing $PROGRESS_FILE"
fi

echo ""

# Verify blog directory exists or create basic structure
if [[ ! -d "$TARGET_DIR/blog" ]] && [[ ! -d "$TARGET_DIR/content" ]]; then
    echo "  Creating basic blog structure..."
    mkdir -p "$TARGET_DIR/content/blog"
    mkdir -p "$TARGET_DIR/themes"
fi

echo "  ✓ Target directory ready"
echo ""

# Change to target directory
cd "$TARGET_DIR"
echo "📂 Working in: $(pwd)"
echo ""

# Pull latest main
echo "📥 Pulling latest changes from main..."
git fetch origin
git checkout main
git pull origin main
echo ""

# Display initial task info
echo "=========================================="
echo "  Agent Identity: Mule"
echo "  Focus: AI Development, Golang"
echo "  Interests: Electronic Music, AGI"
echo "=========================================="
echo ""

# Run ralph-sh with the plan and spec
# Pass -1 to indicate unlimited loops (ralph-sh will skip max loops check)
max_loops="${1:--1}"

echo "🚀 Starting ralph-sh automation..."
echo "   Plan: $PLAN_FILE"
echo "   Spec: $SPEC_FILE"
echo "   Max loops: unlimited"
echo "   Provider: $PROVIDER"
echo "   Model: $MODEL"
echo ""

# Set environment variables for pi provider and model
export PI_PROVIDER=$PROVIDER
export PI_MODEL=$MODEL
export GH_TOKEN

# Always pass max_loops to ralph-sh (-1 = unlimited)
/usr/local/bin/ralph-sh "$PLAN_FILE" "$SPEC_FILE" "$max_loops"

echo ""
echo "=========================================="
echo "  Blog Automation Complete!"
echo "=========================================="
echo ""

# Clean up blog files
rm -f $PLAN_FILE $SPEC_FILE $PROGRESS_FILE $SUMMARY_FILE

# Now run documentation updates if enabled
if [[ "$DOCS_ENABLED" == "true" ]]; then
    echo ""
    echo "=========================================="
    echo "  Starting Documentation Updates"
    echo "=========================================="
    echo ""
    
    # Set up documentation progress
    DOCS_PROGRESS_FILE="docs-progress.md"
    DOCS_SUMMARY_FILE="docs-summary.md"
    
    if [[ ! -f "$DOCS_PROGRESS_FILE" ]]; then
        cat > "$DOCS_PROGRESS_FILE" << 'EOF'
# Documentation Progress Report

## Mule AI Documentation Automation Run

**Started:** $(date)

---

### Notes

EOF
        echo "  Created $DOCS_PROGRESS_FILE"
    fi
    
    echo "  📂 Working on: content/docs/"
    echo ""
    
    # Run ralph-sh for documentation
    echo "🚀 Starting documentation ralph-sh..."
    echo "   Plan: $DOCS_PLAN_FILE"
    echo "   Spec: $DOCS_SPEC_FILE"
    echo "   Provider: $PROVIDER"
    echo "   Model: $MODEL"
    echo ""
    
    # Run documentation improvements
    /usr/local/bin/ralph-sh "$DOCS_PLAN_FILE" "$DOCS_SPEC_FILE" "$max_loops"
    
    # Clean up documentation files
    rm -f $DOCS_PLAN_FILE $DOCS_SPEC_FILE $DOCS_PROGRESS_FILE $DOCS_SUMMARY_FILE
    
    echo ""
    echo "=========================================="
    echo "  Documentation Updates Complete!"
    echo "=========================================="
    echo ""
fi

echo "📝 Check the following files for results:"
echo "   - $TARGET_DIR/$PROGRESS_FILE (blog)"
echo "   - $TARGET_DIR/$SUMMARY_FILE (blog)"
echo "   - $TARGET_DIR/content/blog/"
if [[ "$DOCS_ENABLED" == "true" ]]; then
    echo "   - $TARGET_DIR/content/docs/"
fi
echo ""
echo "🌐 Blog should be live at: https://muleai.io/blog"
if [[ "$DOCS_ENABLED" == "true" ]]; then
    echo "🌐 Documentation should be live at: https://muleai.io/docs"
fi
echo ""

exit 0
