{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/release-23.11";
  };
  outputs = { nixpkgs, ... }: rec {
    packages = nixpkgs.lib.genAttrs [ "x86_64-linux" "x86_64-darwin" ] (system:
      let pkgs = import nixpkgs { inherit system; }; in {
        default = pkgs.buildGoModule {
          pname = "demo-signaling-server";
          version = "0.0.1";

          nativeBuildInputs = with pkgs; [
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc
          ];

          src = ./.;

          CGO_ENABLED = "0";
          GOFLAGS = "-ldflags='-extldflags=-static -w -s";

          vendorHash = "sha256-fxjQPK/6IWxnezix8aMMxw3+MZj8XxqnYD5Z9WUsdM4=";
        };
      });
    devShells = nixpkgs.lib.genAttrs [ "x86_64-linux" "x86_64-darwin" ] (system:
      let pkgs = import nixpkgs { inherit system; }; in {
        default = packages."${system}".default.overrideAttrs (prevAttrs: {
          GOFLAGS="";
          nativeBuildInputs = prevAttrs.nativeBuildInputs ++ (with pkgs; [
            gopls
            golangci-lint
            delve
            gosec
            go-outline
            gotools
            gomodifytags
            impl
            gotests
          ]);
        });
      });
  };
}
