{
  description = "fyom project development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      nixpkgs,
      flake-utils,
      ...
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

        nodejs =
          if pkgs ? nodejs_24 then
            pkgs.nodejs_24
          else if pkgs ? nodejs_26 then
            pkgs.nodejs_26
          else
            pkgs.nodejs;

        libsoup = if pkgs ? libsoup_3 then pkgs.libsoup_3 else pkgs.libsoup;

        /*
          Local Wails v3 package.

          Important:
          - ./nix/wails3.nix depends on Linux WebKitGTK.
          - Keep it Linux-only unless you write a separate Darwin derivation.
        */
        wails3 = pkgs.callPackage ./nix/wails3.nix { };

        /*
          Shared developer tools.

          Keep this list limited to cross-platform CLI tools.
          Do not put Linux runtime libraries or Darwin Apple SDK frameworks here.
        */
        commonPackages = with pkgs; [
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
          curl
          git
          jq
          python3
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
          Linux runtime libraries required by Wails/WebKitGTK and native playback.

          This must stay Linux-only. macOS uses system frameworks from
          Xcode / Command Line Tools and does not use Nix-provided Apple SDK
          frameworks here.
        */
        linuxRuntimeLibs = lib.optionals isLinux (
          with pkgs;
          [
            # GTK / GNOME
            gtk3
            glib
            gdk-pixbuf
            cairo
            atk
            harfbuzz

            # WebKitGTK / networking / native integration
            webkitgtk_6_0
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
            dconf
          ]
        );

        linuxPackages = lib.optionals isLinux (
          linuxRuntimeLibs
          ++ [
            wails3
          ]
        );

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
          export LIBCLANG_PATH="${pkgs.llvmPackages.libclang.lib}/lib"
          export LD_LIBRARY_PATH="${linuxLibraryPath}:''${LD_LIBRARY_PATH:-}"
          export PKG_CONFIG_PATH="${linuxPkgConfigPath}:''${PKG_CONFIG_PATH:-}"

          # Linux dev link fix.
          # Binary link, while ld.bfd still requires those DSOs explicitly.
          export LIBRARY_PATH="${pkgs.fontconfig.lib}/lib:${pkgs.freetype}/lib:''${LIBRARY_PATH:-}"

          export NIX_LDFLAGS="-L${pkgs.fontconfig.lib}/lib \
          -L${pkgs.freetype}/lib \
          -lfontconfig \
          -lfreetype \
          ''${NIX_LDFLAGS:-}"

          export XDG_DATA_DIRS="${pkgs.gsettings-desktop-schemas}/share/gsettings-schemas/${pkgs.gsettings-desktop-schemas.name}:${pkgs.adwaita-icon-theme}/share:${pkgs.hicolor-icon-theme}/share:''${XDG_DATA_DIRS:-}"
          export GIO_EXTRA_MODULES="${pkgs.dconf.lib}/lib/gio/modules"

          export PLAYWRIGHT_BROWSERS_PATH="${pkgs.playwright-driver.browsers}"
          export PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=true
          export PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
          export NIX_PLAYWRIGHT_VERSION="${pkgs.playwright-driver.version}"

          echo "fyom Nix dev shell"
          echo "  platform: linux"
          echo "  node: $(node --version 2>/dev/null || true)"
          echo "  go: $(go version 2>/dev/null || true)"
          echo "  wails3: $(wails3 version 2>&1 | sed -n '1p')"
        '';

        darwinShellHook = lib.optionalString isDarwin ''
          export LIBCLANG_PATH="${pkgs.llvmPackages.libclang.lib}/lib"

          # Use the system Apple SDK from Xcode / Command Line Tools.
          # Do not depend on nixpkgs darwin.apple_sdk.frameworks.* here.
          if command -v xcrun >/dev/null 2>&1; then
            export SDKROOT="$(xcrun --sdk macosx --show-sdk-path)"
            export MACOSX_DEPLOYMENT_TARGET="13.0"
          fi

          echo "fyom Nix dev shell"
          echo "  platform: darwin"
          echo "  node: $(node --version 2>/dev/null || true)"
          echo "  go: $(go version 2>/dev/null || true)"
          echo "  SDKROOT: ''${SDKROOT:-not set}"
        '';
      in
      {
        packages = lib.optionalAttrs isLinux {
          wails3 = wails3;
          default = wails3;
        };

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
