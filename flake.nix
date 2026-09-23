{
  description = "onerep - self-hosted gym progress tracker";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            # Go toolchain
            go
            gopls
            gotools
            golangci-lint
            templ
            air

            # Frontend (tailwind standalone CLI; node only for JS tests of shared calc cases)
            tailwindcss_4
            nodejs_24

            # Database and migrations
            sqlite
            atlas

            # Build, run, auth
            ko
            go-task
            dex-oidc
          ];

          env = {
            CGO_ENABLED = "0";
            ONEREP_ENV = "dev";
          };
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt-rfc-style);
    };
}
