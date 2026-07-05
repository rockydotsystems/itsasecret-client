# Running the CLI against a local server

This guide explains how to point the `itsasecret` CLI (alias `shh`) at a
locally running `www` server instead of production (`https://itsasecret.dev`).
It is intended for local development and manual testing of the end-to-end
flow: authentication, transport encryption, and pulling values into `.env`
files, shell environments, and direnv.

## Prerequisites

The CLI talks to the `www` server over HTTP, and the server needs Postgres.
Both run from the `www/` directory via its nix flake:

```sh
cd ../www
docker compose up -d        # Postgres 17 (container: itsasecret-postgres)
nix run .#dev               # dev server on http://localhost:3000
```

Confirm the server is reachable before continuing:

```sh
curl -s http://localhost:3000/api/health
# {"status":"ok"}
```

You also need a verified account in the local database. Registration is
handled through the website (`/register`); with no `RESEND_API_KEY` configured,
the verification link is printed to the server terminal so the account can be
verified without email delivery. Unverified accounts are rejected by every
protected endpoint with `403 Email not verified`.

## Running and building the CLI

From the `client/` directory you can run the CLI directly through the flake,
forwarding arguments after `--`:

```sh
nix run . -- <args>          # default app; .#dev and .#run are equivalent
nix run . -- secret list --project <project-id>
```

Or build a standalone binary:

```sh
nix run .#build              # produces ./itsasecret
```

The built binary is not installed on `PATH` by default. Either reference it by
path or create an alias for the session:

```sh
alias shh="$PWD/itsasecret"
```

The examples below use `shh`; substitute `nix run . --` or the binary path as
you prefer. Note that direnv (`.envrc`) requires a real binary path, not the
`nix run` form or a shell alias.

## Pointing the CLI at local

The target server is set once per machine with `shh config` — run it bare for
an interactive menu, or set it directly:

```sh
shh config set api http://localhost:3000
# API server set to http://localhost:3000 for this machine.
# Run `shh login` to authenticate against it.
shh login
# Email:    you@example.com
# Password: ********
# Logged in.
```

Every command reads the server URL from the config from then on:

```sh
shh pull   --project <project-id> --env production --shell
shh secret list --project <project-id>
```

A repo can also pin its server by committing an `api =` line in
`.shh.project` — `shh config set api <url> --project` writes it (or pick
"this project" in the bare `shh config` menu). The project override beats the
global config, and `shh config get api` shows which one is in effect.

### Configuration file

`shh config` and `shh login` write `config.json` under the user config
directory (`$XDG_CONFIG_HOME/itsasecret/`, defaulting to
`~/.config/itsasecret/`):

| Field      | Purpose                                                                                          |
| ---------- | ------------------------------------------------------------------------------------------------ |
| `apiUrl`   | Server the CLI targets by default (set by `shh config set api`).                                  |
| `sessions` | Per-server sessions keyed by API URL — token, ECDH transport key, unwrapped org keys. Logging in to local doesn't disturb your production session (or vice versa). |

The file is written with `0600` permissions. To use a throwaway config without
touching your real one, override the config directory for the session:

```sh
export XDG_CONFIG_HOME=/tmp/itsasecret-dev
shh config set api http://localhost:3000
shh login
```

## Linking a directory

Instead of passing `--project`/`--env` on every command, pin the working
directory once. When logged in, run it bare to pick interactively:

```sh
shh link
# Select a project
# > www (gh6p5a84k3xvv8mdjlkrou7x)
#   client (m2k9d0q1x7v5p8n4j6r3t1wz)
#
# Select an environment
# > production
#   staging
#   skip — don't pin an environment
```

The pickers are arrow-key menus (charmbracelet/huh); single-option steps are
selected automatically. When stdin is not a terminal the pickers fall back to
numbered prompts read line by line.

Or non-interactively with flags:

```sh
shh link --project <project-id> --env staging
```

Either way this writes `.shh.project` (the project ID — commit it) and
`.shh.env` (the environment name — local to your machine, automatically added
to `.gitignore`). Commands look for both files in the current directory and
its parents, so linking a repo root covers every subdirectory. Explicit flags
always override the files. When not logged in, bare `shh link` prints what
the current directory resolves to.

## Command reference

All commands target a project (`--project <id>` or `.shh.project`) and an
environment (`--env <name>` or `.shh.env`, defaulting to `production`). Find a
project ID on the dashboard or in the database; IDs are short opaque strings
(e.g. `gh6p5a84k3xvv8mdjlkrou7x`).

| Command                              | Effect                                             |
| ------------------------------------ | -------------------------------------------------- |
| `shh config [set api <url>]`         | View/set the API server (menu when bare).          |
| `shh link --project <id> [--env <e>]`| Pin the directory to a project/environment.        |
| `shh pull`                           | Fetch vars + secrets into a file or shell.         |
| `shh reload`                         | Pull again, delivered the way the last pull was.   |
| `shh secret list`                    | List secret keys (values are never shown).         |
| `shh secret get <key>`               | Print one decrypted secret value.                  |
| `shh secret set <KEY=VALUE>`         | Set a secret (encrypted client-side before sync).  |
| `shh var get <key>`                  | Print one plaintext var value.                     |
| `shh var set <KEY=VALUE>`            | Set a plaintext var.                               |
| `shh fork --name <new>`              | Fork an environment, copying its vars and secrets. |

Secrets are end-to-end with respect to storage: the CLI encrypts under the
transport session key, the server re-encrypts under the org key at rest, and
`pull`/`get` reverse the process. The server never persists plaintext.

## Populating environments

### `.env` file

```sh
shh pull --project <project-id> --env production --out .env
```

Values are single-quoted, so spaces and shell metacharacters survive:

```sh
export WOAH='woah. LMAO'
export HOLA='nini :D'
```

Load it into the current shell with `source .env`.

### Shell (stdout)

`--shell` writes `export` lines to stdout instead of a file, for use with
`eval`:

```sh
eval "$(shh pull --project <project-id> --env production --shell)"
```

### Reloading

`shh pull` records how it delivered values in `.shh.project` (`pull = shell`
or `pull = file:<path>`, the path relative to the `.shh.project` directory).
`shh reload` repeats that delivery — no flags needed, from anywhere in the
tree:

```sh
shh reload                  # rewrites the same file the last pull wrote
eval "$(shh reload)"        # when the last pull was --shell
```

Reload always pulls the linked scope: the project from `.shh.project` and the
environment `shh link` wrote to `.shh.env` (production if none). Pulls that
override `--project`/`--env` one-off don't update the recorded delivery, so a
bare reload keeps reproducing the linked scope's last pull.

### direnv (`.envrc`)

Add the pull to `.envrc` so values load automatically on `cd`. direnv runs the
`.envrc` in a non-interactive shell, so reference the binary by absolute path —
a shell alias will not be visible:

```sh
# .envrc
eval "$(/absolute/path/to/itsasecret pull --project <project-id> --env production --shell)"
```

Then authorize it:

```sh
direnv allow
```

## Switching back to production

Point the config back at production:

```sh
shh config set api https://itsasecret.dev
```

Sessions are stored per server, so if you were logged in to production before,
it still works — no new `shh login` needed.

Or remove the config file to reset entirely:

```sh
rm ~/.config/itsasecret/config.json
```

## Troubleshooting

| Symptom                                   | Cause / fix                                                            |
| ----------------------------------------- | --------------------------------------------------------------------- |
| `login failed: ... HTTP 401`              | Wrong credentials, or the account does not exist in the local DB.     |
| `HTTP 403` (`Email not verified`)         | Verify the account first (link is printed to the server terminal).    |
| `not logged in to <url> — run shh login`  | No session for that server; run `shh login` (after `shh config set api` for local). |
| `environment "<name>" not found`          | The env name does not exist in that project; check `--env`.           |
| `get secret: HTTP 404`                    | The key does not exist in that environment.                           |
| Connection refused                        | The `www` dev server is not running on `http://localhost:3000`.       |
| direnv does not load values               | Use the CLI's absolute path in `.envrc`, not a shell alias.           |
