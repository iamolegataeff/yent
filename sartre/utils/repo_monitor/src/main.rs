// repo_monitor — SARTRE perception utility (first one).
//
// Scans configured paths (research dirs, README, ...) and reports what changed
// against the PREVIOUS state — added / modified / removed, per file — so the model
// has something to think about. Files are keyed by SHA-256 of content, so a change
// is caught even when the size is unchanged (a reworded README line).
//
// A SARTRE utility is a process that speaks JSON lines on stdout; the language is
// irrelevant to the slot. This one is Rust, zero external dependencies (std only),
// and async: a scanner thread walks the tree and ships changes over an mpsc channel
// to the emitter, so scanning never blocks emission (and a future control channel on
// stdin would not block the scan).
//
// Modes:
//   (default) watch  — baseline scan is silent, then every --interval seconds emit
//                       changes vs the running state.
//   --once           — one scan; with --state FILE, diff against / save to that file
//                       (deterministic added/modified/removed for tests and the slot demo).

mod sha256;

use std::collections::BTreeMap;
use std::fs;
use std::io::{self, Write};
use std::path::{Path, PathBuf};
use std::sync::mpsc;
use std::thread;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

/// path -> content sha256
type State = BTreeMap<String, String>;

struct Config {
    paths: Vec<PathBuf>,
    exts: Vec<String>, // lowercase, dot-prefixed (".md"); empty = all files
    interval: u64,
    once: bool,
    state_file: Option<PathBuf>,
}

struct Change {
    kind: &'static str, // "added" | "modified" | "removed"
    path: String,
}

fn now_secs() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

fn is_flag(s: &std::ffi::OsString) -> bool {
    s.to_str().map(|t| t.starts_with("--")).unwrap_or(false)
}

fn parse_args() -> Config {
    let mut paths = Vec::new();
    let mut exts = Vec::new();
    let mut interval = 30u64;
    let mut once = false;
    let mut state_file = None;

    // args_os: never panics on a non-UTF-8 path argument (file paths can be non-UTF-8).
    let args: Vec<std::ffi::OsString> = std::env::args_os().skip(1).collect();
    // a value is present only if the next arg exists AND is not itself a flag
    // (so `--path --once` does not silently eat `--once`).
    let has_val = |i: usize| -> bool { i + 1 < args.len() && !is_flag(&args[i + 1]) };

    let mut i = 0;
    while i < args.len() {
        match args[i].to_str() {
            Some("--path") => {
                if has_val(i) {
                    paths.push(PathBuf::from(&args[i + 1]));
                    i += 1;
                } else {
                    eprintln!("[repo_monitor] --path needs a value");
                }
            }
            Some("--ext") => {
                if has_val(i) {
                    if let Some(v) = args[i + 1].to_str() {
                        for e in v.split(',') {
                            let e = e.trim().to_lowercase();
                            if e.is_empty() {
                                continue;
                            }
                            exts.push(if e.starts_with('.') { e } else { format!(".{}", e) });
                        }
                    }
                    i += 1;
                } else {
                    eprintln!("[repo_monitor] --ext needs a value");
                }
            }
            Some("--interval") => {
                if has_val(i) {
                    interval = args[i + 1].to_str().and_then(|v| v.parse().ok()).unwrap_or(30);
                    i += 1;
                } else {
                    eprintln!("[repo_monitor] --interval needs a value");
                }
            }
            Some("--state") => {
                if has_val(i) {
                    state_file = Some(PathBuf::from(&args[i + 1]));
                    i += 1;
                } else {
                    eprintln!("[repo_monitor] --state needs a value");
                }
            }
            Some("--once") => once = true,
            other => eprintln!("[repo_monitor] ignoring unknown arg: {}", other.unwrap_or("<non-utf8>")),
        }
        i += 1;
    }

    if exts.is_empty() {
        exts = [".md", ".txt", ".rs", ".c", ".h", ".go", ".json"]
            .iter()
            .map(|s| s.to_string())
            .collect();
    }
    if interval == 0 {
        interval = 1;
    }

    Config { paths, exts, interval, once, state_file }
}

fn has_ext(p: &Path, exts: &[String]) -> bool {
    if exts.is_empty() {
        return true;
    }
    match p.extension().and_then(|e| e.to_str()) {
        Some(e) => {
            let dot = format!(".{}", e.to_lowercase());
            exts.iter().any(|x| *x == dot)
        }
        None => false,
    }
}

fn scan_dir(dir: &Path, exts: &[String], out: &mut State) {
    let rd = match fs::read_dir(dir) {
        Ok(r) => r,
        Err(_) => return, // unreadable dir: skip, do not crash
    };
    for entry in rd.flatten() {
        if entry.file_name() == ".git" {
            continue;
        }
        let p = entry.path();
        let ft = match entry.file_type() {
            Ok(t) => t,
            Err(_) => continue,
        };
        if ft.is_dir() {
            scan_dir(&p, exts, out);
        } else if ft.is_file() && has_ext(&p, exts) {
            hash_file(&p, out);
        }
    }
}

fn hash_file(p: &Path, out: &mut State) {
    match fs::read(p) {
        Ok(bytes) => {
            out.insert(p.to_string_lossy().into_owned(), sha256::sha256_hex(&bytes));
        }
        Err(e) => eprintln!("[repo_monitor] hash failed {}: {}", p.display(), e),
    }
}

fn scan(cfg: &Config) -> State {
    let mut m = State::new();
    for base in &cfg.paths {
        if base.is_file() {
            if has_ext(base, &cfg.exts) {
                hash_file(base, &mut m);
            }
        } else {
            scan_dir(base, &cfg.exts, &mut m);
        }
    }
    m
}

fn diff(prev: &State, cur: &State) -> Vec<Change> {
    let mut out = Vec::new();
    for (p, s) in cur {
        match prev.get(p) {
            None => out.push(Change { kind: "added", path: p.clone() }),
            Some(ps) if ps != s => out.push(Change { kind: "modified", path: p.clone() }),
            _ => {}
        }
    }
    for p in prev.keys() {
        if !cur.contains_key(p) {
            out.push(Change { kind: "removed", path: p.clone() });
        }
    }
    out
}

fn json_escape(s: &str) -> String {
    let mut out = String::with_capacity(s.len() + 2);
    for ch in s.chars() {
        match ch {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out
}

// Returns Err if stdout is gone (the slot reader closed the pipe) — the caller
// then exits cleanly instead of panicking, as `println!` would.
fn emit<W: Write>(out: &mut W, ch: &Change) -> io::Result<()> {
    writeln!(
        out,
        "{{\"util\":\"repo_monitor\",\"kind\":\"{}\",\"path\":\"{}\",\"ts\":{}}}",
        ch.kind,
        json_escape(&ch.path),
        now_secs()
    )
}

fn load_state(p: &Path) -> State {
    let mut m = State::new();
    if let Ok(txt) = fs::read_to_string(p) {
        for line in txt.lines() {
            if let Some((sha, path)) = line.split_once('\t') {
                m.insert(path.to_string(), sha.to_string());
            }
        }
    }
    m
}

fn save_state(p: &Path, m: &State) {
    let mut s = String::new();
    for (path, sha) in m {
        s.push_str(sha);
        s.push('\t');
        s.push_str(path);
        s.push('\n');
    }
    let _ = fs::write(p, s);
}

fn main() {
    let cfg = parse_args();

    if cfg.once {
        let prev = cfg.state_file.as_ref().map(|f| load_state(f)).unwrap_or_default();
        let cur = scan(&cfg);
        {
            let mut out = io::stdout().lock();
            for ch in diff(&prev, &cur) {
                if emit(&mut out, &ch).is_err() {
                    break;
                }
            }
            let _ = out.flush();
        }
        if let Some(f) = &cfg.state_file {
            save_state(f, &cur);
        }
        return;
    }

    // watch mode — async: scanner thread -> mpsc -> emitter (main).
    let (tx, rx) = mpsc::channel::<Change>();
    let paths = cfg.paths.clone();
    let exts = cfg.exts.clone();
    let interval = cfg.interval;
    thread::spawn(move || {
        let scfg = Config { paths, exts, interval, once: false, state_file: None };
        let mut state = scan(&scfg); // baseline is silent
        loop {
            thread::sleep(Duration::from_secs(interval));
            let cur = scan(&scfg);
            for ch in diff(&state, &cur) {
                if tx.send(ch).is_err() {
                    return; // emitter gone
                }
            }
            state = cur;
        }
    });

    let mut out = io::stdout().lock();
    for ch in rx {
        if emit(&mut out, &ch).is_err() {
            break; // reader (the slot supervisor) is gone — exit cleanly, do not panic
        }
        let _ = out.flush();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn st(pairs: &[(&str, &str)]) -> State {
        pairs.iter().map(|(p, s)| (p.to_string(), s.to_string())).collect()
    }

    #[test]
    fn diff_added_modified_removed() {
        let prev = st(&[("a", "1"), ("b", "2")]);
        let cur = st(&[("a", "1"), ("b", "9"), ("c", "3")]);
        let changes = diff(&prev, &cur);
        // cur iterated first (sorted): b modified, c added; then removed pass: (none here)
        let got: Vec<(&str, &str)> = changes.iter().map(|c| (c.kind, c.path.as_str())).collect();
        assert!(got.contains(&("modified", "b")));
        assert!(got.contains(&("added", "c")));
        assert!(!got.iter().any(|(k, _)| *k == "removed"));
        assert!(!got.iter().any(|(_, p)| *p == "a")); // unchanged not reported
    }

    #[test]
    fn diff_removed() {
        let prev = st(&[("a", "1"), ("gone", "x")]);
        let cur = st(&[("a", "1")]);
        let changes = diff(&prev, &cur);
        assert_eq!(changes.len(), 1);
        assert_eq!(changes[0].kind, "removed");
        assert_eq!(changes[0].path, "gone");
    }

    #[test]
    fn json_escape_quotes_and_backslashes() {
        assert_eq!(json_escape(r#"a/b"c\d"#), r#"a/b\"c\\d"#);
    }
}
