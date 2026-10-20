# codex-chinese

**Force the Codex desktop app to use its built-in `zh-CN` interface** — even when
Statsig / VPN is unreachable and the app would otherwise fall back to English.

A single-file, zero-dependency Go binary (≈3.5 MB) for Windows, macOS and Linux.

## Why

Codex ships with a built-in Chinese language pack, but enabling it is gated behind a
Statsig flag (`Layer`/`DynamicConfig` `"72216192"`, keys `enable_i18n` and
`locale_source`). When Statsig can't be reached, the flag has no value and the UI
reverts to English.

`codex-chinese` patches that flag at runtime.

## How

It launches Codex with `--remote-debugging-port`, connects over the Chrome DevTools
Protocol, and injects a prelude into the renderer's **main world** via
`Page.addScriptToEvaluateOnNewDocument`. The prelude:

1. spoofs `navigator.language` / `navigator.languages` to `zh-CN`;
2. wraps the Statsig client's `getLayer` / `getDynamicConfig` / `_getLayerImpl` so that
   config `"72216192"` always returns `{ enable_i18n: true, locale_source: "SYSTEM" }`;
3. writes `localeOverride = zh-CN` and reloads once.

It does **not** modify Codex's installation, `app.asar`, or config files, and it never
makes network requests.

## Usage

Fully quit Codex first (including the tray icon), then:

```bash
codex-chinese-windows-amd64.exe      # Windows
./codex-chinese-darwin-arm64         # macOS / Linux
```

Flags: `-codex <path>`, `-port <n>`, `-wait <n>`, `-no-launch`, `-no-reload`,
`-version`, `-doctor`, `-verify`.

### Verify it actually worked

```bash
codex-chinese-windows-amd64.exe -verify
# verify: installed=true lang=zh-CN i18n=true
# PASS                          <- exit code 0 is the only success criterion
```

All three values must hold: `installed=true`, `lang=zh-CN`, `i18n=true`.
The `完成：Codex 已强制中文` line printed by the main run is just a progress
message — it is **not** proof of success.

> **Never wrap this binary in a `.vbs` / `.bat` launcher, and never point a
> shortcut at `wscript.exe` or `cmd.exe`.** A wrapper does not pass
> `--remote-debugging-port`, so there is no CDP session and the injection can
> never happen — the UI simply stays English with no error shown.
> Shortcuts must target `codex-chinese*.exe` directly.

### Windows tray

Run `codex-chinese-tray-windows-amd64.exe` (keep `codex-chinese.exe` in the same folder).
Right-click the tray icon for **Force Chinese / Doctor / Quit**.

## Build

Requires Go ≥ 1.21, no third-party modules. Node ≥ 18 is only needed for tests.

```bash
node test/prelude.test.mjs       # prelude regression tests (11 assertions)
go vet ./...
VERSION=v1.2.0 bash build.sh
```

## AI-driven deployment

- `AGENTS.md` — project rules and entry point for coding agents
- `docs/AI_DEPLOY.md` — deterministic deployment spec
- `deploy.json` — machine-readable build/artifact/validation manifest
- `scripts/ai_deploy.sh` / `ai_deploy.ps1` — one-shot build + self-check, emits JSON

## Disclaimer

Unofficial community tool, not affiliated with OpenAI. Runtime-only modification.
If Codex changes the Statsig layer/key names, the tool may stop working.

## License

[AGPL-3.0](LICENSE). Approach inspired by
[CodexPlusPlus](https://github.com/BigPizzaV3/CodexPlusPlus); this is an independent
reimplementation.
