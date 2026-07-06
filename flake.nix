{
  description = "itsasecret - Go CLI (itsasecret / shh)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAll = nixpkgs.lib.genAttrs systems;
    in {
      packages = forAll (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          # Stamped into internal/commands.Version, matching the release build.
          # A flake ref carries shortRev; a dirty/local tree falls back gracefully.
          version = self.shortRev or self.dirtyShortRev or "nix";
        in {
          default = pkgs.buildGoModule {
            pname = "itsasecret";
            inherit version;
            src = self;
            vendorHash = "sha256-xyewkVVw5Ug05fHMHaezy6jCZnS7BTv4GSuD0ihbGcM=";
            subPackages = [ "cmd/itsasecret" ];
            env.CGO_ENABLED = 0;
            ldflags = [
              "-s"
              "-w"
              "-X"
              "itsasecret.dev/cli/internal/commands.Version=${version}"
            ];
            # The CLI ships as `itsasecret` with a `shh` alias - same binary.
            postInstall = ''
              ln -s itsasecret $out/bin/shh
            '';
            meta = {
              description = "itsasecret / shh - encrypted env var & secret sync CLI";
              homepage = "https://itsasecret.dev";
              # No LICENSE file in the repo yet - leave license unset rather than
              # guess (an `unfree` guess would block installs). Add when chosen.
              mainProgram = "itsasecret";
              platforms = systems;
            };
          };
        });

      devShells = forAll (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in {
          default = pkgs.mkShell {
            packages = with pkgs; [ go gopls golangci-lint git ];
            shellHook = ''
              echo ""
              echo "itsasecret-client dev shell"
              echo "  shh ...                   # CLI, rebuilt each direnv reload (.direnv/bin/shh)"
              echo "  go run ./cmd/itsasecret   # run the CLI"
              echo "  go test ./...             # tests"
              echo "  go build -o itsasecret ./cmd/itsasecret"
              echo "  golangci-lint run         # lint"
              echo "  go mod tidy               # tidy deps"
              echo ""
            '';
          };
        });

      apps = forAll (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          goBin = pkgs.lib.makeBinPath [ pkgs.go pkgs.golangci-lint ];
          # Dev apps run from the working-tree source via `go run`, so they
          # reflect local edits. Args after `--` are forwarded.
          devApp = name: cmd: {
            type = "app";
            program = toString (pkgs.writeShellScript name ''
              export PATH="${goBin}:$PATH"
              exec ${cmd}
            '');
          };
          runCli = ''go run ./cmd/itsasecret "$@"'';
          # The built package's binary - no Go toolchain needed at runtime.
          cli = {
            type = "app";
            program = "${self.packages.${system}.default}/bin/itsasecret";
          };
        in {
          # `nix run .` and `nix run github:rockydotsystems/itsasecret-client`
          # run the compiled binary, forwarding args after `--`, e.g.
          #   nix run github:rockydotsystems/itsasecret-client -- pull --env production
          default = cli;
          shh = cli;
          # Source-tree runners for local development (`nix run .#dev -- ...`).
          dev = devApp "dev" runCli;
          run = devApp "run" runCli;
          test = devApp "test" ''go test ./... "$@"'';
          build = devApp "build" ''go build -o itsasecret ./cmd/itsasecret "$@"'';
          lint = devApp "lint" ''golangci-lint run "$@"'';
        });
    };
}
