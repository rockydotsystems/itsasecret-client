# AGENTS.md — client (itsasecret CLI)

## What this is

The Go CLI for itsasecret. Binary name `itsasecret`, aliased to `shh`. Syncs
env vars/secrets across environments, populates `.env` files or shell env
(direnv integration). Full product/architecture docs live in `../docs/`.

## Tech stack

- **Language**: Go (single static binary, cross-compilable)
- **CLI framework**: cobra
- **Interactive prompts**: charmbracelet/huh (select TUI; falls back to huh's
  accessible numbered prompts when stdin isn't a TTY, e.g. pipes/tests)
- **Crypto**: stdlib + vetted third-party (Argon2id, AES-GCM/XChaCha20-Poly1305)
- **API client**: generated from the www Worker's OpenAPI spec (`@hono/zod-openapi` on the server side → `oapi-codegen` or hand-written typed client on this side)

## Commands (via nix flake)

```
nix develop                    # enter dev shell (go, gopls, golangci-lint)
nix run . -- <args>            # run the CLI (default app); also .#dev / .#run
nix run .#test                 # go test ./...
nix run .#build                # go build -o itsasecret ./cmd/itsasecret
nix run .#lint                 # golangci-lint run
go mod tidy                    # tidy deps (inside dev shell)
```

## Running against a local server

To point the CLI at a locally running `www` server instead of production, see
[`docs/local-server.md`](docs/local-server.md). In short: start the `www` dev
server, then `shh config set url http://localhost:3000` and `shh login` — the
URL persists to the config file, so later commands need no flag.

## CLI behavior (from docs/product-spec.md)

- Binary name `itsasecret`, alias `shh`.
- `shh secret set KEY=VALUE` / `shh var set KEY=VALUE` — values are set one at
  a time (secrets encrypted client-side, vars plaintext); there is
  deliberately **no bulk `push`**. Read back with `shh secret get <key>` /
  `shh var get <key>`.
- `shh pull --shell --project <project-id>` — populate env vars directly into shell (for `.envrc`/direnv).
- `shh reload` — pull again, delivered the way the last `shh pull` here was.
  `pull` records its delivery in `.shh.project` (`pull = shell` or
  `pull = file:<path>`, path relative to the `.shh.project` dir), so reload
  writes to the same place from anywhere in the tree; shell mode re-emits
  exports for `eval "$(shh reload)"`. Reload always targets the linked scope
  (project from `.shh.project`, env from `.shh.env` as set by the last
  `shh link`, defaulting to production), and only pulls of that linked scope
  update the record — one-off `--project`/`--env` overrides don't.
- `shh config` — view/set the API server. Set once per machine (global
  `config.json`), or per repo via a `url =` line in `.shh.project`
  (committed; for self-hosted servers). Bare `shh config` is an interactive
  menu: first an action picker (set the server URL / show the current
  configuration — new actions slot in here as settings grow), then the chosen
  flow. Session status is **verified live** against `/api/auth/me` (5s
  timeout), not guessed from the config file: show lists each stored session
  as "logged in as <email> (session verified)" / "session expired" /
  "couldn't verify (<err>)", and setting a URL ends with the same check for
  that server. `shh config set url <url> [--project]` and
  `shh config get url` are the direct forms. Resolution: `.shh.project` >
  global > default. `shh login` has **no `--api` flag** — it uses the same
  resolution. Sessions are stored **per server** (`sessions` map in
  config.json, keyed by canonical API URL; legacy flat fields migrate on
  load), so logins against production, self-hosted, and local dev coexist and
  every command picks the session matching its resolved URL.
- `shh link --project <id> [--env <name>]` — pins a directory to a project/
  environment by writing `.shh.project` (committed, `key = value` lines —
  `project`, optional `url` (legacy `api` alias parses), optional `pull`; a legacy bare-ID file still
  parses) and `.shh.env` (local, auto-added to `.gitignore`). Commands resolve scope as flag > `.shh.*` file
  (found by walking up from cwd, each file independently) > `production` for
  env. `shh link` with no flags links interactively when logged in (numbered
  org → project → env picker; env skippable); otherwise it prints the current
  resolution.
- Can populate a file (default `.env`) with exported secret values.
- Can do most things the website can: set values, view them, fork environments, etc.
- Project IDs are short opaque IDs (nanoid-style, e.g. `heyq1dpc`). Environment selected by flag/branch-name, defaults to `production`.

## Key decisions (from docs/)

- **Auth**: master password → Argon2id KDF (server-side at login) → unwrap org
  shared keys. CLI sessions are **rolling** (server kind='cli'): valid 30
  minutes, and every successful command's response carries a fresh token
  (`X-New-Session-Token` + `X-Session-Expires-At`; old token keeps a 60s
  grace window) which the client persists immediately (`clientFor` token
  saver — losing it locks the session out). After ~30 idle minutes the next
  command prompts inline for the master password (`ensureSession` →
  `promptLogin`; email is remembered per server) and the full re-login also
  refreshes org keys. The prompt uses `promptIO`: stdin/stdout when both are
  terminals, otherwise it opens `/dev/tty` directly (sudo-style) so captured
  stdout (direnv, `eval "$(shh pull --shell)"`) stays clean and piped stdin
  still gets a TUI prompt. Only headless runs (no controlling terminal) fail:
  "session expired and no terminal is available…". A mid-command 401 (rolled
  token lost before it was saved, revocation, server-side expiry) triggers
  the same unlock prompt and retries the request once (`authedClient` →
  `api.Client.WithReauth`); server-side, rotation is a compare-and-swap on
  the current token hash so concurrent commands can't clobber each other's
  tokens (the loser's request still succeeds via the grace window).
- **Local storage security**: config.json never holds unwrapped org keys —
  only master-password-wrapped blobs (`wrappedOrgKeys`, from login's
  `masterWrappedOrgKeys`); legacy plaintext `orgKeys` are scrubbed on load.
  The stored token + transport sessionKey are only useful for ≤30 minutes.
- **Transport**: per-session ECDH key negotiated at login; server re-encrypts secrets with it. CLI holds the session key to decrypt.
- **RBAC roles**: `read`, `write`, `admin` at environment level.

## Repo layout

```
client/
  flake.nix              # nix dev shell + apps
  go.mod
  cmd/
    itsasecret/          # main entry (binary: itsasecret)
  internal/
    api/                 # HTTP client, typed API surface
    auth/                # login, token storage, KDF
    crypto/              # Argon2id, AES-GCM, ECDH, envelope encrypt/decrypt
    config/              # config file (~/.config/itsasecret/)
    localcfg/            # per-directory .shh.project / .shh.env marker files
    commands/            # cobra command tree (pull, secret, var, fork, login, link, ...)
```

## Version control

This repo uses **jj** (Jujutsu). Use `jj new` to create new revisions — do not
write descriptions (the repo owner handles that). Don't use `git commit`.

## Conventions

- Idiomatic Go, `internal/` for unexported packages.
- No secrets/tokens logged or printed.
- Use `crypto/rand` for all random generation, never `math/rand`.
- Config stored at `~/.config/itsasecret/` (XDG-compatible).
- Alias `shh` is a symlink or wrapper to the `itsasecret` binary.
