# fastype

[简体中文](README.md) | **English**

[![build](https://github.com/xiongyihui/fastype/actions/workflows/build.yml/badge.svg)](https://github.com/xiongyihui/fastype/actions/workflows/build.yml)

**fastype** is a Windows keyboard enhancement tool: it teaches an ordinary keyboard
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

## Quick Start

Download the official build from [Releases](https://github.com/xiongyihui/fastype/releases),
or grab the `fastype-windows-amd64` artifact from the latest successful build on the
[Actions page](https://github.com/xiongyihui/fastype/actions/workflows/build.yml).

1. **Run** `fastype.exe` (a `config.json` is generated on first run)
2. A **tray icon** appears in the bottom-right corner: double-click to open the config
   page; right-click for Open Config / Pause Mapping / Start with Windows / Exit
3. Visit `http://127.0.0.1:8765/` in a browser and edit your keyboard visually

Auto start: toggle「**Start with Windows**」 in the tray right-click menu; or manually
put a shortcut to `fastype.exe` in the `shell:startup` folder (Win+R → `shell:startup`).

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

Config lookup order: `--config` argument > `FASTYPE_CONFIG` > `config.json` in the
current directory > next to the exe > `%APPDATA%\fastype\config.json`. Day-to-day
editing happens in the web UI; manual editing is rarely needed.

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

**Why doesn't it work in elevated windows (Task Manager, etc.)?**
A standard-privilege low-level keyboard hook doesn't receive events destined for
elevated programs. Run fastype as administrator when needed (for auto start, a
scheduled task with highest privileges is the tidy option).

**Does it work in games?**
Some games and anti-cheat systems ignore synthesized keystrokes. Don't rely on
fastype in games, and follow the game's rules.

**Will keys get stuck after quitting?**
No. Tray exit or Ctrl+C releases every synthesized key that is still held.

**Can I run two instances?**
No. If an existing instance is detected at startup, fastype exits with a notice to
avoid two hooks interfering with each other.

**How do I pause it temporarily?**
「Pause Mapping」 in the tray menu or the pause button in the web UI; quit from the
tray to stop entirely.

**Which systems are supported?**
Windows x64 only.

## Build from Source

Requires Go ≥ 1.22, zero third-party dependencies:

```
go test ./...
go build -ldflags "-s -w -H windowsgui" -o dist\fastype.exe .\cmd\fastype
go build -ldflags "-X main.debugDefault=1" -o dist\fastype-debug.exe .\cmd\fastype
```

`fastype.exe` is for daily use (windowless background); `fastype-debug.exe` attaches a
console and prints tap/hold decisions from startup — handy for troubleshooting.

## Origin & Credits

- Design inspired by [keyboard](https://github.com/xiongyihui/keyboard)
- The Layers design of [TMK](https://github.com/tmk/tmk_keyboard)
- Jason Rudolph's [Toward a more useful keyboard](https://github.com/jasonrudolph/keyboard)
- The [QMK](https://qmk.fm) / [VIA](https://www.caniusevia.com) communities — layer
  semantics and the config UI inspiration
