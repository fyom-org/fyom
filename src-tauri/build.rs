//! fyom Tauri build script.
//!
//! Wires the pre-built libmpv (from `fyom-org/fork-mpv`, fetched by
//! `scripts/setup_runtime_libs.*` into `src-tauri/libs/mpv/`) into the link line.
//!
//! This is a CLEAN rewrite of soia's build.rs — it does NOT reference the closed-source
//! `libsoia_utils`, the `config.data` license token, or the `SOIA_API` XOR-decode logic
//! (all soia-specific, all stripped in fyom's fork-mpv). fyom links only `libmpv` and
//! lets the `libmpv2` crate handle the render-context + event API.
//!
//! The `MPV_LIB_DIR` env var (set in `.cargo/config.toml`) tells the `libmpv2` build
//! script where to find `mpv/client.h` + the dylib. This build script emits the final
//! `cargo:rustc-link-{search,lib}` directives so the linker resolves `libmpv.{dylib,so,dll}`.

use std::env;
use std::path::PathBuf;

fn main() {
    let manifest_dir = env::var("CARGO_MANIFEST_DIR").unwrap();
    let target_os = env::var("CARGO_CFG_TARGET_OS").unwrap_or_default();
    let target_triple = env::var("TARGET").unwrap_or_default();

    let mpv_lib_dir = PathBuf::from(&manifest_dir).join("libs").join("mpv");

    // On Windows the import library name varies (mpv.lib / mpv-2.lib / libmpv.dll.a / ...).
    let windows_link_name = if mpv_lib_dir.join("mpv.lib").exists() {
        Some("mpv")
    } else if mpv_lib_dir.join("mpv-2.lib").exists() {
        Some("mpv-2")
    } else if mpv_lib_dir.join("libmpv.lib").exists() {
        Some("libmpv")
    } else if mpv_lib_dir.join("libmpv-2.lib").exists() {
        Some("libmpv-2")
    } else if mpv_lib_dir.join("libmpv.dll.a").exists() || mpv_lib_dir.join("mpv.dll.a").exists() {
        Some("mpv")
    } else if mpv_lib_dir.join("libmpv-2.dll.a").exists() || mpv_lib_dir.join("mpv-2.dll.a").exists() {
        Some("mpv-2")
    } else {
        None
    };

    // Verify the runtime library is present.
    let has_runtime = match target_os.as_str() {
        "macos" => mpv_lib_dir.join("libmpv.dylib").exists() || mpv_lib_dir.join("libmpv.2.dylib").exists(),
        "windows" => windows_link_name.is_some(),
        "linux" => mpv_lib_dir.join("libmpv.so").exists()
            || mpv_lib_dir.join("libmpv.so.2").exists()
            || mpv_lib_dir.join("libmpv.so.1").exists(),
        _ => mpv_lib_dir.join("libmpv.dylib").exists(),
    };

    if !has_runtime {
        panic!(
            "\n[!] Cannot find libmpv runtime/import library for target '{}'.\n    \
             Run `node scripts/setup_runtime_libs.mjs --platform <p>` to fetch the fork-mpv tarball,\n    \
             or install libmpv system-wide and remove the MPV_LIB_DIR override in .cargo/config.toml.\n",
            target_triple
        );
    }

    // Emit link directives for fyom's own final link step.
    println!("cargo:rustc-link-search=native={}", mpv_lib_dir.display());
    if target_os == "windows" {
        let link_name = windows_link_name.unwrap_or("mpv");
        println!("cargo:rustc-link-lib={}", link_name);
    } else {
        println!("cargo:rustc-link-lib=dylib=mpv");
    }

    // Re-run if the libs dir or setup scripts change.
    println!("cargo:rerun-if-changed=libs/mpv");
    println!("cargo:rerun-if-changed=../scripts/setup_runtime_libs_macos.sh");
    println!("cargo:rerun-if-changed=../scripts/setup_runtime_libs_linux.sh");
    println!("cargo:rerun-if-changed=../scripts/setup_runtime_libs_windows.mjs");
    println!("cargo:rerun-if-changed=../scripts/setup_runtime_libs.mjs");
    println!("cargo:rerun-if-env-changed=TARGET");

    tauri_build::build();
}
