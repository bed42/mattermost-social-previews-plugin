---
name: td-integration-test
description: Write integration tests for the td-sync admin API using the TestHarness in internal/api/testharness_test.go. Use when asked to write, add, or fix integration tests for admin API endpoints (server, users, projects, events, snapshots, CORS, auth). The harness provides a real HTTP server, fluent state builder, and assertion helpers. Tests go in internal/api/admin_integration_test.go.
---

# td-sync Admin API Integration Tests

Write integration tests in `internal/api/admin_integration_test.go` using the harness in `internal/api/testharness_test.go`. Tests run against a real HTTP server on a random port with isolated temp databases.

## Quick Start Pattern

```go
func TestIntegration_DescriptiveName(t *testing.T) {
    t.Parallel()
    h := newTestHarness(t)
    state := h.Build().
        WithUser("user@test.com").
        WithAdmin("admin@test.com", "admin:read:server,sync").
        WithProject("proj1", "user@test.com").
        WithEvents("proj1", "user@test.com", 5).
        Done()

    token := state.AdminToken("admin@test.com")
    pid := state.ProjectID("proj1")

    var resp adminEventsResponse
    h.DoJSON("GET", fmt.Sprintf("/v1/admin/projects/%s/events", pid), token, nil, &resp)

    if len(resp.Data) != 5 {
        t.Fatalf("expected 5 events, got %d", len(resp.Data))
    }
}
```

## Harness API

See [references/harness-api.md](references/harness-api.md) for the complete API reference with all method signatures and detailed usage notes.

### Core

- `newTestHarness(t, ...func(*Config)) *TestHarness` -- real HTTP server, isolated DB, auto-cleanup
- `h.Do(method, path, token, body) *http.Response` -- real HTTP request (caller closes body)
- `h.DoJSON(method, path, token, body, &out) *http.Response` -- request + JSON decode (fatals on 4xx/5xx)

### State Builder

```go
h.Build().
    WithUser(email).                          // sync-scoped key
    WithAdmin(email, scopes).                 // admin key with scopes
    WithProject(name, ownerEmail).            // via API (owner must exist)
    WithMember(projectName, email, role).     // "owner"/"writer"/"reader"
    WithEvents(projectName, userEmail, count).// cycles issues/logs/comments
    WithSnapshot(projectName).                // triggers snapshot build
    WithAuthEvents(count).                    // inserts directly to DB
    WithRateLimitEvents(count).               // inserts directly to DB
    Done() // -> *TestState
```

Ordering matters: create users before projects, projects before members/events/snapshots.

### State Accessors

`state.UserToken(email)`, `state.UserID(email)`, `state.AdminToken(email)`, `state.ProjectID(name)`, `state.Harness()`

### Assertions

- `AssertStatus(t, resp, 200)` -- checks status, prints body on failure
- `AssertErrorResponse(t, resp, 403, "insufficient_admin_scope")` -- checks status + error code
- `ReadJSON[T](t, resp) T` -- generic JSON decode
- `AssertPaginated[T](t, resp, count, hasMore) PaginatedResponse[T]` -- checks paginated list
- `AssertCORSHeaders(t, resp, origin)` / `AssertNoCORSHeaders(t, resp)`
- `h.AssertRequiresAdminScope(t, method, path, wrongToken)` -- 403 + error code check

## Admin Scopes

| Scope | Endpoints |
|-------|-----------|
| `admin:read:server` | server/overview, server/config, rate-limit-violations, users, users/{id}, users/{id}/keys, auth/events |
| `admin:read:projects` | projects, projects/{id}, projects/{id}/members, sync/status, sync/cursors |
| `admin:read:events` | projects/{id}/events, projects/{id}/events/{seq}, entity-types |
| `admin:read:snapshots` | projects/{id}/snapshot/meta, projects/{id}/snapshot/query |
| `admin:export` | projects/{id}/events/export |

## Response Types

Internal types accessible from test files in package `api`:

- `serverOverviewResponse` -- server overview
- `serverConfigResponse` -- server config
- `adminEventsResponse` -- `{Data []adminEvent, HasMore bool}`
- `adminEvent` -- single event: `ServerSeq`, `EntityType`, `EntityID`, `ActionType`, `Payload`
- `adminSyncStatusResponse` -- `{EventCount, LastServerSeq, LastEventTime}`
- `adminCursorEntry` -- `{ClientID, LastEventID, LastSyncAt, DistanceFromHead}`
- `serverdb.AdminProject` -- project: `ID, Name, MemberCount, EventCount`
- `serverdb.AdminUser` -- user: `ID, Email, IsAdmin, ProjectCount`
- `serverdb.AdminProjectMember` -- `{UserID, Email, Role}`

## Rules

1. Always `t.Parallel()` -- each harness is isolated
2. Test name prefix: `TestIntegration_`
3. Config overrides via opts: `newTestHarness(t, func(cfg *Config) { cfg.CORSAllowedOrigins = []string{"https://x.com"} })`
4. First user created is auto-admin; consume with `h.CreateUser("first@test.com")` when testing non-admin denial
5. CORS tests need manual `http.NewRequest` since `Do` doesn't support custom headers
6. Run tests: `go test -v -run TestIntegration ./internal/api/`