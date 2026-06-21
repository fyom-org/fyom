{
  buildGoModule,
  fetchFromGitHub,
  webkitgtk_6_0,
  pkg-config,
  lib,
}:

buildGoModule (finalAttrs: {
  pname = "wails3";
  version = "3.0.0-alpha2.104";

  src = fetchFromGitHub {
    owner = "wailsapp";
    repo = "wails";
    tag = "v${finalAttrs.version}";
    hash = "sha256-gHsJKq9TS0wFbzykU+eBs0hZk/Ytyiz2FFp0t/gncpE=";
  };

  vendorHash = "sha256-cFAwRPI10xk0AcjJ7aqrm65c4Wy+WQpUV/CEB2Ll2eo=";

  nativeBuildInputs = [
    pkg-config
  ];

  buildInputs = [
    webkitgtk_6_0
  ];

  subPackages = [ "cmd/wails3" ];

  sourceRoot = "${finalAttrs.src.name}/v3";

  env.GOWORK = "off";

  postPatch = ''
    export GOWORK=off

    # Make parent source tree writable, then remove workspace files.
    chmod -R u+w ..
    rm -f ../go.work ../go.work.sum

    echo "GOWORK=$GOWORK"
    echo "Checking parent workspace files:"
    ls -la .. | grep 'go.work' || true
  '';

  preBuild = ''
    export GOWORK=off
    rm -f ../go.work ../go.work.sum

    echo "GOWORK=$GOWORK"
    go env GOWORK
  '';

  overrideModAttrs = oldAttrs: {
    env = (oldAttrs.env or { }) // {
      GOWORK = "off";
    };

    postPatch = ''
      export GOWORK=off

      chmod -R u+w ..
      rm -f ../go.work ../go.work.sum

      echo "GOWORK=$GOWORK"
      echo "Checking parent workspace files in go-modules derivation:"
      ls -la .. | grep 'go.work' || true
    '';

    preBuild = ''
      export GOWORK=off
      rm -f ../go.work ../go.work.sum

      echo "GOWORK=$GOWORK"
      go env GOWORK
    '';
  };

  proxyVendor = true;

  __structuredAttrs = true;

  meta = {
    description = "Build desktop applications using Go & Web Technologies";
    homepage = "https://wails.io";
    license = lib.licenses.mit;
    maintainers = with lib.maintainers; [
      Simon-Weij
      yonzilch
    ];
    mainProgram = "wails3";
    platforms = lib.platforms.linux;
  };
})
