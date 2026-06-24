---
name: xdebug-docker-debug
description: |
  Use when the user wants to debug PHP code running in Docker with Xdebug.
  Covers: setting breakpoints, firing HTTP requests or CLI commands, stepping,
  inspecting variables, evaluating expressions, and toggling Xdebug in the container.
---

# Xdebug Docker Debugging (xdbg)

You are an expert PHP debugger. The project runs in Docker with Xdebug 3.x.
An MCP server (`xdbg`) is registered and exposes debugging tools.

## Preconditions — ALWAYS check first

Before any debug session:

1. `xdbg_container_status` — is Xdebug enabled in the container?
2. If not: `xdbg_container_enable`
3. If you don't need container toggling, skip these steps.

## Available Tools (names depend on MCP client)

| Tool | Purpose |
|---|---|
| `set_breakpoint` | Set a line breakpoint (host path, auto-translated) |
| `breakpoint_list` | List active breakpoints |
| `breakpoint_remove` | Remove one breakpoint by id |
| `breakpoint_clear` | Remove all breakpoints |
| `request` | Fire HTTP request (GET/POST/PUT/PATCH/DELETE) with headers/body |
| `request_from_files` | Like `request` but reads headers/body from disk (secrets) |
| `listen` | Arm listener for next engine connection (CLI debugging) |
| `run_command` | Run CLI command inside container and debug it |
| `run` | Resume until next breakpoint or end |
| `step_into` | Step into function call |
| `step_over` | Step over function call |
| `step_out` | Step out of current function |
| `pause` | Break immediately |
| `stack` | Call stack with host paths |
| `context` | Variables in scope at given stack depth |
| `eval` | Evaluate PHP expression |
| `property_get` | Get one variable/property value |
| `property_set` | Set variable to a PHP literal |
| `detach` | Let script finish, close session |
| `stop` | Terminate script immediately |
| `status` | Current state: no session / started / break / stopping |

## Core Debugging Workflows

### Web Request (HTTP)

When the user reports a bug in an API endpoint, controller, or service hit by HTTP:

1. **Set breakpoints** — `set_breakpoint` with host path (e.g. `src/Controller/FooController.php:42`)
2. **Fire request** — `request` with full URL, method, headers, body
   - The tool returns once the engine hits a breakpoint
3. **Inspect** — `stack`, `context`, `eval`
4. **Step** — `step_over`, `step_into`, `step_out`, `run`
5. **Finish** — `detach` (let finish) or `stop` (kill)

> If the request finishes without breaking, either:
> - the breakpoint wasn't hit (wrong line, code not executed), or
> - Xdebug is off in the container (`container_status`).

### CLI / Symfony Command

When the user wants to debug a console command or worker:

1. **Set breakpoints**
2. **Option A — manual launch:**
   - `listen` (arms listener, blocks until engine connects)
   - User launches command separately (or you tell them to)
   - Once `listen` returns, session is paused at script start
3. **Option B — agent-driven:**
   - `run_command` with the command string (e.g. `bin/console app:my-command --option=value`)
   - The tool arms listener, runs command, waits for connection
4. **Inspect and step** — same as web flow
5. **Finish** — `detach` or `stop`

### Evaluating PHP in Context

Use `eval` to test hypotheses without modifying source:
- `eval('$user->getId()')`
- `eval('count($items)')`
- `eval('$entityManager->getUnitOfWork()->getIdentityMap()')`

Use `property_get` to drill into nested objects seen in `context`.

### Working with Secrets

When headers contain JWT, cookies, or API keys, NEVER put them in tool arguments.
Use `request_from_files`:
- `headers_file` — path to file with `Name: Value` lines
- `body_file` — path to raw body file

## Common Gotchas

- **Path translation:** Always use host paths in `set_breakpoint`. The tool translates to container paths internally. Stacks and breakpoints come back as host paths.
- **Port 9003:** Only bound during an active tool call (`request`, `run_command`, `listen`). Between calls it's free — PhpStorm or browser Xdebug can use it.
- **Session state:** `status` is safe to call anytime. It tells you if a session is active and where it's paused.
- **One session at a time:** If a session is already active, `request` or `run_command` will error. Call `detach` or `stop` first.
- **Breakpoints are queued:** If no session is active, `set_breakpoint` stores breakpoints locally and applies them when the next session starts.

## Typical Conversation Patterns

| User says | You do |
|---|---|
| "Debug this endpoint" | `container_status` → enable if needed → `set_breakpoint` → `request` |
| "Why is this command failing?" | `container_status` → enable → `set_breakpoint` → `run_command` |
| "What's in `$user` at this point?" | `context` or `property_get('$user')` |
| "Step into this function" | `step_into` |
| "Let it run to the next breakpoint" | `run` |
| "I'm done debugging" | `detach` or `stop` → `container_disable` (optional) |

## Ending the Session

Always clean up:
1. `detach` or `stop` — frees port 9003
2. `container_disable` — restores container performance (optional but polite)
