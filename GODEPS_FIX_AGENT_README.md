# Go Dependency Fix Agent Documentation

This repository contains comprehensive documentation for the Go Dependency Fix Agent, an AI agent specialized in diagnosing and fixing Go module dependency issues.

## Files Created

1. **GODEPS_FIX_AGENT_PROMPT.md** - Main system prompt defining the agent's role, responsibilities, and methodology
2. **GODEPS_FIX_AGENT_DETAILED_GUIDE.md** - Detailed procedures for handling specific types of dependency issues
3. **GODEPS_FIX_AGENT_CHEATSHEET.md** - Quick reference for common commands and error patterns
4. **godeps_fix_agent_config.json** - Configuration file with system prompt and metadata

## Agent Capabilities

The Go Dependency Fix Agent is designed to handle:

- Missing go.sum entries
- Incorrect module paths
- Multi-module repository navigation
- Proper command sequencing
- Version conflicts
- Local path vs import path confusion

## Key Features

- Methodical error analysis approach
- Directory context awareness for multi-module repositories
- Proper command sequencing to avoid common mistakes
- Verification-focused workflow to ensure fixes actually work
- Repository-specific customization for this project

## Usage Instructions

To use the agent effectively:

1. Provide clear error messages or descriptions of dependency issues
2. Specify which module or directory needs attention if working in a multi-module repository
3. Allow the agent to run diagnostic commands to understand the problem
4. Let the agent verify fixes work before considering the issue resolved

The agent will follow a systematic approach:
1. Analyze error messages carefully
2. Assess repository structure
3. Identify root cause
4. Plan solution
5. Execute proper commands in correct sequence
6. Verify fix works with appropriate tests