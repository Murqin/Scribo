# AGENTS.md

Instructions for coding agents working on Scribo. Humans want
[CONTRIBUTING.md](CONTRIBUTING.md) — it covers the same ground with more
explanation. This file is the short, directive version, weighted towards the
things that are easy to get wrong here.

## Before you write anything

```bash
make test        # go test -race ./...
gofmt -l .       # must print nothing
go vet ./...
```

Those three are the CI gate, plus `CGO_ENABLED=0 go build`. A pull request that
fails any of them will not merge.

## The traps

**The `.gitignore` is a whitelist.** It starts with `*` and re-includes files
one by one. **A new file you create is invisible to git until you add it.** This
is the single most common way work gets lost here. After adding any file:

```bash
git status --short          # your file must appear as untracked
git add -An path/to/file    # must say "add 'path/to/file'"
```

`git check-ignore -v` is not a reliable check: it exits 0 for a negation match
too, so an un-ignored file still looks "ignored".

**Empty is not unset.** `config.getEnv` treats an empty environment variable as
absent and returns the default — but `HISTORY_FILE` deliberately does not, because
an empty value is how a user turns transcript storage off. Before you add an
environment variable to `docker-compose.yml`'s `environment:` block, check which
behaviour it has: a variable declared there is *set* even when it resolves to
nothing.

**Both READMEs, always.** `README.md` and `README.tr.md` are full counterparts,
not an original and a summary. Change one, change the other.

**Catalogs stay parallel.** `i18n/locales/en.json` and `tr.json` must hold the
same keys, and format verbs must match. `i18n` has tests that fail otherwise.
The default mode sets are per language too: `mode/default_modes.en.json` and
`mode/default_modes.tr.json`.

**Redact before logging.** Telegram file URLs and the Gemini endpoint carry
credentials in the URL, and `net/http` quotes the full URL in its error text.
Any error that reaches a log line or a chat message goes through
`cfg.Redact()` first. There are tests for this; do not remove them.

**No listening sockets.** Scribo reaches Telegram by outbound long polling. Do
not add an HTTP server, a health endpoint, a metrics port, or an `EXPOSE`. The
deployment story — no domain, no certificate, no reverse proxy — depends on it.

**One dependency.** `go.mod` has exactly one require line and the binary is
static (`CGO_ENABLED=0`). Adding a dependency is an architectural decision, not
an implementation detail: SQLite was rejected for the history store on these
grounds. Raise it before you write the code.

## Testing expectations

Tests here are expected to *catch* things, and that is checked by mutation: break
the code deliberately, confirm a test fails, restore it. If a mutation survives,
either write the test or write down why the mutation is acceptable — there is a
precedent for the latter in `history.Append`'s comment about the mutex.

Mutations worth trying on anything you touch: delete the call entirely, invert a
condition, drop an argument, return the zero value, skip an authorisation check.

## Style

**Everything in and around code is English.** Identifiers, comments, docstrings,
log messages, error strings, commit messages, file names. The user-facing
interface is bilingual and lives in the catalogs; the code is not.

**Comment the why, not the what.** The test is whether a reader could work it out
from the code. If yes, no comment. If no — the reason for the chosen approach,
the trap avoided, the ordering constraint — write it. Do not write comments that
translate code into prose, section-header decorations, or commented-out code.

**Match the surrounding code.** Where a local convention differs from a general
Go one, the local convention wins.

## Commits and pull requests

Conventional-commit style, matching what is already in the log:

```
feat(i18n): give every command an English name and a Turkish alias
fix(docker): stop compose from prompting for settings that already have defaults
docs: add Turkish README and fix stale claims in the English one
```

The body explains *why*, in prose, wrapped at about 80 columns. Look at recent
commits before writing one — they are the specification for this.

Version tags are derived from commit messages by the release workflow: `feat:`
bumps the minor version, `#major` or `BREAKING CHANGE:` the major, anything else
the patch.

Branch from `main`, open a pull request, let CI finish. Do not commit to `main`
directly.

**Do not add AI attribution.** No `Co-Authored-By` trailer for an assistant, no
"generated with" line in a pull request body. The work is published under the
repository owner's name.

## Where things are

| Path | What lives there |
| :--- | :--- |
| `main.go` | Startup order — language before modes, both before the bot |
| `bot/` | Telegram plumbing, command dispatch, media flow, output formatting |
| `provider/` | `AIProvider` interface, Google and OpenRouter implementations |
| `mode/` | Mode registry, `modes.json` loading, per-language default sets |
| `i18n/` | Embedded message catalogs and language selection |
| `history/` | JSONL transcript store, also the spending ceiling's memory |
| `budget/` | Daily and monthly spending windows |
| `config/` | Environment and `.env` parsing, validation, redaction |

Project state, plans and decision records are **not** in this repository. If you
need the reasoning behind an existing design, read the commit that introduced it.
