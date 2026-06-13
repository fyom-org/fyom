{
  description = "fyom project development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
        lib = pkgs.lib;

        nodejs = if pkgs ? nodejs_22 then pkgs.nodejs_22 else pkgs.nodejs_20;

        vite = if pkgs ? vite then pkgs.vite else pkgs.vitejs;

        webkitgtk = if pkgs ? webkitgtk_4_1 then pkgs.webkitgtk_4_1 else pkgs.webkitgtk;

        libsoup = if pkgs ? libsoup_3 then pkgs.libsoup_3 else pkgs.libsoup;

        commonPackages = with pkgs; [
          # -- Rust Toolchain --
          rustc
          cargo
          rust-analyzer
          rustfmt
          clippy
          cargo-tauri

          # -- Native build tooling --
          pkg-config
          llvmPackages.libclang

          # -- Frontend Tooling --
          nodejs
          pnpm
          typescript
          vite

          # -- Go Toolchain --
          go
          go-task
          golangci-lint
          gnumake

          # -- Database --
          sqlite
          dbmate

          # -- Utilities --
          git
          jq
          python3
          cmake
          ninja
        ];

        linuxPackages = lib.optionals pkgs.stdenv.isLinux (
          with pkgs;
          [
            gcc

            # -- Tauri / Linux System Dependencies --
            webkitgtk
            libsoup
            glib
            gtk3
            gdk-pixbuf
            pango
            cairo
            openssl
            dbus
            alsa-lib
            fontconfig
            libsecret
            libayatana-appindicator

            # -- NixOS Desktop Runtime Fixes --
            gsettings-desktop-schemas
            adwaita-icon-theme
          ]
        );

        darwinPackages = lib.optionals pkgs.stdenv.isDarwin (
          with pkgs;
          [
            libiconv

            darwin.apple_sdk.frameworks.AppKit
            darwin.apple_sdk.frameworks.Foundation
            darwin.apple_sdk.frameworks.Security
            darwin.apple_sdk.frameworks.WebKit
            darwin.apple_sdk.frameworks.CoreServices
            darwin.apple_sdk.frameworks.SystemConfiguration
          ]
        );

        linuxShellHook = lib.optionalString pkgs.stdenv.isLinux ''
          # Fix missing GTK/WebKit schemas and themes on NixOS.
          export XDG_DATA_DIRS=${pkgs.gsettings-desktop-schemas}/share/gsettings-schemas/${pkgs.gsettings-desktop-schemas.name}:${pkgs.adwaita-icon-theme}/share:$XDG_DATA_DIRS

          echo "🚀 fyom Linux dev shell loaded."
          echo "⚙️  Run 'task dev:desktop' for development"
          echo "⚙️  Run 'task build:desktop' for building"
        '';

        darwinShellHook = lib.optionalString pkgs.stdenv.isDarwin ''
          echo "🚀 fyom macOS dev shell loaded."
          echo "⚙️  Run 'task build:desktop' for building"
        '';

      in
      {
        devShells.default = pkgs.mkShell {
          packages = commonPackages ++ linuxPackages ++ darwinPackages;

          env = {
            LIBCLANG_PATH = "${pkgs.llvmPackages.libclang.lib}/lib";
          };

          shellHook = linuxShellHook + darwinShellHook;
        };
      }
    );
}
