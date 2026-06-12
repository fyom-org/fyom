{ pkgs ? import <nixpkgs> {} }:
pkgs.mkShell {
  buildInputs = with pkgs; [
    gcc
    pkg-config
    glib
    gtk3
    webkitgtk_4_1
    libsoup_3
    adwaita-icon-theme
    dbus
    openssl
  ];
}
