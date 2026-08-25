# fastype

[简体中文](README.md) | **English**

[![build](https://github.com/xiongyihui/fastype/actions/workflows/build.yml/badge.svg)](https://github.com/xiongyihui/fastype/actions/workflows/build.yml)

**fastype** is a Windows / macOS keyboard enhancement tool: it teaches an ordinary keyboard
「tap-hold」 and 「layers」. No new keyboard, no firmware flashing, no change to your
typing habits — the arrow keys and the shortcuts you use most are moved to your
fingertips, so coding gets faster and easier.

## Design Philosophy

Count how often your right hand leaves the home row while coding: reaching for the
arrow keys, Home/End, Page Up/Down, or stretching your pinky to the corner Ctrl…

fastype's design comes from a discovery made while studying 60% keyboard layouts:
with just three mechanisms — **tap-hold**, **combos**, and **layers** — an ordinary
keyboard can cover almost every efficiency need, keeping your hands around
ASDF / HJKL, much like VIM.

- **Tap-Hold**: one key that types as itself on tap, and becomes something else when
  held (switch a layer or act as a modifier)
- **Combo**: map any physical key to another key or a combo, e.g. `p` → `Shift+Insert` (paste)
- **Layer**: while a key is held, the whole keyboard temporarily switches to another
  mapping; release it and everything is back

## Works Out of the Box

On first run, a default configuration matching the philosophy above is generated and
takes effect immediately:

| You press | You get |
|---|---|
| Quick tap <kbd>d</kbd> / <kbd>;</kbd> / <kbd>'</kbd> / <kbd>CapsLock</kbd> | Everything unchanged — the original key |
| **Hold <kbd>d</kbd>** then <kbd>H</kbd> <kbd>J</kbd> <kbd>K</kbd> <kbd>L</kbd> | Arrow keys ← ↓ ↑ → |
| Hold <kbd>d</kbd> then <kbd>U</kbd> / <kbd>N</kbd> | Page Up / Page Down |
| Hold <kbd>d</kbd> then <kbd>Y</kbd> / <kbd>M</kbd> | Home / End |
| Hold <kbd>d</kbd> then <kbd>P</kbd> | Paste (Shift+Insert) |
| Hold <kbd>d</kbd> then <kbd>;</kbd> / <kbd>'</kbd> | Backspace / Esc |
| **Hold <kbd>;</kbd>** then <kbd>C</kbd> / <kbd>V</kbd> / <kbd>X</kbd> / <kbd>A</kbd> | Copy / Paste / Cut / Select All (equivalent Ctrl combos) |
| Hold <kbd>'</kbd> | Alt held |
| Hold <kbd>CapsLock</kbd> | Ctrl held (a quick tap is still the original CapsLock) |

+ **Holding <kbd>d</kbd> enters a temporary navigation layer** — move the cursor, page
  around, jump to line start/end without leaving the home row;
+ **Holding <kbd>;</kbd> is Ctrl** — copy and paste without diving your pinky down.

Quick taps pass through unchanged, so your existing typing habits are never affected.

The tap/hold threshold defaults to 500 ms and is adjustable in the config UI.

> **macOS default differences**: <kbd>p</kbd> maps to **⌘V** (paste) and holding <kbd>'</kbd>
> is ⌥ Option; everything else matches Windows (hold <kbd>;</kbd> = Ctrl, hold <kbd>d</kbd>
> for the navigation layer). macOS CapsLock has no key-up event, so tap-hold is not
> possible on it and it is left unmapped by default. In configs `command`/`option` are
> aliases of `windows`/`alt`, so config files are fully portable between the two platforms.

## Quick Start

Download the official build from [Releases](https://github.com/xiongyihui/fastype/releases),
or grab the `fastype-windows-amd64` / `fastype-macos` artifacts from the latest
successful build on the
[Actions page](https://github.com/xiongyihui/fastype/actions/workflows/build.yml).

1. **Run** `fastype.exe` (a `config.json` is generated on first run)
2. A **tray icon** appears in the bottom-right corner: double-click to open the config
   page; right-click for Open Config / Pause Mapping / Start with Windows / Exit
3. Visit `http://127.0.0.1:8765/` in a browser and edit your keyboard visually

Auto start: toggle「**Start with Windows**」 in the tray right-click menu; or manually
put a shortcut to `fastype.exe` in the `shell:startup` folder (Win+R → `shell:startup`).

### macOS

**Install (DMG recommended)**: download `Fastype-<version>-macos.dmg`, open it,
drag **Fastype** into Applications and double-click to launch (first run of an
unsigned app: right-click → Open). After launch:

1. A **⌨ icon appears in the menu bar at the top of the screen** (note: macOS has
   no Windows-style bottom-right tray; status icons live on the right end of the
   top menu bar — hover shows "Fastype - Waiting for permission").
2. **Grant permission**: on first launch fastype posts a notification and opens
   the Accessibility settings pane — add **Fastype** and enable the switch. It
   **activates automatically within seconds; no restart needed**. (You can also
   try `--dry-run` first with no permission: read-only, logs decisions only.)
3. Click the menu bar icon: Open Config / Pause / Start at Login / Exit; visit
   `http://127.0.0.1:8765/` to edit your keyboard visually (⌘/⌥ labels appear
   automatically; the status badge shows "Waiting for permission" until granted).

Start at Login is toggled from the menu bar (writes
`~/Library/LaunchAgents/com.xiongyihui.fastype.plist`, effective from the next
login); the Accessibility permission applies to **Fastype.app**.

**Running the CLI binary** (e.g. from a Homebrew bin dir): `fastype` /
`fastype --dry-run` — grant Accessibility to the **terminal app** running it.

## Visual Config UI

![fastype web config UI](docs/webui.png)

The config page is a web UI with a visual keyboard:
switch **Chinese/English** at the top-right; the tray menu follows the Windows display language

- Click any **keycap** to edit its mapping on the current layer in the side panel
- No typing key names — click an input and **press the key** on your real keyboard;
  modifiers (Ctrl/Alt/Shift/Win) are checkboxes
- Switch **layers** at the top; add or delete layers (up to 8)
- **Save & Apply**: changes take effect immediately, no restart needed
- If a mapping gets in the way while editing, hit **Pause** at the top-right and
  resume when done
- A built-in **test area** lets you try mappings right away
- A built-in **key monitor** shows currently held real keys and simulated
  (injected) keys in real time, plus a scrolling event log (recorded
  asynchronously, never blocking key processing)

## Three Mapping Types

Each key can be bound to one of three action types:

1. **Map to key / combo**: outputs a key or combo on press (e.g. `caps lock` → `esc`)
2. **Tap / Hold (Tap-Hold)**: tap outputs the original or any key; hold switches to a
   layer or acts as a modifier (turning the key into your Ctrl/Alt/Shift/Win)
3. **Momentary layer**: switches to a layer immediately on press, optionally with
   modifiers (equivalent to QMK's MO/LT semantics)

Keys without a mapping on the active layer pass through unchanged, so each layer only
needs the keys you actually change.

## CLI & Environment Variables

```
fastype.exe            # start normally
fastype.exe --dry-run  # rehearsal mode: log decisions only, intercept nothing
fastype.exe --config D:\my.json
fastype.exe --version / --help
```

| Variable | Effect |
|---|---|
| `FASTYPE_DEBUG=1` | Log every tap/hold decision (debugging aid) |
| `FASTYPE_DRY_RUN=1` | Same as `--dry-run` |
| `FASTYPE_CONFIG` | Path to the config file |
| `FASTYPE_NO_PROMPT=1` | macOS: skip the system permission dialog (for launchd/SSH) |

Config lookup order: `--config` argument > `FASTYPE_CONFIG` > `config.json` in the
current directory > next to the exe > the platform config dir
(Windows: `%APPDATA%\fastype`, macOS: `~/Library/Application Support/fastype`).
Day-to-day editing happens in the web UI; manual editing is rarely needed.

## FAQ

**My antivirus flagged or even deleted fastype.exe?**
This is a common false positive. How fastype works: it installs a Windows low-level
keyboard hook (`WH_KEYBOARD_LL`) to watch keystrokes globally, then synthesizes the
remapped keys with `SendInput` — "global keyboard hook + key injection" is exactly the
signature behavior of keylogger-style malware, and since the binary is unsigned and
freshly compiled, heuristic and cloud-reputation engines tend to flag it. fastype is
fully open source and auditable: it phones nothing home, the web UI listens on the
loopback interface (127.0.0.1) only, and it touches no files besides `config.json`.
If it happens, add an exclusion in your antivirus (Windows Defender: Virus & threat
protection settings → Exclusions), or build from source yourself.

**Why doesn't it work in elevated windows (Task Manager, etc.)? (Windows)**
A standard-privilege low-level keyboard hook doesn't receive events destined for
elevated programs. Run fastype as administrator when needed (for auto start, a
scheduled task with highest privileges is the tidy option).

**macOS asks for the Accessibility permission?**
fastype watches keys via CGEventTap and rewrites them via CGEventPost; macOS requires
the Accessibility permission first (the same requirement as every key-remapping tool
such as Karabiner). Grant it to **Fastype.app** (DMG install) or the terminal app
(CLI runs). Without the permission fastype keeps running: the menu bar icon shows
"Waiting for permission", re-checks every 2 seconds, and activates automatically
once you enable the switch — no restart needed. `--dry-run` needs no permission.

**Lost the Accessibility permission after installing a new build? (macOS)**
The GitHub DMGs are ad-hoc signed (no Apple Developer certificate). Since the
2026-08 builds the signing requirement is pinned to the bundle identifier, so
**upgrades normally keep the Accessibility permission**. If the UI still shows
"Waiting for permission" after an upgrade: remove Fastype (select it, press
"−") under System Settings → Privacy & Security → Accessibility, re-add it
with "+" and enable the switch — it activates within seconds. fastype notices
and opens that settings pane for you automatically.

**Does it work in games?**
Some games and anti-cheat systems ignore synthesized keystrokes (on both Windows and
macOS). Don't rely on fastype in games, and follow the game's rules.

**Will keys get stuck after quitting?**
No. Tray exit or Ctrl+C releases every synthesized key that is still held.

**Can I run two instances?**
No. If an existing instance is detected at startup, fastype exits with a notice to
avoid two hooks interfering with each other.

**How do I pause it temporarily?**
「Pause Mapping」 in the tray / menu bar, or the pause button in the web UI; quit from
the tray / menu bar to stop entirely.

**Which systems are supported?**
Windows x64 and macOS (Intel / Apple Silicon, macOS 13+).

## Build from Source

Requires Go ≥ 1.22 (plus Xcode Command Line Tools for clang on macOS), zero
third-party dependencies:

```
go test ./...

# Windows
go build -ldflags "-s -w -H windowsgui" -o dist\fastype.exe .\cmd\fastype
go build -ldflags "-X main.debugDefault=1" -o dist\fastype-debug.exe .\cmd\fastype

# macOS (native)
go build -ldflags "-s -w" -o dist/fastype ./cmd/fastype
go build -ldflags "-X main.debugDefault=1" -o dist/fastype-debug ./cmd/fastype

# macOS cross builds
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC="clang -arch x86_64" go build -o dist/fastype-macos-amd64 ./cmd/fastype
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CC="clang -arch arm64" go build -o dist/fastype-macos-arm64 ./cmd/fastype

# macOS package .app + DMG (universal, icon + ad-hoc signature)
scripts/package-macos.sh
```

On Windows `fastype.exe` is for daily use (windowless background); `fastype-debug.exe`
attaches a console and prints tap/hold decisions from startup. Same on macOS
(`fastype-debug`).

## Origin & Credits

- Design inspired by [keyboard](https://github.com/xiongyihui/keyboard)
- The Layers design of [TMK](https://github.com/tmk/tmk_keyboard)
- Jason Rudolph's [Toward a more useful keyboard](https://github.com/jasonrudolph/keyboard)
- The [QMK](https://qmk.fm) / [VIA](https://www.caniusevia.com) communities — layer
  semantics and the config UI inspiration
