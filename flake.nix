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
        pkgs = import nixpkgs {
          inherit system;
          config = {
            allowUnfree = false;
          };
        };

        lib = pkgs.lib;
        stdenv = pkgs.stdenv;

        isLinux = stdenv.isLinux;
        isDarwin = stdenv.isDarwin;

        nodejs = if pkgs ? nodejs_22 then pkgs.nodejs_22 else pkgs.nodejs_20;

        webkitgtk = if pkgs ? webkitgtk_4_1 then pkgs.webkitgtk_4_1 else pkgs.webkitgtk;

        libsoup = if pkgs ? libsoup_3 then pkgs.libsoup_3 else pkgs.libsoup;

      in
      let
        /*
          Shared developer tools.

          Keep this list limited to cross-platform CLI tools. Do not put Linux
          runtime libraries or Darwin Apple SDK frameworks here.
        */
        commonPackages = with pkgs; [
          # Rust
          rustc
          cargo
          rust-analyzer
          rustfmt
          clippy
          cargo-tauri

          # Native build tooling
          pkg-config
          llvmPackages.libclang
          cmake
          ninja
          gnumake

          # Frontend
          nodejs
          pnpm
          typescript

          # Go
          go
          go-task
          golangci-lint

          # Database
          sqlite
          dbmate

          # Utilities
          git
          jq
          python3
          curl
          unzip
          zip
        ];

        /*
          Platform compiler/tooling split.

          On Linux, gcc is useful and expected by several native dependencies.
          On Darwin, prefer the system Clang from Xcode / Command Line Tools
          instead of pulling GCC into the shell.
        */
        linuxTooling = lib.optionals isLinux (
          with pkgs;
          [
            gcc
            patchelf
          ]
        );

        darwinTooling = lib.optionals isDarwin (
          with pkgs;
          [
            libiconv
          ]
        );

        /*
          Linux runtime libraries required by Tauri/WebKitGTK and native playback.

          These are intentionally Linux-only. macOS uses system frameworks from
          Xcode / Command Line Tools and does not use Nix-provided Apple SDK
          frameworks, because legacy apple_sdk_11_0 stubs were removed from
          recent nixpkgs.
        */
        linuxRuntimeLibs = with pkgs; [
          # GTK / GNOME
          gtk3
          glib
          gdk-pixbuf
          pango
          cairo
          atk
          harfbuzz

          # Tauri / WebKitGTK
          webkitgtk
          libsoup
          openssl
          dbus
          alsa-lib
          fontconfig
          freetype
          libsecret
          libayatana-appindicator

          # X11 / Wayland compatibility
          libX11
          libXext
          libXcursor
          libXrandr
          libXi
          libXtst
          libxkbcommon
          wayland

          # OpenGL / GLX / EGL
          mesa
          libglvnd

          # Desktop integration
          gsettings-desktop-schemas
          adwaita-icon-theme
          hicolor-icon-theme

          # Phase 2 native playback local-dev runtime.
          #
          # Release installers still consume fyom-org/fork-mpv tarballs via
          # scripts/setup_runtime_libs.* and scripts/bundle_runtime_libs_*.
          mpv
          libass
        ];

        linuxPackages = lib.optionals isLinux linuxRuntimeLibs;

        /*
          Do not add darwin.apple_sdk.frameworks.* here.

          New nixpkgs versions removed the old apple_sdk_11_0 compatibility
          stub. Referencing darwin.apple_sdk.frameworks can force evaluation of
          the removed legacy SDK on some channels/systems.

          For fyom macOS development, Xcode / Command Line Tools provide AppKit,
          Foundation, Security, WebKit, CoreServices, SystemConfiguration, and
          related frameworks.
        */
        darwinPackages = lib.optionals isDarwin darwinTooling;

        linuxLibraryPath = lib.makeLibraryPath linuxRuntimeLibs;

        linuxPkgConfigPath =
          lib.makeSearchPathOutput "dev" "lib/pkgconfig" linuxRuntimeLibs
          + ":"
          + lib.makeSearchPathOutput "out" "lib/pkgconfig" linuxRuntimeLibs
          + ":"
          + lib.makeSearchPathOutput "out" "share/pkgconfig" linuxRuntimeLibs;

        linuxShellHook = lib.optionalString isLinux ''
          export RUST_BACKTRACE="1"

          export LIBCLANG_PATH="${pkgs.llvmPackages.libclang.lib}/lib"

          export LD_LIBRARY_PATH="${linuxLibraryPath}:''${LD_LIBRARY_PATH:-}"
          export PKG_CONFIG_PATH="${linuxPkgConfigPath}:''${PKG_CONFIG_PATH:-}"

          # Rust/Tauri Linux dev link fix.
          # pangocairo may not propagate fontconfig/freetype through the final
          # Rust binary link, while ld.bfd still requires those DSOs explicitly.
          export LIBRARY_PATH="${pkgs.fontconfig.lib}/lib:${pkgs.freetype}/lib:''${LIBRARY_PATH:-}"

          export NIX_LDFLAGS="-L${pkgs.fontconfig.lib}/lib \
          -L${pkgs.freetype}/lib \
          -lfontconfig \
          -lfreetype \
          ''${NIX_LDFLAGS:-}"

          export RUSTFLAGS="-C link-arg=-L${pkgs.fontconfig.lib}/lib \
          -C link-arg=-L${pkgs.freetype}/lib \
          -C link-arg=-lfontconfig \
          -C link-arg=-lfreetype \
          ''${RUSTFLAGS:-}"

          export XDG_DATA_DIRS="${pkgs.gsettings-desktop-schemas}/share/gsettings-schemas/${pkgs.gsettings-desktop-schemas.name}:${pkgs.adwaita-icon-theme}/share:${pkgs.hicolor-icon-theme}/share:''${XDG_DATA_DIRS:-}"
          export GIO_EXTRA_MODULES="${pkgs.dconf.lib}/lib/gio/modules"

          export PLAYWRIGHT_BROWSERS_PATH="${pkgs.playwright-driver.browsers}"
          export PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=true
          export PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
          export NIX_PLAYWRIGHT_VERSION="${pkgs.playwright-driver.version}"

          if pkg-config --exists mpv 2>/dev/null; then
            export MPV_LIB_DIR="$(pkg-config --variable=libdir mpv)"
          fi

          echo "fyom Nix dev shell"
          echo "  platform: linux"
          echo "  node: $(node --version 2>/dev/null || true)"
          echo "  go: $(go version 2>/dev/null || true)"
          echo "  rustc: $(rustc --version 2>/dev/null || true)"
          echo "  MPV_LIB_DIR: ''${MPV_LIB_DIR:-not set}"
        '';

        darwinShellHook = lib.optionalString isDarwin ''
          export RUST_BACKTRACE="1"
          export LIBCLANG_PATH="${pkgs.llvmPackages.libclang.lib}/lib"

          # Use the system Apple SDK from Xcode / Command Line Tools.
          # Do not depend on nixpkgs darwin.apple_sdk.frameworks.* here.
          if command -v xcrun >/dev/null 2>&1; then
            export SDKROOT="$(xcrun --sdk macosx --show-sdk-path)"
            export MACOSX_DEPLOYMENT_TARGET="13.0"
          fi

          # macOS native playback development uses fyom-org/fork-mpv runtime
          # artifacts instead of Nix-provided libmpv.
          #
          # Run one of:
          #   node scripts/setup_runtime_libs.mjs --platform macos
          #   node scripts/setup_runtime_libs.mjs --platform darwin
          #
          # depending on the platform names supported by the current scripts.
          unset MPV_LIB_DIR

          echo "fyom Nix dev shell"
          echo "  platform: darwin"
          echo "  node: $(node --version 2>/dev/null || true)"
          echo "  go: $(go version 2>/dev/null || true)"
          echo "  rustc: $(rustc --version 2>/dev/null || true)"
          echo "  SDKROOT: ''${SDKROOT:-not set}"
          echo "  MPV_LIB_DIR: intentionally unset on macOS"
        '';

      in
      {
        devShells.default = pkgs.mkShell {
          packages = commonPackages ++ linuxTooling ++ linuxPackages ++ darwinPackages;

          nativeBuildInputs = [
            pkgs.pkg-config
            pkgs.cmake
            pkgs.ninja
          ]
          ++ lib.optionals isLinux [
            pkgs.gcc
          ];

          buildInputs =
            lib.optionals isLinux linuxRuntimeLibs
            ++ lib.optionals isDarwin [
              pkgs.libiconv
            ];

          shellHook = linuxShellHook + darwinShellHook;
        };

        formatter = pkgs.nixfmt-rfc-style;
      }
    );
}
