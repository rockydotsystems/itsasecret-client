{
  description = "itsasecret - Go CLI (itsasecret / shh)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAll = nixpkgs.lib.genAttrs systems;
    in {
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
          app = name: cmd: {
            type = "app";
            program = toString (pkgs.writeShellScript name ''
              export PATH="${goBin}:$PATH"
              exec ${cmd}
            '');
          };
          runCli = ''go run ./cmd/itsasecret "$@"'';
        in {
          # `nix run .` / `.#default` / `.#dev` / `.#run` all run the CLI,
          # forwarding args after `--` to the binary, e.g.
          #   nix run .#default -- pull --project <id> --env production
          default = app "default" runCli;
          dev = app "dev" runCli;
          run = app "run" runCli;
          shh = app "shh" runCli;
          test = app "test" ''go test ./... "$@"'';
          build = app "build" ''go build -o itsasecret ./cmd/itsasecret "$@"'';
          lint = app "lint" ''golangci-lint run "$@"'';
        });
    };
}
