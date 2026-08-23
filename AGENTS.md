# AGENTS.md

## Project overview

`ecvb` is a Go web application. Keep the implementation small, explicit, and
server-rendered.

## Required stack

- Use the Go version declared in `go.mod`.
- Build the server with Go's standard `net/http` facilities unless an existing
  project package provides the needed behavior.
- Render old-school HTML templates on the server with `html/template`.
- Use HTMX for server interactions and Alpine.js only for small, local browser
  behaviors. Do not introduce a client-side application framework or a JSON API
  when an HTML response is sufficient.
- Use `zombiezen.com/go/sqlite` directly. Do not use `database/sql`, `sql.DB`, or
  an ORM.
- Use `github.com/peterbourgon/ff/v4` for command-line flags and subcommands. Do
  not use Cobra or Viper.

## Commands

### `cmd/db`

`cmd/db` owns database creation and migrations and must include a subcommand for
seeding a database.

- Database creation is explicit; other commands must not create a database as a
  side effect.
- Never create a missing directory or parent path. Before creating a database,
  verify that its parent directory already exists and is a directory. Fail with
  a clear error otherwise.
- Migration and seed operations require the database file to exist unless the
  command explicitly creates it as part of the requested operation.
- Migrations must be ordered, repeatable to apply, and recorded in the database.
- Seed data should be deterministic and safe for the intended environment.

### `cmd/server`

`cmd/server` starts the HTTP server.

- The configured database file must already exist. Fail fast with a clear error
  if it or any required path is missing.
- Never create or migrate the database during server startup.

## Authentication and authorization

- Users are identified by email address. Trim surrounding whitespace and force
  email addresses to lowercase before validation, lookup, comparison, or
  persistence. Enforce uniqueness on the normalized value in SQLite.
- Registration is invite-only. Do not add public or implicit registration.
- Authenticate with short-lived, single-use magic links; there are no passwords.
  Store only a digest of a magic-link token, expire tokens, and invalidate them
  after successful use.
- Prefer opaque session tokens in secure, HTTP-only cookies for HTMX and normal
  browser requests. Store only a digest of each session token server-side.
- Set authentication cookies with `HttpOnly`, `Secure` in non-local
  environments, `SameSite=Lax`, a narrow `Path`, and an explicit expiry.
- Protect state-changing requests against cross-site request forgery. HTMX
  requests are not inherently CSRF-safe.
- Roles are exactly `administrator` and `non-administrator`. Keep authorization
  checks server-side and deny access by default. Hiding a control in HTML is not
  authorization.
- Do not log magic-link tokens, session tokens, cookies, or other credentials.

## HTTP and templates

- Handlers should parse and validate input, call application/database code, and
  render a response. Keep SQL and business rules out of templates.
- Return complete HTML pages for normal navigation and HTML fragments for HTMX
  requests where appropriate. Preserve a usable non-HTMX path when practical.
- Use semantic HTML and ordinary links and forms. Alpine state must remain local
  and disposable; server state is authoritative.
- Use appropriate status codes. Render validation failures as actionable HTML,
  preserving safe user input.
- Escape untrusted content through `html/template`; do not construct trusted
  HTML from user input.

## SQLite practices

- Keep SQL explicit and close to the package that owns the data operation.
- Use transactions for multi-step state changes such as accepting an invite,
  consuming a magic link, or applying a migration.
- Enable and rely on foreign-key enforcement for every connection.
- Check every SQLite result and error. Close statements, rows, and connections
  according to the ZombieZen driver APIs.
- Do not silently create files through SQLite open flags. Callers must choose
  explicitly whether creation is allowed, and only `cmd/db` creation paths may
  allow it.

## Code organization and style

- Keep `cmd/*` packages thin. Put reusable server, authentication, template, and
  database behavior in focused internal packages as those responsibilities
  emerge.
- Prefer the standard library and small, direct functions over frameworks,
  dependency injection containers, or speculative abstractions.
- Pass `context.Context` through request-scoped and database operations.
- Wrap errors with useful operation context while preserving the underlying
  error. Do not expose internal errors or secrets to users.
- Run `gofmt` on changed Go files.

## Verification

- Add focused tests for new behavior, especially email normalization,
  authorization boundaries, invite and magic-link consumption, session expiry,
  database path handling, and migrations.
- Use temporary directories in filesystem tests. Explicitly test that commands
  reject missing parent directories and that server startup rejects a missing
  database.
- Before finishing a change, run the narrowest relevant tests and then
  `go test ./...` when the repository state permits it.
