# fastype

**简体中文** | [English](README.en.md)

[![build](https://github.com/xiongyihui/fastype/actions/workflows/build.yml/badge.svg)](https://github.com/xiongyihui/fastype/actions/workflows/build.yml)

**fastype** 是一个 Windows / macOS 键盘增强工具：让普通键盘学会「长按」与「分层」。
不用换键盘、不用刷固件、不用改打字习惯，把方向键、快捷键这些最常用的功能搬到手边，让写代码更快、更省力。

## 设计理念

数一数你写代码时右手离开主键区的次数：够方向键、够 Home/End、够翻页键，小拇指一次次压向角落里的 Ctrl……

fastype 的设计，源自研究 60% 键盘按键方案时的发现：普通键盘只需要 **长按**、**组合键**、**层** 三种机制，就能覆盖绝大多数效率需求——
让手始终停留在 ASDF / HJKL 附近，类似VIM。

- **长按（Tap-Hold）**：一个键，点按是它自己，长按变成别的功能（切层或按住修饰键）
- **组合键**：把任意物理键映射成另一个键或组合键，如 `p` → `Shift+Insert`（粘贴）
- **层（Layer）**：按住某个键期间，整块键盘临时切换到另一套映射，松开即回

## 开箱即用

第一次运行自动生成一套与上述理念一致的默认配置，立即生效：

| 你按下的键 | 输出 |
|---|---|
| 快速点按 <kbd>d</kbd> / <kbd>;</kbd> / <kbd>'</kbd> / <kbd>CapsLock</kbd> | 一切照旧，输出原键 |
| **按住 <kbd>d</kbd>** 再按 <kbd>H</kbd> <kbd>J</kbd> <kbd>K</kbd> <kbd>L</kbd> | 方向键 ← ↓ ↑ → |
| 按住 <kbd>d</kbd> 再按 <kbd>U</kbd> / <kbd>N</kbd> | 上一页 / 下一页 |
| 按住 <kbd>d</kbd> 再按 <kbd>Y</kbd> / <kbd>M</kbd> | 行首 / 行尾 |
| 按住 <kbd>d</kbd> 再按 <kbd>P</kbd> | 粘贴（Shift+Insert） |
| 按住 <kbd>d</kbd> 再按 <kbd>;</kbd> / <kbd>'</kbd> | 退格 / Esc |
| **长按 <kbd>;</kbd>** 再按 <kbd>C</kbd> / <kbd>V</kbd> / <kbd>X</kbd> / <kbd>A</kbd> | 复制 / 粘贴 / 剪切 / 全选（等价 Ctrl 组合键） |
| 长按 <kbd>'</kbd> | 按住 Alt |
| 长按 <kbd>CapsLock</kbd> | 按住 Ctrl（点按仍是原 CapsLock） |

+ **按住 d 就进入一层临时导航层**，编辑代码时手不离主键区就能移动光标、翻页、跳行首行尾；
+ **长按分号就是 Ctrl**，复制粘贴不再需要小拇指下探。

而快速点按时所有键保持原功能——你原有的打字习惯完全不受影响。

点按与长按的判定阈值默认 500ms，可在配置界面调整。

> **macOS 默认键位差异**：<kbd>p</kbd> 映射为 **⌘V**（粘贴）、长按 <kbd>'</kbd> 是 ⌥ Option；
> 其余与 Windows 相同（长按 <kbd>;</kbd> = Ctrl、按住 <kbd>d</kbd> 进导航层）。
> macOS 的 CapsLock 没有抬起事件、不支持长按判定，默认不做映射。
> 配置中 `command`/`option` 与 `windows`/`alt` 互为别名，两平台配置文件完全兼容。

## 上手三步

正式版到 [Releases](https://github.com/xiongyihui/fastype/releases) 下载；或打开
[Actions 构建页](https://github.com/xiongyihui/fastype/actions/workflows/build.yml)）下载最新一次成功构建 `fastype-windows-amd64` 产物。

1. **运行** `fastype.exe`（首次运行自动生成 `config.json`）
2. 屏幕右下角出现**托盘图标**：双击打开配置页面，右键可 打开配置 / 暂停映射 / 开机自启 / 退出
3. 浏览器访问 `http://127.0.0.1:8765/` ，可视化编辑你的键盘

开机自启：托盘右键菜单点「**开机自启**」即可一键开启/关闭；也可以手动把 `fastype.exe` 的快捷方式放进 `shell:startup` 文件夹
（Win+R 输入 `shell:startup` 回车即达）。

### macOS

**安装（推荐 DMG）**：下载 `Fastype-<版本>-macos.dmg`，打开后把 **Fastype** 拖入 Applications，
双击启动（首次运行未签名应用：右键 → 打开）。启动后：

1. **屏幕顶部菜单栏出现 ⌨ 图标**（注意：macOS 没有 Windows 那样的右下角托盘，
   状态图标在屏幕最上沿的菜单栏右侧，悬停显示「Fastype - 等待授权」）。
2. **授权**：首次启动 fastype 会弹出系统通知并打开「辅助功能」设置页——
   把 **Fastype** 加入并打开开关即可，**几秒内自动生效，无需重启**。
   （也可先用 `--dry-run` 免授权体验：只读监听并打印判定结果，不拦截不注入。）
3. 点击菜单栏图标：打开配置 / 暂停映射 / 登录自启 / 退出；
   浏览器访问 `http://127.0.0.1:8765/` 可视化编辑键盘（界面自动显示 ⌘/⌥ 标签，
   状态徽标在未授权时显示「等待授权」）。

登录自启由菜单栏菜单一键开关（写入 `~/Library/LaunchAgents/com.xiongyihui.fastype.plist`，
下次登录生效），授权对象为 **Fastype.app**。

**命令行方式运行**（Homebrew bin 目录等）：`fastype` / `fastype --dry-run`，
此时辅助功能授权对象是运行它的**终端 App**。

## 可视化配置界面

![fastype Web 配置界面](docs/webui.png)

配置页面采用Web UI可视化键盘：
界面右上角可**切换中英文**；托盘菜单跟随 Windows 显示语言

- 点击任意**键帽**，在侧边面板编辑它在当前层的映射
- 绑定按键不用打名字——点击输入框后**直接按键盘**捕获，修饰键（Ctrl/Alt/Shift/Win）
  用复选框勾选
- 顶部切换**层**，可增删层（最多 8 层）
- **保存并生效**：保存即热更新，无需重启程序
- 配置期间如果映射干扰输入，点右上角**暂停映射**，改完再恢复
- 页面内置**测试区**，改完立即试打感受效果

## 三种映射能力

配置界面里每个键可以绑定三种功能之一：

1. **映射为按键 / 组合键**：按下即输出指定键或组合键（如 `caps lock` → `esc`）
2. **点按 / 长按（Tap-Hold）**：点按输出原键或任意键；长按切换到某一层，或按住
   修饰键（于是这个键变成了你的 Ctrl/Alt/Shift/Win）
3. **切层**：按下立即切换到某一层，可附带修饰键（相当于 QMK 的 MO/LT 语义）

层内没有映射的键直接透传原始按键，所以每一层只需要配置「改动的键」。

## 命令行与环境变量

```
fastype.exe            # 正常启动
fastype.exe --dry-run  # 演练模式：只打印判定结果，不拦截任何按键（试用无风险）
fastype.exe --config D:\my.json
fastype.exe --version / --help
```

| 环境变量 | 作用 |
|---|---|
| `FASTYPE_DEBUG=1` | 打印每次按键的判定过程（调试点按/长按行为） |
| `FASTYPE_DRY_RUN=1` | 等同 `--dry-run` |
| `FASTYPE_CONFIG` | 指定配置文件路径 |
| `FASTYPE_NO_PROMPT=1` | macOS：缺少辅助功能权限时不弹系统授权界面（launchd/SSH 场景） |

配置文件查找顺序：`--config` 参数 > `FASTYPE_CONFIG` > 当前目录 `config.json` >
exe 同目录 > 系统配置目录（Windows: `%APPDATA%\fastype`，macOS: `~/Library/Application Support/fastype`）。
日常用配置界面编辑即可，一般不需要手改。

## 常见问题

**杀毒软件把 fastype.exe 报毒甚至删除了？**
这是常见的误报。fastype 的工作原理：通过 Windows 低级键盘钩子（`WH_KEYBOARD_LL`）监听全局按键，
再用 `SendInput` 合成改写后的按键——「全局键盘钩子 + 按键注入」正是键盘记录器（keylogger）类木马
的典型行为特征；加上程序没有数字签名、属于新编译的未知文件，杀软的启发式与云信誉引擎容易误判。
fastype 完全开源、行为可审计：不联网上报，Web 界面只监听本机回环（127.0.0.1），除 `config.json`
外不读写任何文件。遇到误报时，可在杀毒软件中加入信任/排除（Windows Defender：病毒和威胁防护
设置 → 排除项），或从源码自行编译。

**macOS 提示需要「辅助功能」权限？**
macOS 通过 CGEventTap 监听全局按键、CGEventPost 注入改写后的按键，系统要求先授予
「辅助功能」权限（这是所有按键改写类工具如 Karabiner 的统一要求）。授权对象取决于运行方式：
**Fastype.app**（DMG 安装）或终端 App（命令行运行）。未授权时 fastype 不会退出：菜单栏图标
保持「等待授权」状态并每 2 秒自动检测，开关打开后几秒内自动生效，无需重启。
`--dry-run` 只读模式不需要授权。

**重新安装新版本后授权失效了？（macOS）**
fastype 的 DMG 目前是 ad-hoc 签名（没有 Apple 开发者证书）。自 2026-08 构建起，签名要求
已固定为应用标识（bundle identifier），正常情况下**升级新版本后辅助功能授权会保留**。
若升级后界面仍显示「等待授权」：到 系统设置 → 隐私与安全性 → 辅助功能，
选中 Fastype 按「−」删除，再点「＋」重新添加并打开开关（几秒内自动生效）。
fastype 检测到未授权会自动弹出通知并打开该设置页。

**游戏里会生效吗？**
部分游戏及其反作弊会无视程序合成的按键（Windows 与 macOS 同理），请不要在游戏中依赖 fastype，
也请遵守游戏规则。

**退出后按键会卡住吗？**
不会。托盘退出或 Ctrl+C 时会自动释放所有按住的合成键。

**能双开吗？**
不能。启动时若检测到已有实例会直接提示退出，避免两个钩子互相干扰。

**临时不想用怎么办？**
托盘/菜单栏「暂停按键映射」，或配置页面右上角暂停按钮；彻底不用就退出程序。

**支持哪些系统？**
Windows x64 与 macOS（Intel / Apple Silicon，macOS 13+）。

## 从源码构建

需要 Go ≥ 1.22（macOS 端另需 Xcode Command Line Tools 提供 clang），零第三方依赖：

```
go test ./...

# Windows
go build -ldflags "-s -w -H windowsgui" -o dist\fastype.exe .\cmd\fastype
go build -ldflags "-X main.debugDefault=1" -o dist\fastype-debug.exe .\cmd\fastype

# macOS（本机构建）
go build -ldflags "-s -w" -o dist/fastype ./cmd/fastype
go build -ldflags "-X main.debugDefault=1" -o dist/fastype-debug ./cmd/fastype

# macOS 交叉构建另一架构
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC="clang -arch x86_64"  go build -o dist/fastype-macos-amd64  ./cmd/fastype
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CC="clang -arch arm64"  go build -o dist/fastype-macos-arm64  ./cmd/fastype

# macOS 打包 .app + DMG（universal，含图标与 ad-hoc 签名）
scripts/package-macos.sh
```

Windows：`fastype.exe` 日常使用（无窗口后台运行）；`fastype-debug.exe` 带控制台，
启动即输出按键判定日志。macOS 同理（`fastype-debug` 带调试日志）。

## 来源与致谢

- 设计理念源自 [keyboard](https://github.com/xiongyihui/keyboard)
- [TMK](https://github.com/tmk/tmk_keyboard) 的 Layers 设计
- Jason Rudolph 的 [Toward a more useful keyboard](https://github.com/jasonrudolph/keyboard)
- [QMK](https://qmk.fm) / [VIA](https://www.caniusevia.com) 社区——层语义与配置界面的灵感来源
