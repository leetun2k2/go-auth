# Observability and Operations

## TL;DR
- **Goal:** Make authentication health and failures visible with a small v1 operations surface.
- **Key decision:** Use structured JSON logs plus `/health` and `/ready`; defer metrics, tracing, dashboards, and alert thresholds.
- **Impact:** Operators can inspect provider, stage, and status without exposing authentication material.

## Operational signals

```mermaid
flowchart LR
    A[Shared Auth] --> H[/health]
    A --> R[/ready]
    A --> L[JSON Logs]
    L --> O[Operator]
```

## Purpose

Define the operational behavior needed to run the service, diagnose failures, and protect authentication material.

## Scope

### In scope

| Item | Readiness | Basis / action |
|---|---|---|
| Health and readiness | **Ready** | The service must expose whether it can safely serve auth traffic. |
| Structured JSON logs | **Ready** | Every auth event includes correlation ID, client ID, provider, stage, and status; use `none` outside provider flows. |
| Log handling | **Ready** | Authentication logs are access-restricted and retained for 30 days. |

### Out of scope

- Product analytics, user tracking, marketing attribution, and a general security information platform.
- Logging raw provider payloads, emails, tokens, codes, or session secrets.
- Per-user behavior scoring.
- Distributed tracing, including Jaeger and OpenTelemetry.
- Prometheus metrics, dashboards, alert rules, and alert thresholds.

## Responsibilities

- Report service health separately from readiness to accept authentication traffic.
- Emit structured JSON logs for login, provider-subject resolution, verified-email linking, code exchange, session validation, and logout.
- Include a non-secret correlation ID, public client ID, provider, stage, and status in every auth event.
- Redact provider credentials, authorization codes, JWTs, signing keys, secrets, emails, and raw provider payloads.

## Explicit non-responsibilities

- Decide product business health or access.
- Record raw identity claims for convenience.
- Provide v1 metrics, distributed traces, dashboards, or alert thresholds.

## User or system behavior

- **Healthy:** Health reports process availability; readiness reports critical dependency availability.
- **Provider outage:** New login may fail safely while existing valid sessions continue when their validation path is healthy.
- **Internal outage:** The service fails closed where identity or session correctness cannot be guaranteed.
- **Diagnostic event:** JSON logs show correlation ID, public client ID, provider, stage, and status without authentication material.

## Important edge cases

- A provider is slow but not fully unavailable.
- Redaction misses credentials embedded in URLs or upstream error bodies.
- Health stays green while the session or identity store cannot serve traffic.

## Dependencies on other specifications

- Observable outcomes from specs [04](04-provider-authentication.md) through [10](10-security-requirements.md).
- [01 Domain and boundaries](01-domain-and-boundaries.md) for operational ownership.

## Acceptance criteria

- Operators can distinguish Google, GitHub, identity-resolution, session, and internal failures.
- No log contains provider credentials, authorization codes, JWTs, session material, signing keys, secrets, emails, or raw provider payloads.
- Readiness reflects critical dependency failure.
- `/health` reports process availability and `/ready` reports whether critical dependencies can safely serve traffic.
- No v1 acceptance gate requires Jaeger, OpenTelemetry, Prometheus, dashboards, or alert thresholds.

## Fixed operations policy

- Retain authentication logs for 30 days with access limited to the service operations and security roles.
- `/ready` requires PostgreSQL connectivity and valid required provider/client configuration. A provider outage does not make existing-session validation unready.
- Metrics, tracing, dashboards, and numeric service-level targets are outside the v1 release gate.
