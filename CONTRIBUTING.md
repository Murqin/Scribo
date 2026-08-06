# Contributing to Scribo

Thanks for taking the time. Scribo is a small Go project with a deliberately
narrow shape — one dependency, one static binary, no listening ports — and most
of what follows exists to keep it that way.

Coding agents: [AGENTS.md](AGENTS.md) is the condensed version of this file.

Issues and discussion are welcome in **English or Turkish**. Code, comments and
commit messages are English only; see [Style](#style) for why.

## Getting set up

You need Go 1.21 or newer. Nothing else — no database, no build tooling, no
services to run.

```bash
git clone https://github.com/Murqin/Scribo.git
cd Scribo
make test      # should pass on a fresh clone
make build     # produces ./scribo
```

To actually run it you need a Telegram bot token from
[@BotFather](https://t.me/botfather) and a free
[Google AI Studio](https://aistudio.google.com/) key:

```bash
cp .env.example .env
$EDITOR .env       # TELEGRAM_TOKEN, ALLOWED_USER_ID, GEMINI_API_KEY
./scribo
```

Use a throwaway bot for development. The bot answers only the IDs in
`ALLOWED_USER_ID`, so put your own there — ask [@userinfobot](https://t.me/userinfobot)
for it.

## Before you open a pull request

```bash
make test        # go test -race ./...
gofmt -l .       # must print nothing
go vet ./...
```

CI runs exactly these plus `CGO_ENABLED=0 go build`. Everything has to pass.

Branch from `main`, push the branch, open a pull request. Please don't commit to
`main` directly — the release workflow tags every push there.

## The one thing that catches everybody

**`.gitignore` is a whitelist.** It begins with `*` and then re-includes files
explicitly:

```
# 1. Ignore everything by default
*
!*/

# 2. Whitelist project configuration & metadata
!.gitignore
!LICENSE
...
```

A file you add is invisible to git until you add a line for it. If your new
source file is not in `git status`, this is why. Check with:

```bash
git add -An path/to/your/file    # should print: add 'path/to/your/file'
```

(`git check-ignore -v` will *not* tell you reliably — it exits 0 when a negation
pattern matches too.)

## How the pieces fit

| Package | Responsibility |
| :--- | :--- |
| `config` | Reads the environment and `.env`, validates, and redacts secrets from strings |
| `i18n` | Embedded message catalogs; picks the language from `SCRIBO_LANG` |
| `mode` | The mode registry, `modes.json` loading, per-language default sets |
| `provider` | The `AIProvider` interface and its Google and OpenRouter implementations |
| `bot` | Telegram plumbing: commands, the media flow, output formatting |
| `history` | Append-only JSONL transcript store, and where the spending ceiling gets its memory |
| `budget` | Daily and monthly spending windows |

`main.go` wires them together, and its ordering matters: the language has to be
settled before the modes are, because the mode package builds its button labels
and prompts from the active catalog.

## Design constraints

These are not preferences; changing one changes what the project is. If your
change needs to bend one, open an issue first and let's talk about it.

**One dependency.** `go.mod` requires exactly one module. The binary is static
(`CGO_ENABLED=0`) and runs anywhere with no runtime. When persistent history was
added, SQLite was rejected for this reason and a plain JSONL file was used
instead — a full file scan is affordable for a bot serving one person.

**No listening sockets.** Scribo talks to Telegram by outbound long polling. That
is what lets it run with no domain, no certificate, no open port and no reverse
proxy — including inside a container on a home server. Please don't add an HTTP
server, a health endpoint or a metrics port.

**Bilingual by construction.** The interface *and* the prompts follow
`SCRIBO_LANG`, so the bot answers in the language it was configured for. Anything
user-visible goes in `i18n/locales/*.json`, and both catalogs must carry the same
keys — there are tests that fail if they drift.

**Secrets never reach a log or a chat.** Telegram file URLs and the Gemini
endpoint carry credentials in the URL itself, and `net/http` quotes the whole URL
in its error messages. Every error that surfaces goes through `cfg.Redact()`
first.

## Testing

Run `make test`. New behaviour needs a test; that part is ordinary.

What is less ordinary is the bar: **a test is expected to fail when the code it
covers is broken.** The practice here is to check that by mutation — delete the
call, invert the condition, drop the argument, return the zero value, skip the
authorisation check — and confirm something goes red. If a mutation survives,
either the test is missing or the mutation is genuinely harmless; the second case
gets written down. There is an example in `history.Append`, where removing the
mutex is documented as acceptable because `O_APPEND` is what makes the write
atomic.

Two things worth knowing when writing tests:

- `bot` tests use a mock Telegram client and assert on what was *sent*, not on
  what was logged.
- `useLanguage(t, "en")` switches the catalog for one test and restores it after.

## Style

**Everything in and around code is English:** identifiers, comments, docstrings,
log and error messages, file names, commit messages. The bot speaks Turkish and
English to its users, and that lives entirely in the catalogs. Keeping the code
in one language keeps it readable to contributors who speak neither.

**Comments explain why, not what.** The test is whether a reader could work it
out from the code itself. If they could, leave it out. If they couldn't — the
reason for the approach, the trap being avoided, the ordering constraint, a link
to the source — write it down. Please skip comments that restate the code,
decorative section headers, and commented-out code.

**Follow the surrounding code.** Where local convention and general Go advice
disagree, local convention wins.

## Commit messages

Conventional-commit subjects, and a body in prose that explains the reasoning:

```
feat(i18n): give every command an English name and a Turkish alias

The localization phase translated the interface but not the command names, so
an English install was told, in English, to type a Turkish word.

/last and /start are the canonical names now, with /son and /basla accepted as
aliases. Resolving the name from the catalog at runtime was rejected because
Telegram's own Start button sends /start, which therefore cannot stop working
in any language.
```

The existing log is the specification — read a few entries before writing yours.

Subjects also drive versioning: the release workflow bumps the minor version for
`feat:`, the major for `#major` or `BREAKING CHANGE:`, and the patch otherwise.

Please don't add AI attribution — no `Co-Authored-By` trailer for an assistant,
no "generated with" line in the pull request body.

## Modes and prompts

Prompts and button labels are data, not code. They live in
`mode/default_modes.tr.json` and `mode/default_modes.en.json`, which are embedded
in the binary, and users override them with a `modes.json` in the working
directory. Please don't hardcode a prompt in Go.

Each mode declares a `format` that decides how its output is rendered:

| `format` | Rendering | For |
| :--- | :--- | :--- |
| `code` (default) | Wrapped in `<code>`, one tap copies it | Output you paste somewhere else |
| `markdown` | Rendered as Telegram formatting | Output meant to be read |
| `plain` | Verbatim | Output whose markup should stay literal |

The choice follows what the user does with the result, not how pretty it looks:
a note destined for another program should stay raw and copyable.

## Docker

`docker-compose.yml` targets home server panels (CasaOS, Cosmos Cloud, Portainer,
Dockge) as much as the command line, and a few of its choices are load-bearing:

- **No compose interpolation.** Panels scan the file for it and turn every match
  into a form field, without reading the default that follows — so it must only
  appear for settings that genuinely have no answer.
- **`env_file` is `required: false`,** because a panel import creates no `.env`.
- **`.dockerignore` is a whitelist using `**`, not `*`.** Docker matches patterns
  against whole paths and `*` stops at a slash, so the `git`-style `*` plus `!*/`
  idiom silently leaves everything below the first level in the build context.

## Questions

Open an issue. If you are unsure whether a change fits the constraints above,
asking first is cheaper than writing it twice.
