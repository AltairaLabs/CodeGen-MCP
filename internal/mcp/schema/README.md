MCP Tool Registry — Schema and examples

Purpose
-------
This folder defines the contract (JSON Schema) the Coordinator will expose to LLM agents and clients. The schema describes:

- Language profiles: what languages are supported and what capabilities (AST/refactor tools, default formatters, allowed text helpers) are available.
- Tool descriptors: the shape of tools (tool_id, input/output schema, allowed scopes, dry-run support).

Files
-----
- `tool_schema.json` — authoritative JSON Schema for the registry data model.

Example: language profile (Python)
----------------------------------
```json
{
  "id": "python",
  "display_name": "Python",
  "file_extensions": [".py"],
  "formatter": "black",
  "ast_tools": ["libcst", "rope"],
  "text_commands": ["find", "sed", "xargs"],
  "test_command": "python -m pytest -q",
  "notes": "Prefer AST refactors via libcst where possible. Text commands are limited to small, curated operations."
}
```

Example: tool descriptor (fs.write)
-----------------------------------
```json
{
  "tool_id": "fs.write",
  "language": null,
  "description": "Write a file within the workspace. Path must be inside workspace root.",
  "atomic": true,
  "dry_run_supported": false,
  "input_schema": {
    "type": "object",
    "properties": {
      "path": { "type": "string" },
      "contents": { "type": "string" }
    },
    "required": ["path", "contents"]
  },
  "output_schema": {
    "type": "object",
    "properties": {
      "path": { "type": "string" },
      "written_bytes": { "type": "integer" }
    }
  },
  "max_files_affected": 1,
  "allowed_scopes": ["workspace:write"]
}
```

Example: tool descriptor (edit.ast.apply_refactor)
--------------------------------------------------
```json
{
  "tool_id": "edit.ast.apply_refactor",
  "language": "python",
  "description": "Apply a language-aware refactor using the configured AST tool.",
  "atomic": true,
  "dry_run_supported": true,
  "input_schema": {
    "type": "object",
    "properties": {
      "tool": { "type": "string", "enum": ["libcst", "rope"] },
      "op": { "type": "string", "enum": ["rename_symbol", "extract_function", "move_file"] },
      "params": { "type": "object" }
    },
    "required": ["tool", "op", "params"]
  },
  "output_schema": {
    "type": "object",
    "properties": {
      "patch": { "type": "string", "description": "Unified diff patch representing the changes (when dry-run=true)." },
      "applied": { "type": "boolean" }
    }
  },
  "max_files_affected": null,
  "allowed_scopes": ["workspace:refactor"]
}
```

Execution API (conceptual)
--------------------------
- `GET /v1/mcp/languages` — returns the list of language profiles (validate against `languageProfile` schema).
- `GET /v1/mcp/tools?language=python` — returns tool descriptors filtered by language.
- `POST /v1/mcp/tools/execute` — body follows the tool input schema plus `workspace_id`, `task_id`, `dry_run` and `caller_id` metadata.
- `GET /v1/mcp/tools/result/{request_id}` — returns the execution result and provenance.

Notes & next steps
-------------------
- This schema is intentionally conservative: tools are discrete and carry explicit input/output schemas to facilitate validation and auditing.
- Next: implement a small Go-based registry service that loads validated data (JSON or YAML) conforming to `tool_schema.json` and exposes the discovery endpoints. Then implement an executor stub that will validate an incoming request, enforce rate-limits/scopes, and forward to a worker.

Design choice for Python AST tool
--------------------------------
For the POC we recommend `libcst` as the first-class AST/refactor tool for Python because it supports precise, lossless transformations and round-trip code formatting while preserving code style comments and formatting. `rope` and `bowler` provide additional refactor primitives; they can be added later as optional `ast_tools` entries in the profile.

If you want, I'll now scaffold the minimal Go registry server (`internal/mcp/registry`) with:
- a small in-memory loader that validates `tool_schema.json`,
- `GET /v1/mcp/languages` and `GET /v1/mcp/tools`, and
- a stub `POST /v1/mcp/tools/execute` that returns a queued response with a `request_id`.

Tell me to proceed and I'll implement the Go skeleton next (I recommend doing that now).