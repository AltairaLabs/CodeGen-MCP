# Phase 2: Package Restructuring Execution Plan

## Overview
Moving from 55 files in coordinator to ~15 core files, extracting 7 new packages using hybrid manual/gomvpkg approach.

## Phase 2A: Tools Package (14 files) - GOMVPKG
**Target:** `internal/tools`
**Method:** Group then move with gomvpkg
**Files:**
- tool_*.go (8 implementation files)
- tool_*_test.go (6 test files)

**Steps:**
1. Create tools package structure in coordinator
2. Move all tool files to coordinator/tools
3. Use gomvpkg to move to internal/tools
4. Update imports automatically

## Phase 2B: Storage Package - GOMVPKG  
**Target:** `internal/storage` 
**Method:** Direct gomvpkg move
**Files:**
- storage/ directory (4 files)
- storage_queued_task_adapter.go

**Steps:**
1. Use gomvpkg to move entire storage directory
2. Move storage adapter to adapters package

## Phase 2C: Worker Management (7 files) - GOMVPKG
**Target:** `internal/worker`
**Method:** Group then move
**Files:**
- worker*.go (4 files)
- lifecycle_*.go (3 files - worker lifecycle)

**Steps:**
1. Group worker-related files
2. Use gomvpkg for clean move

## Phase 2D: Session Management (4 files) - GOMVPKG  
**Target:** `internal/session`
**Method:** Direct move
**Files:**
- session*.go
- sse_session_manager*.go

## Phase 2E: Server/HTTP Layer (4 files) - MANUAL + GOMVPKG
**Target:** `internal/server`  
**Method:** Extract interfaces first, then move
**Files:**
- server*.go (4 files)

**Complexity:** High - has dependencies on coordinator

## Phase 2F: Streaming (4 files) - MANUAL + GOMVPKG
**Target:** `internal/streaming`
**Method:** Extract interfaces, then move  
**Files:**
- result_streamer*.go
- sse_session_manager*.go (if not in session)

## Phase 2G: Task Management (11 files) - HYBRID
**Target:** Split between coordinator and `internal/tasks`
**Method:** Keep core in coordinator, move complex parts
**Files:**
- task_queue*.go (keep core, extract dispatch/loops)
- task_handlers*.go (move to internal/handlers)
- task_dispatcher*.go (move to internal/tasks)

## Phase 2H: Adapters Package - GOMVPKG
**Target:** `internal/adapters` 
**Method:** Group all adapter files
**Files:**
- *_adapter.go (4 adapter files)

## Final State: Coordinator Core (~15 files)
**Remaining in coordinator:**
- Core coordination logic
- Main task queue interface
- Integration points
- Primary types

## Execution Order:
1. **Phase 2A: Tools** (easy, no dependencies)
2. **Phase 2B: Storage** (easy, clear boundaries)  
3. **Phase 2D: Session** (medium, some dependencies)
4. **Phase 2C: Worker** (medium, lifecycle complexity)
5. **Phase 2H: Adapters** (easy, after other moves)
6. **Phase 2F: Streaming** (hard, interface extraction)
7. **Phase 2E: Server** (hard, many dependencies) 
8. **Phase 2G: Tasks** (hardest, core functionality)

## Success Criteria:
- All tests pass after each phase
- Import paths automatically updated
- Clean package boundaries
- No circular dependencies
- High test coverage maintained