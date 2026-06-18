//! Fuzzy external subtitle discovery — ports the algorithmic core of soia's
//! `subtitles.rs` for LOCAL filesystem media.
//!
//! PORTED_FROM_SOIA @ <2025-Q4> (`src-tauri/src/subtitles.rs`, GPL-3.0-only)
//!
//! Adapted for fyom:
//! - soia's `PlaybackSource` enum (Local / Webdav / Dlna / Smb / DirectSmbUrl) + the
//!   `network_subtitle_matches` / `resolve_webdav_subtitle_url` / SMB / DLNA paths are
//!   **dropped** — those network protocols are deferred modules in fyom (see ROADMAP
//!   "Deferred modules: soia `network/protocols/{smb,dlna,webdav}.rs`"). fyom is a media
//!   catalog + resource dispatcher: the server issues presigned URLs and the client
//!   streams directly; subtitle discovery only makes sense for the LOCAL-desktop case
//!   where the Tauri app runs on the same machine as the media library (e.g. a Home
//!   Theater PC running fyom desktop against a local fyom server).
//! - The pure string-matching algorithm (`subtitle_match_score` + `normalize_match_text` +
//!   `tokenize_match_text` + `extract_episode_keys` + `extract_year_keys` +
//!   `matching_subtitle_entries` + `is_subtitle_file` + `is_matching_subtitle_folder` +
//!   `nested_subtitle_match_name` + `has_boundary_prefix`) is ported **verbatim** — it
//!   has zero soia-specific dependencies.
//! - The Tauri command `find_fuzzy_external_subtitle_matches` is renamed to
//!   `find_external_subtitles` (clearer for fyom's command surface) and takes a local
//!   filesystem path directly (no `PlaybackSource` parsing).
//!
//! See `docs/libmpv-assessment.md` §2.1 for the reuse inventory.

use std::path::Path;

use serde::{Deserialize, Serialize};

// ---------------------------------------------------------------------------
// Payload types
// ---------------------------------------------------------------------------

/// Incoming request payload for `find_external_subtitles`. Re-exported as
/// `ExternalSubtitleMatchesPayloadResolved` for the `commands::playback` adapter.
#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ExternalSubtitleMatchesPayload {
    /// Absolute filesystem path to the media file (must exist + be a file).
    pub media_path: String,
    /// Optional media title (NFO metadata) — used as the primary match candidate when
    /// the on-disk filename is unhelpful (e.g. `S01E01.mkv` → title `Pilot`).
    pub media_title: Option<String>,
}

/// Alias used by `commands::playback::find_external_subtitles` to construct the payload
/// from positional invoke args (keeps the Tauri command signature flat for the frontend).
pub type ExternalSubtitleMatchesPayloadResolved = ExternalSubtitleMatchesPayload;

/// A single matched external subtitle file (absolute filesystem path).
#[derive(Serialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct ExternalSubtitleMatch {
    /// Absolute filesystem path to the subtitle file.
    pub path: String,
    /// Match score 0.0–100.0 (higher = better match). Useful for the frontend to
    /// surface "best match" first or filter out weak matches.
    pub score: f64,
    /// Display name (file stem) for the subtitle — convenient for the frontend's
    /// subtitle picker UI without re-parsing the path.
    pub label: String,
}

/// Internal intermediate type used during scoring.
#[derive(Clone)]
struct SubtitleMatchEntry {
    name: String,
    path: String,
}

// ---------------------------------------------------------------------------
// Constants (ported verbatim from soia)
// ---------------------------------------------------------------------------

/// File extensions recognized as subtitles (ported verbatim from soia).
const SUBTITLE_FILE_EXTENSIONS: &[&str] = &[
    "srt", "ass", "ssa", "vtt", "sub", "idx", "sup", "smi", "smil", "lrc", "ttml", "dfxp",
];

/// Tokens ignored when scoring subtitle matches (release-group / codec / language tags
/// that don't help match a subtitle to its media). Ported verbatim from soia.
const IGNORED_SUBTITLE_MATCH_TOKENS: &[&str] = &[
    "1080p", "2160p", "720p", "480p", "4k", "8k", "web", "webrip", "webdl", "web-dl", "dl",
    "bluray", "bdrip", "hdrip", "dvdrip", "hdtv", "x264", "x265", "h264", "h265", "hevc",
    "avc", "aac", "dts", "ddp", "atmos", "proper", "repack", "remux", "extended", "internal",
    "multi", "chs", "cht", "chi", "zho", "zh", "cn", "gb", "big5", "eng", "en", "jpn", "jp",
    "kor", "kr", "sc", "tc",
];

// ---------------------------------------------------------------------------
// Path helpers (pure, no soia `playback_source` dependency)
// ---------------------------------------------------------------------------

/// Lowercased file extension (no leading dot), or empty string if none.
fn path_extension(path: &str) -> String {
    Path::new(path)
        .extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| ext.to_lowercase())
        .unwrap_or_default()
}

/// Final path segment (file name with extension), or the original string if none.
fn path_file_name(path: &str) -> String {
    Path::new(path)
        .file_name()
        .and_then(|name| name.to_str())
        .map(|s| s.to_string())
        .unwrap_or_else(|| path.to_string())
}

/// File stem (file name without extension), or the file name if none.
fn path_stem(path: &str) -> String {
    Path::new(path)
        .file_stem()
        .and_then(|stem| stem.to_str())
        .map(|s| s.to_string())
        .unwrap_or_else(|| path_file_name(path))
}

/// Parent directory path as a String, or None if the path has no parent.
fn path_parent(path: &str) -> Option<String> {
    Path::new(path).parent()?.to_str().map(|s| s.to_string())
}

// ---------------------------------------------------------------------------
// Matching algorithm (ported verbatim from soia)
// ---------------------------------------------------------------------------

fn is_subtitle_file(path: &str) -> bool {
    let ext = path_extension(path);
    SUBTITLE_FILE_EXTENSIONS.iter().any(|item| *item == ext)
}

fn is_matching_subtitle_folder(media_file_name: &str, folder_name: &str) -> bool {
    let media_stem = normalize_match_text(&path_stem(media_file_name));
    let folder_name = normalize_match_text(folder_name);
    !media_stem.is_empty() && media_stem == folder_name
}

fn nested_subtitle_match_name(media_file_name: &str, subtitle_file_name: &str) -> String {
    format!("{} {}", path_stem(media_file_name), subtitle_file_name)
}

fn normalize_match_text(value: &str) -> String {
    let mut result = String::new();
    let mut previous_space = false;
    for ch in value.to_lowercase().chars() {
        let next = if ch == '\'' || ch == '"' {
            None
        } else if ch.is_alphanumeric() {
            Some(ch)
        } else {
            Some(' ')
        };
        if let Some(ch) = next {
            if ch == ' ' {
                if !previous_space {
                    result.push(ch);
                    previous_space = true;
                }
            } else {
                result.push(ch);
                previous_space = false;
            }
        }
    }
    result.trim().to_string()
}

fn tokenize_match_text(value: &str) -> Vec<String> {
    let mut tokens: Vec<String> = Vec::new();
    for token in value.split_whitespace() {
        if IGNORED_SUBTITLE_MATCH_TOKENS.iter().any(|item| *item == token) {
            continue;
        }
        if !tokens.iter().any(|item| item == token) {
            tokens.push(token.to_string());
        }
    }
    tokens
}

fn extract_episode_keys(value: &str) -> Vec<String> {
    let compact = value
        .to_lowercase()
        .chars()
        .filter(|ch| ch.is_alphanumeric())
        .collect::<String>();
    let chars = compact.chars().collect::<Vec<_>>();
    let mut result: Vec<String> = Vec::new();
    for index in 0..chars.len() {
        if chars[index] != 's' {
            continue;
        }
        let mut cursor = index + 1;
        let season_start = cursor;
        while cursor < chars.len() && chars[cursor].is_ascii_digit() && cursor - season_start < 2 {
            cursor += 1;
        }
        if season_start == cursor || cursor >= chars.len() || chars[cursor] != 'e' {
            continue;
        }
        cursor += 1;
        let episode_start = cursor;
        while cursor < chars.len() && chars[cursor].is_ascii_digit() && cursor - episode_start < 3 {
            cursor += 1;
        }
        if episode_start == cursor {
            continue;
        }
        let key = chars[index..cursor].iter().collect::<String>();
        if !result.iter().any(|item| item == &key) {
            result.push(key);
        }
    }
    result
}

fn extract_year_keys(value: &str) -> Vec<String> {
    let mut result: Vec<String> = Vec::new();
    for token in normalize_match_text(value).split_whitespace() {
        if token.len() != 4 || !token.chars().all(|ch| ch.is_ascii_digit()) {
            continue;
        }
        if let Ok(year) = token.parse::<u16>() {
            if (1900..=2099).contains(&year) && !result.iter().any(|item| item == token) {
                result.push(token.to_string());
            }
        }
    }
    result
}

fn has_boundary_prefix(value: &str, prefix: &str) -> bool {
    value == prefix || value.starts_with(&format!("{prefix} "))
}

fn subtitle_match_score(media_name: &str, subtitle_name: &str) -> f64 {
    let media_stem = path_stem(media_name);
    let subtitle_stem = path_stem(subtitle_name);
    let normalized_media = normalize_match_text(&media_stem);
    let normalized_subtitle = normalize_match_text(&subtitle_stem);
    if normalized_media.is_empty() || normalized_subtitle.is_empty() {
        return 0.0;
    }
    if normalized_media == normalized_subtitle {
        return 100.0;
    }

    let media_episode_keys = extract_episode_keys(&media_stem);
    let subtitle_episode_keys = extract_episode_keys(&subtitle_stem);
    if !media_episode_keys.is_empty()
        && !media_episode_keys
            .iter()
            .any(|key| subtitle_episode_keys.iter().any(|item| item == key))
    {
        return 0.0;
    }

    let media_year_keys = extract_year_keys(&media_stem);
    let subtitle_year_keys = extract_year_keys(&subtitle_stem);
    if !media_year_keys.is_empty()
        && !subtitle_year_keys.is_empty()
        && !media_year_keys
            .iter()
            .any(|key| subtitle_year_keys.iter().any(|item| item == key))
    {
        return 0.0;
    }

    if has_boundary_prefix(&normalized_subtitle, &normalized_media) {
        return 95.0;
    }
    if has_boundary_prefix(&normalized_media, &normalized_subtitle) {
        return 85.0;
    }

    let media_tokens = tokenize_match_text(&normalized_media);
    let subtitle_tokens = tokenize_match_text(&normalized_subtitle);
    if media_tokens.is_empty() || subtitle_tokens.is_empty() {
        return 0.0;
    }
    let shared_count = media_tokens
        .iter()
        .filter(|token| subtitle_tokens.iter().any(|item| item == *token))
        .count();
    let token_score =
        (2.0 * shared_count as f64) / (media_tokens.len() + subtitle_tokens.len()) as f64;
    if shared_count >= 2 && token_score >= 0.55 {
        return token_score * 80.0;
    }
    if shared_count >= 1 && token_score >= 0.75 {
        return token_score * 75.0;
    }
    0.0
}

/// Score + sort candidate subtitle entries against the media names. Ported verbatim from
/// soia's `matching_subtitle_entries` (with `score` + `label` surfaced in the output for
/// fyom's subtitle picker UI).
fn matching_subtitle_entries(
    media_names: &[String],
    media_path: &str,
    entries: Vec<SubtitleMatchEntry>,
) -> Vec<(f64, String, SubtitleMatchEntry)> {
    let media_path = media_path.trim();
    let mut scored = entries
        .into_iter()
        .filter(|entry| entry.path.trim() != media_path && is_subtitle_file(&entry.path))
        .filter_map(|entry| {
            let score = media_names
                .iter()
                .map(|media_name| subtitle_match_score(media_name, &entry.name))
                .fold(0.0, f64::max);
            if score <= 0.0 {
                return None;
            }
            Some((score, entry.name.to_lowercase(), entry))
        })
        .collect::<Vec<_>>();
    scored.sort_by(|left, right| {
        right
            .0
            .partial_cmp(&left.0)
            .unwrap_or(std::cmp::Ordering::Equal)
            .then_with(|| left.1.cmp(&right.1))
    });
    scored
}

// ---------------------------------------------------------------------------
// Impl entry point (called by `commands::playback::find_external_subtitles`)
// ---------------------------------------------------------------------------

/// Find external subtitle files matching a LOCAL media file.
///
/// Scans the media file's parent directory + any sibling subdirs whose name matches the
/// media stem (the common "Subs"/"Subtitles" folder convention, e.g.
/// `Movie (2020)/Movie (2020).mkv` + `Movie (2020)/Subs/movie.en.srt` — but soia's
/// algorithm matches the folder name against the media stem, so `Movie (2020)/Movie
/// (2020)/movie.en.srt` also works).
///
/// Returns a sorted list (best match first) of `{ path, score, label }`. The frontend
/// feeds the paths to mpv's `sub-add` command (one by one — the first as `select`, the
/// rest as `auto`).
///
/// **LOCAL-only**: `media_path` must be an absolute filesystem path. For remote (S3 /
/// presigned URL) media, this function returns an empty list (fyom's architecture: the
/// server issues presigned URLs; the client streams directly; there's no filesystem to
/// scan). A future Phase 3 server-side endpoint may surface sibling subtitles for remote
/// media.
pub async fn find_external_subtitles_impl(
    payload: ExternalSubtitleMatchesPayload,
) -> Result<Vec<ExternalSubtitleMatch>, String> {
    let media_path_str = payload.media_path.trim();
    if media_path_str.is_empty() {
        return Ok(Vec::new());
    }
    let local_path = Path::new(media_path_str);
    if !local_path.is_file() {
        // Not a local file (presigned URL, network path, or missing) → no local subs.
        return Ok(Vec::new());
    }
    let Some(parent) = local_path.parent() else {
        return Ok(Vec::new());
    };

    let primary_media_name = payload
        .media_title
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
        .unwrap_or_else(|| path_file_name(media_path_str));
    let fallback_media_name = local_path
        .file_name()
        .and_then(|name| name.to_str())
        .map(ToString::to_string)
        .unwrap_or_else(|| path_file_name(media_path_str));
    let mut media_names = vec![primary_media_name];
    if !media_names.iter().any(|item| item == &fallback_media_name) {
        media_names.push(fallback_media_name.clone());
    }

    let parent_entries = std::fs::read_dir(parent)
        .map_err(|error| error.to_string())?
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .collect::<Vec<_>>();

    let mut entries = parent_entries
        .iter()
        .filter(|entry_path| entry_path.is_file())
        .map(|entry_path| SubtitleMatchEntry {
            name: entry_path
                .file_name()
                .and_then(|name| name.to_str())
                .unwrap_or_default()
                .to_string(),
            path: entry_path.to_string_lossy().into_owned(),
        })
        .collect::<Vec<_>>();

    // Sibling subdirs whose name matches the media stem (soia's "Subs folder" convention).
    let subtitle_dirs = parent_entries
        .iter()
        .filter(|entry_path| entry_path.is_dir())
        .filter(|entry_path| {
            let folder_name = entry_path
                .file_name()
                .and_then(|name| name.to_str())
                .unwrap_or_default();
            is_matching_subtitle_folder(&fallback_media_name, folder_name)
        })
        .cloned()
        .collect::<Vec<_>>();
    for subtitle_dir in subtitle_dirs {
        let Ok(child_entries) = std::fs::read_dir(&subtitle_dir) else {
            continue;
        };
        entries.extend(
            child_entries
                .filter_map(Result::ok)
                .map(|entry| entry.path())
                .filter(|entry_path| entry_path.is_file())
                .map(|entry_path| SubtitleMatchEntry {
                    name: nested_subtitle_match_name(
                        &fallback_media_name,
                        entry_path
                            .file_name()
                            .and_then(|name| name.to_str())
                            .unwrap_or_default(),
                    ),
                    path: entry_path.to_string_lossy().into_owned(),
                }),
        );
    }

    let matches = matching_subtitle_entries(&media_names, media_path_str, entries);
    Ok(matches
        .into_iter()
        .map(|(score, _sorted_key, entry)| ExternalSubtitleMatch {
            path: entry.path,
            score,
            label: path_stem(&entry.name),
        })
        .collect())
}
