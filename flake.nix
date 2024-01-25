{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };
  outputs = { nixpkgs, ... }: {
    packages = nixpkgs.lib.genAttrs [ "x86_64-linux" "x86_64-darwin" ] (system:
      let pkgs = import nixpkgs { inherit system; }; in {
        default = pkgs.buildGoModule {
          pname = "demo-signaling-server";
          version = "0.0.1";

          src = ./.;

          vendorHash = "sha256-IUPGl5vHLyzbTVYsCLu4lIWoyq0h96deQ7q/nnVkPjc=";
        };
      });
    devShells = nixpkgs.lib.genAttrs [ "x86_64-linux" "x86_64-darwin" ] (system:
      let pkgs = import nixpkgs { inherit system; }; in {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go

            gopls
            golangci-lint
          ];
        };
      });
  };
}
