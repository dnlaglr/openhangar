# Telemetry and Logging

Hangar runs on your private code. This document lists everything Hangar records about itself, where it goes and how to turn it off. If the code ever disagrees with this page, please treat that as a bug and report it.

## Summary

| What                            | Leaves your machine?                       | On by default?                       |
| ------------------------------- | ------------------------------------------ | ------------------------------------ |
| Local Logs                      | No, never                                  | Yes                                  |
| Diagnostics Bundle              | Only if you attach it to an issue yourself | No, you create it by hand            |
| Crash Reports & Usage Analytics | Not yet implemented                        | **Off** by default, until you opt-in |

**Hangar sends no telemetry today.** There is no analytics SDK, crash reporter or background upload in the codebase.

## Local Logs

The engine writes structured JSON logs so problems can be diagnosed after the fact.

- **Where:** `~/.cache/hangar/logs/` inside the engine's environment. On Windows that is inside the WSL distro Hangar uses. On macOS it is `~/Library/Caches/hangar/logs/`.
- **Retention:** files rotate at 10 MB. Hangar keeps at most 5 old files, and deletes files older than 14 days.
- **Contents:** timestamps, log levels, event messages, error messages, and internal IDs (workspace, chat, turn and host IDs, which are random UUIDs).
- **Redaction:** every line is redacted before it is written:
  - Your home directory becomes `~`. Your Windows home becomes `<winhome>`.
  - All URLs become `scheme://<redacted>`, including git remotes and Tower addresses. So do `user@host:path` remotes.
  - API keys, bearer, OAuth and GitHub tokens, JWTs and other credential-shaped strings are replaced with `<redacted>`, as are values whose name looks like a secret (`token`, `password`, `api_key`, ...).
  - Email addresses, your machine's hostname and `hangar/<name>` branch names are masked.
- **Debug mode:** turning on Debug mode in the app adds detailed debug lines. It also records raw agent output (which contains your code and prompts) to `logs/raw/`. That directory never leaves your machine and is never included in a diagnostics bundle.

Redaction is pattern-based and can miss things. That is why logs never leave your machine on their own.

## Diagnostics Bundle

When you choose **Export Diagnostics**, Hangar writes a zip file to a location you pick. Nothing is uploaded. You can open the zip and review it before attaching it to a GitHub issue.

The bundle contains:

- `logs/`: your recent local logs, redacted again on export. `logs/raw/` is always excluded.
- `system.json`:
  - Hangar app, engine and guest versions, and the engine's build revision
  - OS name and release, kernel release, WSL version, CPU architecture and core count
  - Sandbox runtime type (`worktree` or `firecracker`) and whether KVM is available
  - Whether telemetry is disabled

The bundle never contains your code, prompts, repository contents, file paths outside the redacted forms above, credentials, or your Tower address.

## Crash Reports and Analytics (Planned)

A future release may offer crash reports and anonymous usage analytics to help prioritize fixes. When it does:

- They will be **opt-in**, off until you turn them on.
- They will go through a single choke point that sends only fixed event names and numbers, never free-form text such as messages, paths or names.
- This document will be updated in the same release, listing every field sent and where it goes.

## Opting Out

Set either of these environment variables to any value other than empty, `0` or `false`:

```sh
DO_NOT_TRACK=1
HANGAR_TELEMETRY_DISABLED=1
```

`DO_NOT_TRACK` is the cross-tool convention from [consoledonottrack.com](https://consoledonottrack.com). Either variable turns off telemetry even if you opted in inside the app. It does not affect local logs or diagnostics bundles, because neither sends anything.

On Windows the engine runs inside WSL, so the variable has to reach it. Hangar forwards both variables into WSL when it starts the engine. If you start the engine yourself, set the variable inside the distro, or add it to `WSLENV` (for example `WSLENV=DO_NOT_TRACK/u`).

## Questions

Open an issue on the repository if anything here is unclear, or if you find Hangar recording something this page doesn't list.
