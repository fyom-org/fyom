//! External subtitle discovery for local filesystem media.
//!
//! This module finds subtitle files near a local media file and scores them using
//! filename/title similarity. Remote media URLs intentionally return no matches.

use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

// -----------------------------------------------------------------------------
// Payload types
// -----------------------------------------------------------------------------

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ExternalSubtitleMatchesPayload {
    pub media_path: String,
    pub media_title: Option<String>,
}

pub type ExternalSubtitleMatchesPayloadResolved = ExternalSubtitleMatchesPayload;

#[derive(Serialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct ExternalSubtitleMatch {
    pub path: String,
    pub score: f64,
    pub label: String,
}

#[derive(Clone)]
struct SubtitleMatchEntry {
    name: String,
    path: String,
}

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const SUBTITLE_FILE_EXTENSIONS: &[&str] = &[
    "srt", "ass", "ssa", "vtt", "sub", "idx", "sup", "smi", "smil", "lrc", "ttml", "dfxp",
];

const IGNORED_SUBTITLE_MATCH_TOKENS: &[&str] = &[
    "1080p", "2160p", "720p", "480p", "4k", "8k", "web", "webrip", "webdl", "web-dl", "dl",
    "bluray", "bdrip", "hdrip", "dvdrip", "hdtv", "x264", "x265", "h264", "h265", "hevc", "avc",
    "aac", "dts", "ddp", "atmos", "proper", "repack", "remux", "extended", "internal", "multi",
    "chs", "cht", "chi", "zho", "zh", "cn", "gb", "big5", "eng", "en", "jpn", "jp", "kor", "kr",
    "sc", "tc",
];

const COMMON_SUBTITLE_DIR_NAMES: &[&str] = &[
    "subs",
    "sub",
    "subtitle",
    "subtitles",
    "captions",
    "caption",
];

// -----------------------------------------------------------------------------
// Path helpers
// -----------------------------------------------------------------------------

fn path_extension(path: &str) -> String {
    Path::new(path)
        .extension()
        .and_then(|ext| ext.to_str())
        .map(str::to_lowercase)
        .unwrap_or_default()
}

fn path_file_name(path: &str) -> String {
    Path::new(path)
        .file_name()
        .and_then(|name| name.to_str())
        .map(ToString::to_string)
        .unwrap_or_else(|| path.to_string())
}

fn path_stem(path: &str) -> String {
    Path::new(path)
        .file_stem()
        .and_then(|stem| stem.to_str())
        .map(ToString::to_string)
        .unwrap_or_else(|| path_file_name(path))
}

fn file_name_from_path(path: &Path) -> String {
    path.file_name()
        .and_then(|name| name.to_str())
        .unwrap_or_default()
        .to_string()
}

fn path_to_string(path: &Path) -> String {
    path.to_string_lossy().into_owned()
}

// -----------------------------------------------------------------------------
// Matching algorithm
// -----------------------------------------------------------------------------

fn is_subtitle_file(path: &str) -> bool {
    let ext = path_extension(path);
    SUBTITLE_FILE_EXTENSIONS.iter().any(|item| *item == ext)
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

        let Some(ch) = next else {
            continue;
        };

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

    result.trim().to_string()
}

fn tokenize_match_text(value: &str) -> Vec<String> {
    let mut tokens = Vec::new();

    for token in value.split_whitespace() {
        if IGNORED_SUBTITLE_MATCH_TOKENS
            .iter()
            .any(|ignored| *ignored == token)
        {
            continue;
        }

        if !tokens.iter().any(|existing| existing == token) {
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
    let mut result = Vec::new();

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

        while cursor < chars.len() && chars[cursor].is_ascii_digit() && cursor - episode_start < 3
        {
            cursor += 1;
        }

        if episode_start == cursor {
            continue;
        }

        let key = chars[index..cursor].iter().collect::<String>();

        if !result.iter().any(|existing| existing == &key) {
            result.push(key);
        }
    }

    result
}

fn extract_year_keys(value: &str) -> Vec<String> {
    let mut result = Vec::new();

    for token in normalize_match_text(value).split_whitespace() {
        if token.len() != 4 || !token.chars().all(|ch| ch.is_ascii_digit()) {
            continue;
        }

        let Ok(year) = token.parse::<u16>() else {
            continue;
        };

        if (1900..=2099).contains(&year) && !result.iter().any(|existing| existing == token) {
            result.push(token.to_string());
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

// -----------------------------------------------------------------------------
// Directory scanning
// -----------------------------------------------------------------------------

fn is_common_subtitle_dir(folder_name: &str) -> bool {
    let normalized = normalize_match_text(folder_name);

    COMMON_SUBTITLE_DIR_NAMES
        .iter()
        .any(|name| normalized == *name)
}

fn is_matching_media_named_dir(media_file_name: &str, folder_name: &str) -> bool {
    let media_stem = normalize_match_text(&path_stem(media_file_name));
    let folder_name = normalize_match_text(folder_name);

    !media_stem.is_empty() && media_stem == folder_name
}

fn should_scan_subtitle_dir(media_file_name: &str, folder_name: &str) -> bool {
    is_common_subtitle_dir(folder_name) || is_matching_media_named_dir(media_file_name, folder_name)
}

fn nested_subtitle_match_name(media_file_name: &str, subtitle_file_name: &str) -> String {
    format!("{} {}", path_stem(media_file_name), subtitle_file_name)
}

fn read_dir_paths(path: &Path) -> Result<Vec<PathBuf>, String> {
    let entries = std::fs::read_dir(path).map_err(|error| error.to_string())?;

    Ok(entries
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .collect())
}

fn direct_subtitle_entries(paths: &[PathBuf]) -> Vec<SubtitleMatchEntry> {
    paths.iter()
        .filter(|path| path.is_file())
        .map(|path| SubtitleMatchEntry {
            name: file_name_from_path(path),
            path: path_to_string(path),
        })
        .collect()
}

fn nested_subtitle_entries(
    media_file_name: &str,
    parent_entries: &[PathBuf],
) -> Vec<SubtitleMatchEntry> {
    let mut entries = Vec::new();

    let subtitle_dirs = parent_entries
        .iter()
        .filter(|path| path.is_dir())
        .filter(|path| {
            let folder_name = path.file_name().and_then(|name| name.to_str()).unwrap_or("");
            should_scan_subtitle_dir(media_file_name, folder_name)
        })
        .collect::<Vec<_>>();

    for subtitle_dir in subtitle_dirs {
        let Ok(child_entries) = read_dir_paths(subtitle_dir) else {
            continue;
        };

        entries.extend(
            child_entries
                .iter()
                .filter(|path| path.is_file())
                .map(|path| {
                    let subtitle_file_name = file_name_from_path(path);

                    SubtitleMatchEntry {
                        name: nested_subtitle_match_name(media_file_name, &subtitle_file_name),
                        path: path_to_string(path),
                    }
                }),
        );
    }

    entries
}

// -----------------------------------------------------------------------------
// Public entry point
// -----------------------------------------------------------------------------

pub async fn find_external_subtitles_impl(
    payload: ExternalSubtitleMatchesPayload,
) -> Result<Vec<ExternalSubtitleMatch>, String> {
    let media_path_str = payload.media_path.trim();

    if media_path_str.is_empty() {
        return Ok(Vec::new());
    }

    let media_path = Path::new(media_path_str);

    if !media_path.is_file() {
        return Ok(Vec::new());
    }

    let Some(parent) = media_path.parent() else {
        return Ok(Vec::new());
    };

    let primary_media_name = payload
        .media_title
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
        .unwrap_or_else(|| path_file_name(media_path_str));

    let fallback_media_name = media_path
        .file_name()
        .and_then(|name| name.to_str())
        .map(ToString::to_string)
        .unwrap_or_else(|| path_file_name(media_path_str));

    let mut media_names = vec![primary_media_name];

    if !media_names
        .iter()
        .any(|name| name == &fallback_media_name)
    {
        media_names.push(fallback_media_name.clone());
    }

    let parent_entries = read_dir_paths(parent)?;

    let mut entries = direct_subtitle_entries(&parent_entries);
    entries.extend(nested_subtitle_entries(
        &fallback_media_name,
        &parent_entries,
    ));

    let matches = matching_subtitle_entries(&media_names, media_path_str, entries);

    Ok(matches
        .into_iter()
        .map(|(score, _sort_key, entry)| ExternalSubtitleMatch {
            path: entry.path,
            score,
            label: path_stem(&entry.name),
        })
        .collect())
}
