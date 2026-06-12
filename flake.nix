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

        # Compatibility fallbacks for different nixpkgs branches
        webkitgtk = if pkgs ? webkitgtk_4_1 then pkgs.webkitgtk_4_1 else pkgs.webkitgtk;
        libsoup = if pkgs ? libsoup_3 then pkgs.libsoup_3 else pkgs.libsoup;
        nodejs = if pkgs ? nodejs_22 then pkgs.nodejs_22 else pkgs.nodejs_20;

      in
      {
        devShells.default = pkgs.mkShell {
          packages = [
            webkitgtk
            libsoup
            nodejs
          ]
          ++ (with pkgs; [
            # -- Rust Toolchain --
            rustc
            cargo
            rust-analyzer
            rustfmt
            clippy
            pkg-config
            gcc

            # -- Tauri / Linux System Dependencies --
            cargo-tauri
            glib
            gtk3
            gdk-pixbuf
            pango
            cairo
            openssl
            dbus
            alsa-lib
            fontconfig
            libsecret # Required for Tauri keyring/api password storage
            libayatana-appindicator # Required for system tray icons

            # -- NixOS Desktop Runtime Fixes --
            gsettings-desktop-schemas # Prevents GLib-GIO-ERROR on startup
            adwaita-icon-theme

            # -- Frontend Tooling --
            pnpm
            typescript
            vitejs

            # -- Go Toolchain --
            go
            go-task
            golangci-lint
            air
            gnumake

            # -- Database --
            sqlite
            dbmate

            # -- Utilities --
            git
            gitui
            onefetch
            jq
            just
            rclone
            wrangler
            python3
            cmake
            ninja
          ]);

          shellHook = ''
            # Fix missing GTK/WebKit schemas and themes on NixOS
            export XDG_DATA_DIRS=${pkgs.gsettings-desktop-schemas}/share/gsettings-schemas/${pkgs.gsettings-desktop-schemas.name}:${pkgs.adwaita-icon-theme}/share:$XDG_DATA_DIRS

            echo "🚀 fyom dev shell loaded."
            echo "⚙️  Run 'task dev:desktop' for development"
            echo "⚙️  Run 'task build:desktop' for building"
          '';
        };
      }
    );
}
