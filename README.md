# histwise

**Learn from your own shell history.** `histwise` reads your `bash`, `zsh`, or
`fish` history and shows what you actually type the most — then suggests
aliases ranked by the keystrokes they would *really* save you (frequency ×
length), not just raw counts.

It is a single static binary. It works completely offline, never phones home,
and never modifies your history files.

```
$ histwise
histwise  (zsh: /home/you/.zsh_history)

Analyzed 18422 commands, 3140 unique.

Top programs
  git     ########################  4821
  docker  ############              2110
  kubectl ########                  1455
  ...

Alias suggestions (ranked by keystrokes saved)
  gs     = git status                               x612, ~4896 saved
  gco    = git checkout                             x330, ~2970 saved
  dcup   = docker compose up --build                x120, ~2640 saved

Tip: histwise --aliases >> ~/.bash_aliases
```

## Why

Everyone *means* to set up aliases but never gets around to it, and when they
do they guess at what matters. `histwise` answers it with data: it ranks
candidates by `(command_length - alias_length) * times_used`, so the
suggestions at the top are the ones that pay back the most typing.

## Install

With Go 1.21+:

```sh
go install github.com/vishalkaaay/histwise@latest
```

Or build from source:

```sh
git clone https://github.com/vishalkaaay/histwise
cd histwise
go build -o histwise .
```

## Usage

```
histwise [flags]

  --file PATH    analyze a specific history file (default: auto-detect)
  --shell NAME   force shell format: bash | zsh | fish
  --json         emit machine-readable JSON
  --aliases      print a sourceable alias file
  --top N        entries to show in each list (default 15)
  --min N        minimum occurrences for an alias suggestion (default 3)
  --all          include common noise commands (ls, cd, ...)
  --version      print version
```

### Generate and use aliases

```sh
# Review first, then append to your shell config:
histwise --aliases
histwise --aliases >> ~/.bash_aliases   # bash
histwise --aliases >> ~/.zshrc          # zsh
```

Fish uses `alias name cmd` syntax, which `histwise` emits automatically when it
detects a fish history.

### Scripting

```sh
histwise --json | jq '.alias_suggestions[] | .alias + " = " + .command'
```

## How it works

- **Detection** — uses `$SHELL`, then falls back to probing the standard
  history file locations.
- **Parsing** — understands bash plain lines (and `HISTTIMEFORMAT` epoch
  comments), zsh extended `: <epoch>:<elapsed>;cmd` entries with multi-line
  continuations, and fish's YAML-ish `- cmd:` records.
- **Scoring** — commands are whitespace-normalized and counted; alias
  candidates must be long enough and win by more than two characters per use,
  with collisions resolved in favor of the more frequently used command.

`NO_COLOR` is honored, and color is disabled automatically when output is piped.

## Privacy

`histwise` only reads local files you point it at. No network access, no
telemetry, no writes to your history. The alias file is printed to stdout; you
decide whether to save it.

## Development

```sh
go test ./...
go vet ./...
```

## License

MIT — see [LICENSE](LICENSE).
