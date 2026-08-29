# Ask

A Super+A lookup. One box, type what you mean, get the answer.

It is for the things you would otherwise open a browser for: a quick calculation, a currency or unit conversion, a stock or crypto quote, the time in another city, a definition, the weather somewhere else, or opening a site. Type the question the way you would type it into an address bar. Ask guesses from the prompt. When the guess is the wrong kind of answer, a one-letter prefix steers it.

Escape always closes. Enter copies the selected row — or launches Brave — and closes.

Designed together by [Allan Kristensen](https://github.com/allanjorch) and [Grok](https://x.ai).

## Try it

```
2+2
15% of 80
10 km to miles
100 usd to dkk
72f to c
NVDA
NVDA 200
BTC
9am PT
time in Tokyo
ephemeral
w Tokyo
!dr.dk
!omarchy
```

`NVDA 200` is two hundred shares at last price. `!dr.dk` opens that site in Brave. `!omarchy` searches Brave. `!` alone launches the browser.

## Operators

With an empty box, Ask shows these as a faint hint. You do not need them on the happy path. They are filters for when several engines could claim the same prompt.

| Key | Also | Does |
|---|---|---|
| *(none)* | | Try everything, rank, hide misses |
| `m` | `calc` `math` `=` | Arithmetic |
| `c` | `conv` `convert` `~` | Units and currency |
| `s` | `stock` `quote` `ticker` `crypto` `$` | Quote; a number is a position |
| `t` | `time` `tz` `@` | Time in a place, or convert a wall clock |
| `d` | `define` `dict` `?` | Dictionary |
| `w` | `weather` `wx` `forecast` `*` | Weather in a place |
| `!` | | Brave: empty launches, a host opens, a word searches |

Up and down move the selection when more than one row comes back.

## Build

Linux (OpenGL/EGL):

```
make
./ask            # window
./ask '2+2'      # print a result, no window
make install     # ~/.local/bin/ask
make test
```

Windows, from Linux, no CGO:

```
make windows     # ask.exe
```

The UI is [Gio](https://gioui.org). On Omarchy it reads `~/.local/state/omarchy/current/theme/colors.toml` and retints with the desktop.

## Omarchy

Bind Super+A and float the window. In `~/.config/hypr/bindings.lua`:

```lua
o.bind("SUPER + A", "Ask", { launch = "ask", focus = "^ask$" })
```

In `~/.config/hypr/hyprland.lua`:

```lua
o.window("ask", { float = true, center = true, rounding = 12 })
```

Then `hyprctl reload`. Super+A opens Ask, Escape puts it away.

## What it is not

Omacalc stays the four-function keypad on Super+Ctrl+Q. Ask is the oracle next to it: conversions, quotes, time, words, weather, and the browser, from a single prompt.
