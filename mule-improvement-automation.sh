#!/usr/bin/env bash
#
# Mule Project Improvement Automation Script
# 
# This script runs the ralph-sh methodology to automate improvements to the
# Mule project (github.com/mule-ai/mule)
#
# It:
# 1. Reads project documentation (CLAUDE.md, spec.md, plan.md, README.md)
# 2. Runs ralph-sh to execute tasks sequentially
# 3. The agent identifies as "Mule" - focused on AI development and Golang
# 4. When tasks are complete, creates a PR and merges it
#
# Usage:
#   ./mule-improvement-automation.sh
#
# Arguments:
#   (none) - Runs with unlimited loops (recommended)
#   -1      - Explicitly unlimited loops
#   N       - Run max N loops (not recommended, use unlimited)
#
# Requirements:
#   - pi CLI installed
#   - ralph-sh at /usr/local/bin/ralph-sh
#   - Git configured with GitHub credentials
#   - Network access for GitHub API
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MULE_PROJECT_DIR="$SCRIPT_DIR"
LOCK_FILE="/tmp/mule-improvement-automation.lock"

# Files to read
CONTEXT_FILES=("CLAUDE.md" "README.md" "MULE-V2.md" "CONTRIBUTING.md" "SKILL.md")

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
echo "  Mule Project Improvement Automation"
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

# Check git is configured
if ! git config --global user.email &> /dev/null; then
    echo "Error: Git user.email not configured"
    exit 1
fi
echo "  ✓ Git configured"

if ! git config --global user.name &> /dev/null; then
    echo "Error: Git user.name not configured"
    exit 1
fi
echo "  ✓ Git user configured"

# Check context files exist
for file in "${CONTEXT_FILES[@]}"; do
    if [[ ! -f "$MULE_PROJECT_DIR/$file" ]]; then
        echo "Warning: $file not found in $MULE_PROJECT_DIR"
    fi
done
echo "  ✓ Context files present"

# Check for gh CLI (needed for PR creation/merge)
if ! command -v gh &> /dev/null; then
    echo "Warning: gh CLI not found. PR creation/merge may not work."
else
    echo "  ✓ gh CLI found"
fi

echo ""

# Change to project directory
cd "$MULE_PROJECT_DIR"
echo "📂 Working in: $(pwd)"
echo ""

# Pull latest main
echo "📥 Pulling latest changes from main..."
git fetch origin
git checkout main
git pull origin main
echo ""

# Display initial context
echo "=========================================="
echo "  Agent Identity: Mule"
echo "  Focus: AI Development, Golang"
echo "  Mission: Improve the Mule project"
echo "=========================================="
echo ""

# Ensure spec.md and plan.md exist
SPEC_FILE="improvement-spec.md"
PLAN_FILE="improvement-plan.md"
PROGRESS_FILE="improvement-progress.md"
SUMMARY_FILE="SUMMARY.md"

if [[ ! -f "$SPEC_FILE" ]]; then
    echo "Error: $SPEC_FILE not found"
    exit 1
fi

if [[ ! -f "$PLAN_FILE" ]]; then
    echo "Error: $PLAN_FILE not found"
    exit 1
fi
echo "  ✓ Using $SPEC_FILE and $PLAN_FILE"
echo ""

# Run ralph-sh with the plan and spec
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
echo "  Improvement Automation Complete!"
echo "=========================================="
echo ""

# Check if there are changes to commit (only staged/tracked files, ignore untracked automation files)
if git diff --quiet && git diff --cached --quiet && [[ -z $(git ls-files --others --exclude-standard | grep -v -E "(improvement-|mule-.*automation|SUMMARY\.md|progress\.md|SKILL\.md|api/)") ]]; then
    echo "No changes to commit."
else
    echo "Changes detected. Creating PR..."
    
    # Clean up any automation files that shouldn't be committed
    git restore SUMMARY.md progress.md plan.md spec.md 2>/dev/null || true
    rm -f SUMMARY.md progress.md plan.md spec.md
    rm -f improvement-plan.md improvement-progress.md improvement-spec.md
    rm -f mule-blog-automation.sh mule-improvement-automation.sh
    
    # Check if there's already a branch with improvements
    EXISTING_BRANCH=$(git branch -a | grep "improvement/phase-" | head -1 | sed 's/^[ *]*//' || true)
    
    if [[ -n "$EXISTING_BRANCH" ]]; then
        echo "Using existing branch: $EXISTING_BRANCH"
        git checkout main
        git branch -D "$EXISTING_BRANCH" 2>/dev/null || true
    fi
    
    # Create a new branch for the improvements
    BRANCH_NAME="improvement/phase-$(date +%Y%m%d-%H%M%S)"
    git checkout -b "$BRANCH_NAME"
    
    # Stage all changes (excluding automation files)
    git add -A
    git reset HEAD mule-blog-automation.sh mule-improvement-automation.sh SKILL.md api improvement-plan.md improvement-progress.md improvement-spec.md 2>/dev/null || true
    
    # Create a commit
    if git diff --cached --quiet; then
        echo "No significant changes to commit."
    else
        git commit -m "Mule AI automated improvements

This commit contains automated improvements made by the Mule AI agent.

Changes include:
- Code improvements and refactoring
- Documentation updates
- Bug fixes and optimizations

Generated by mule-improvement-automation.sh"
        
        # Push branch
        git push -u origin "$BRANCH_NAME"
        
        # Try to create and merge PR using gh CLI
        if command -v gh &> /dev/null; then
            echo ""
            echo "Creating pull request..."
            PR_URL=$(gh pr create --title "Mule AI Automated Improvements" --body "This PR contains automated improvements made by the Mule AI agent.

Changes include code improvements, refactoring, documentation updates, and optimizations.

Please review the changes and merge when ready." --base main 2>&1)
            
            if [[ $? -eq 0 ]]; then
                echo "PR created successfully!"
                echo "$PR_URL"
                
                # Extract PR number from URL
                PR_NUMBER=$(echo "$PR_URL" | grep -oE '[0-9]+$' | tail -1)
                
                # Wait for CI checks to pass (max 10 minutes)
                echo ""
                echo "Waiting for CI checks to pass..."
                echo "This may take several minutes..."
                
                CI_TIMEOUT=600  # 10 minutes
                CI_START=$(date +%s)
                CI_PASSED=false
                CHECKS_FOUND=false
                
                while true; do
                    CURRENT_TIME=$(date +%s)
                    ELAPSED=$((CURRENT_TIME - CI_START))
                    
                    if [[ $ELAPSED -ge $CI_TIMEOUT ]]; then
                        echo "Timeout waiting for CI checks (${CI_TIMEOUT}s). CI may still be running."
                        echo "PR is ready for manual review: $PR_URL"
                        break
                    fi
                    
                    # Get CI status using the commits API which is more reliable
                    CI_STATUS=$(gh api repos/mule-ai/mule/commits --paginate 2>/dev/null | head -1)
                    
                    # Try using the run status API
                    RUN_STATUS=$(gh run list --workflow="CI/CD Pipeline" --limit 1 --json status,conclusion --jq '.[0]' 2>/dev/null || echo '{"status":"unknown"}')
                    RUN_CONCLUSION=$(echo "$RUN_STATUS" | jq -r '.conclusion // "unknown"' 2>/dev/null)
                    RUN_STATUS_VALUE=$(echo "$RUN_STATUS" | jq -r '.status // "unknown"' 2>/dev/null)
                    
                    # Also check PR checks directly
                    PR_CHECKS=$(gh pr view "$PR_NUMBER" --json statusCheckRollup --jq '.statusCheckRollup | length' 2>/dev/null || echo "0")
                    
                    if [[ "$PR_CHECKS" != "0" ]]; then
                        CHECKS_FOUND=true
                        # Get all check statuses
                        ALL_CHECKS=$(gh pr view "$PR_NUMBER" --json statusCheckRollup --jq '.statusCheckRollup[] | "\(.conclusion)_\(.status)"' 2>/dev/null)
                        
                        # Count failures and pending
                        FAILURES=$(echo "$ALL_CHECKS" | grep -c "FAILURE" || echo "0")
                        IN_PROGRESS=$(echo "$ALL_CHECKS" | grep -c "IN_PROGRESS" || echo "0")
                        EXPECTED=$(echo "$ALL_CHECKS" | grep -c "EXPECTED" || echo "0")
                        
                        if [[ "$FAILURES" -gt 0 ]]; then
                            echo "CI checks failed. PR requires manual review."
                            echo "Branch $BRANCH_NAME is available for review."
                            break
                        elif [[ "$IN_PROGRESS" -gt 0 ]] || [[ "$EXPECTED" -gt 0 ]]; then
                            echo "CI checks still running... (${ELAPSED}s elapsed)"
                        else
                            echo "All CI checks passed!"
                            CI_PASSED=true
                            break
                        fi
                    else
                        if [[ "$CHECKS_FOUND" == "false" ]]; then
                            echo "Waiting for CI checks to start... (${ELAPSED}s elapsed)"
                        else
                            echo "Waiting for CI... (${ELAPSED}s elapsed)"
                        fi
                    fi
                    
                    sleep 30
                done
                
                # Attempt to merge if CI passed
                if [[ "$CI_PASSED" == "true" ]]; then
                    echo ""
                    echo "Attempting to merge PR..."
                    if gh pr merge "$PR_NUMBER" --squash --delete-branch 2>&1; then
                        echo "PR merged successfully!"
                    else
                        echo "PR created but could not auto-merge. Please review and merge manually."
                        echo "Branch $BRANCH_NAME is available for review."
                    fi
                fi
            else
                echo "Could not create PR. Please check gh CLI authentication."
            fi
        else
            echo "gh CLI not found. Please create PR manually."
            echo "Branch: $BRANCH_NAME"
        fi
    fi
    
    # Return to main branch
    git checkout main 2>/dev/null || true
fi

echo ""
echo "📝 Check the following files for results:"
echo "   - $MULE_PROJECT_DIR/$PROGRESS_FILE"
echo "   - $MULE_PROJECT_DIR/$SUMMARY_FILE"
echo ""

exit 0
