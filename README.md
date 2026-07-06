# itsasecret CLI (`shh`)

Sync encrypted secrets and env vars across every environment your team ships to - production, staging, per-developer forks - straight from your terminal.

The binary is `itsasecret`, aliased to `shh`. Values are encrypted on your machine before they ever leave it, so the server only ever stores ciphertext.

- **End-to-end encrypted** | every secret is sealed on-device with a key derived from your master password.
- **Environment-aware** | one command pulls the right values for production, staging, or your own fork.
- **Shell-native** | write a `.env` file or load values straight into your shell, with first-class direnv support.

Full documentation lives at [itsasecret.dev/docs](https://itsasecret.dev/docs). How the crypto works is at [itsasecret.dev/how-it-works](https://itsasecret.dev/how-it-works).

## Install

### One line (recommended)

```sh
curl -fsSL https://itsasecret.dev/install.sh | sh
```

Installs the `itsasecret` binary and its `shh` alias to `~/.local/bin` for linux and macOS on amd64 and arm64. The script verifies a sha256 checksum before anything lands on disk. Override the destination with `SHH_INSTALL_DIR`.

Prefer to read it first? The same URL prints the script to your terminal.

### Nix

The repo is a flake, so you can run or install it without cloning:

```sh
# run it once without installing
nix run github:rockydotsystems/itsasecret-client -- --version

# install into your profile
nix profile install github:rockydotsystems/itsasecret-client
```

Or add it as an input and pull the package into `environment.systemPackages` (NixOS) or `home.packages` (home-manager):

```nix
# flake.nix
{
  inputs.itsasecret.url = "github:rockydotsystems/itsasecret-client";
  # environment.systemPackages = [ inputs.itsasecret.packages.${pkgs.system}.default ];
  # home.packages          = [ inputs.itsasecret.packages.${pkgs.system}.default ];
}
```

### From source

Requires Go 1.26+.

```sh
git clone https://github.com/rockydotsystems/itsasecret-client
cd itsasecret-client
go build -o itsasecret ./cmd/itsasecret
```

## Quick start

```sh
shh login                 # authenticate (once per machine)
shh link                  # pick an org, project, and environment
shh pull                  # decrypt into ./.env
```

Load values straight into your current shell instead of a file:

```sh
eval "$(shh pull --shell)"
```

Wire it into [direnv](https://direnv.net) by dropping that line into an `.envrc` - your secrets load the moment you enter the directory, and nothing is written to disk.

## Commands

| Command | What it does |
| --- | --- |
| `shh login` | Authenticate; your key is derived on your machine. |
| `shh link` | Pin the current directory to a project and environment. |
| `shh pull` | Decrypt values into a file (`--out`) or your shell (`--shell`). |
| `shh reload` | Pull again, delivered the way the last pull was. |
| `shh secret set KEY=VALUE` | Set a secret, encrypted before it leaves your machine. |
| `shh secret get <key>` | Print one decrypted secret value. |
| `shh secret list` | List secret keys (values are never shown). |
| `shh var set KEY=VALUE` | Set a plaintext config var. |
| `shh var get <key>` | Print one plaintext var value. |
| `shh fork --name <new>` | Fork an environment, copying its vars and secrets. |
| `shh auth <token>` | Authenticate a headless machine with a long-lived token. |

Every environment-scoped command accepts `--project <id>` and `--env <name>` to override the linked scope.

## Headless machines (CI, servers, containers)

Create a long-lived access token from the **Tokens** page in the dashboard, then authenticate with it - no master password on the machine:

```sh
shh auth shht_...
```

Token sessions don't roll or idle out; they last until the expiry you picked or until you revoke them.

## Platforms

linux and macOS, amd64 and arm64. Windows is not supported yet.
