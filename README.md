# subplay

A terminal subtitle player. Load an SRT file and play it in sync with a movie running in another window.

## Usage

```
subplay [options] <file.srt>

Options:
  -s2 <file.srt>   second subtitle file (shown in amber, obscured by default)
```

### Single track

```
subplay movie.en.srt
```

### Dual track (language learning)

```
subplay -s2 movie.native.srt movie.target.srt
```

The target language (primary) is shown in white. The native language (secondary) starts obscured — use `r` to reveal it for the current subtitle, or `R` to toggle global obscuring on/off.

## Keyboard shortcuts

| Key | Action |
|-----|--------|
| `Space` | Play / pause |
| `→` or `n` | Jump to next subtitle |
| `←` or `p` | Jump to previous subtitle |
| `t` | Jump to timestamp (enter `HH:MM:SS`, `MM:SS`, seconds, or Go duration like `1h30m`) |
| `r` | Reveal current secondary subtitle (dual-track, obscure mode) |
| `R` | Toggle secondary subtitle obscuring on/off |
| `q` | Quit |

## Install

```
go install github.com/cdavison/subplay@latest
```

Or build from source:

```
git clone https://github.com/cdavison/subplay
cd subplay
go build -o subplay .
```

## SRT format

Standard SRT files are supported. HTML tags (`<i>`, `<b>`, `<font>`, etc.) are stripped automatically. Both Unix and Windows line endings are handled.
