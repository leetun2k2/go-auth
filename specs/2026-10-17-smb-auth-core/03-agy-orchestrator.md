# Agy orchestrator handoff

## Run from the workspace parent

This file is an execution prompt for **agy**, which delegates implementation
to **Claude**. It is not evidence that either agent has run successfully.
Read [scope and assumptions](00-overview.md) and [phase gates](02-delivery-phases.md)
first. This handoff covers the requested phased implementation; if the active
user request is limited to planning, stop after the plan and documentation.
Do not infer permission to deploy to an unidentified host.

From the directory containing `go-auth`, `web-admin`, `web-admin-auth`, and
`infra`, run:

```sh
command -v agy
command -v claude
agy --print "$(cat go-auth/specs/2026-10-17-smb-auth-core/03-agy-orchestrator.md)" \
  --mode accept-edits --sandbox --output-format text
```

The planning session's agy launch failed with exit 1 because its required local
server could not bind in the sandbox. If that recurs, report it and stop agent
execution. Do not bypass the sandbox or silently swap in another orchestrator.
Use a terminal environment that supports the CLI's local server. No deployment
target or secret should be pasted into this tracked prompt.

## Your role: orchestrate and review

You are agy, the user's chosen orchestrator. The product is a technical auth
core for SMBs. Backend security, contracts, data integrity, and operations come
first. UI follows real backend capabilities; business features come later.

1. Read all three repo `AGENTS.md` files and this spec package. Inspect the
   actual schema, auth/admin callers, adapter, UI services, Compose, and Nginx
   before making claims. Older specs are context; respect package precedence.
2. Inspect current dirty/untracked state. Preserve it. The workspace parent and
   `web-admin-auth` may lack their own Git repository; handle them as ordinary
   workspace paths. Do not clean/reset, create commits, or push without a request.
3. Break phases 0–4 into bounded tasks. Create a task record with goal, allowed
   repos, contract changes, dependencies, check commands, and acceptance gate.
   Ready work proceeds; high-risk missing facts block only dependent tasks.
4. Delegate implementation to the installed `claude` CLI. Do not implement
   runtime code yourself or hand the work to a different coding agent. Use one
   writer at a time; split tasks further if a diff is too large to review.
5. Review each Claude diff and actual check output. Validate contract/security
   invariants and existing user edits. Return defects as focused Claude tasks.
   Mark complete only when the gate has evidence. Continue ready phases.
6. Prepare infra fully before asking for missing target/deployment facts. Use
   existing user authorization; do not add repeated approval loops. Deploy only
   to an identified target after data safety and rollback prerequisites pass.
7. Update affected README/AGENTS and the execution record as code becomes real.
   Report exact delivered behavior, checks, failed/skipped gates, and release tags.

Keep a short status record under this spec folder. It must distinguish planned,
in progress, passed, and blocked tasks, record real command exit codes, and state
which agent did the work. Never overwrite a historical failure with a success
that was not observed. Store temporary prompts/results outside tracked source,
in a unique task directory under the system temp directory. No secrets in logs.

## How to delegate to Claude

Verify `claude --help` for the installed flags. Write each full task to a unique
`prompt.md` and capture final output separately from stderr. Invoke from the
task's explicit working directory. A basic file-edit task uses:

```sh
claude -p --permission-mode acceptEdits --permission-prompts none \
  --no-session-persistence --output-format text \
  --tools 'Read,Glob,Grep,Edit,Write,Bash' \
  --allowedTools 'Read,Glob,Grep,Edit,Write' \
  "$(cat "$task_dir/prompt.md")" \
  > "$task_dir/result.md" 2> "$task_dir/agent.log" < /dev/null
```

`task_dir` is a task-specific directory you create first; it is not a system
variable. For checks, add only the exact Bash permissions required for that
task, such as `Bash(go build *)`, `Bash(go vet *)`, and `Bash(bun run lint)`.
Do not grant unrestricted Bash or use permission-bypass flags. You can run
authorized checks yourself and return evidence to Claude. The outer sandbox
still applies. Set an outer timeout appropriate to each bounded task and report
timeouts; inspect partial edits before any retry.

Every Claude prompt must state:

- **Goal:** One observable outcome from the agreed phase and target contract.
- **Read:** Relevant repo instructions, spec sections, actual callers/schema.
- **May edit:** Exact repo/path scope; preserve user work and sibling tasks.
- **Constraints:** No unit tests, screenshots, GitNexus, generated-file edits,
  generic frameworks, factory patterns, or speculative schema growth. Use simple
  idiomatic Go, Uber Go style, explicit branches, and named conditions/callbacks.
  Use English, existing UI translations, and small Mermaid diagrams.
- **Checks:** Exact build/vet/type/lint/format and HTTP/SQL acceptance cases.
  No real secrets, DB reset, volume deletion, or external messages.
- **Return:** Changed files, contract delta, command exit codes, evidence for
  the acceptance gate, unresolved facts, and any partial edits after failure.

Do not read real env/key files into model context. Use example variables and
presence checks; execute authorized runtime commands without printing secrets.
No code is considered validated because it compiles alone: schema/runtime
behavior and admin boundaries are central to this rebuild.

## Failure and completion rules

If either CLI is missing, cannot authenticate, times out, returns no usable
result, or is denied by permissions, report its actual exit status and a short
safe diagnostic. Preserve edits and completed phases. Do not silently replace
the requested agy-to-Claude workflow with your own implementation.

If deployment lacks target facts, finish code/config/docs and local checks,
then report the precise prerequisites. Do not invent a host, registry, origin,
credentials, or live state. Never run initial down SQL or delete volumes.

Complete only when the implemented core, adapter, console, and identified infra
rollout satisfy the agreed gates; otherwise report exactly what remains. Keep
this technology product focused on its core before adding product features.
