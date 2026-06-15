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
          # Rust
          rustc
          cargo
          rust-analyzer
          rustfmt
          clippy
          cargo-tauri

          # Native tooling
          pkg-config
          llvmPackages.libclang
          gcc
          cmake
          ninja

          # Frontend
          nodejs
          pnpm
          typescript
          vite

          # Go
          go
          go-task
          golangci-lint
          gnumake

          # Database
          sqlite
          dbmate

          # Utilities
          git
          jq
          python3
        ];

        linuxRuntimeLibs = with pkgs; [
          # GTK / GNOME
          gtk3
          glib
          gdk-pixbuf
          pango
          cairo

          # Tauri
          webkitgtk
          libsoup
          openssl
          dbus
          alsa-lib
          fontconfig
          libsecret
          libayatana-appindicator

          # X11 / Wayland compatibility
          libX11
          libXcursor
          libXrandr
          libXi
          libXtst
          libxkbcommon

          # Desktop integration
          gsettings-desktop-schemas
          adwaita-icon-theme
        ];

        linuxPackages = lib.optionals pkgs.stdenv.isLinux linuxRuntimeLibs;

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
          export FYOM_BIN="build/fyom"
          export RUST_BACKTRACE="1"
          export LIBCLANG_PATH="${pkgs.llvmPackages.libclang.lib}/lib"
          export LD_LIBRARY_PATH="${lib.makeLibraryPath linuxRuntimeLibs}:$LD_LIBRARY_PATH"
          export XDG_DATA_DIRS="${pkgs.gsettings-desktop-schemas}/share/gsettings-schemas/${pkgs.gsettings-desktop-schemas.name}:${pkgs.adwaita-icon-theme}/share:$XDG_DATA_DIRS"
          export GIO_EXTRA_MODULES="${pkgs.dconf.lib}/lib/gio/modules"

          export PLAYWRIGHT_BROWSERS_PATH=${pkgs.playwright-driver.browsers}
          export PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=true
          export PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
          echo "✅ Vitest Browser Mode (Playwright) ready on NixOS"
        '';

        darwinShellHook = lib.optionalString pkgs.stdenv.isDarwin ''
          export LIBCLANG_PATH="${pkgs.llvmPackages.libclang.lib}/lib"
        '';

      in
      {
        devShells.default = pkgs.mkShell {
          packages = commonPackages ++ linuxPackages ++ darwinPackages;

          nativeBuildInputs = lib.optionals pkgs.stdenv.isLinux [
            pkgs.pkg-config
          ];

          buildInputs = lib.optionals pkgs.stdenv.isLinux linuxRuntimeLibs;

          shellHook = linuxShellHook + darwinShellHook;
        };
      }
    );
}
