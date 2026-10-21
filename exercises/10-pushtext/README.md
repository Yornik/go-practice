# 10 · The moon emoji broke Android

**Tier 2 · about 1.5 h** · Go: strings as bytes vs runes, `for range` over a string, `unicode/utf8` (`RuneLen`, `DecodeRuneInString`, `ValidString`, `RuneError`), `strings.Builder`, reading a fuzz test · Problem solving: decide which unit every limit counts; test every boundary with multi-byte text

`text[:178]`: that is how `notify`, a hackathon prototype that nobody ever rewrote, cuts each of
HUSH's morning summaries to the 178-byte preview field of a push. A Go string is a row of bytes,
and 🌙 is 4 of them. Under a long cluster name, the cut lands inside the moon of "HUSH resolved 3
incidents while you slept 🌙", and the payload is no longer valid UTF-8. For 2 hours, while the
neon of the night market fades into the morning, the Vesper Labs app crashes on every Android
phone that gets such a summary. Marisol Quispe Mamani, the support lead, comes off a call with
Ilona Varga, who runs a night bakery: "Ilona says her phone shouts VESPER LABS HAS STOPPED every
time her ovens are calm." The crash log says only `invalid UTF-8 at byte offset 175`, and your
editor counts characters, not bytes. In this exercise you write the text helpers that `notify`
never had: cut text to a limit, repair broken text, and turn a byte offset into a line and a
column.

## Task

Implement the stubs in `pushtext.go`. `Ellipsis` is given.

```go
const Ellipsis = "…" // given; 1 rune, 3 bytes in UTF-8

func TruncateBytes(s string, limit int) string // at most limit bytes, never half a rune
func TruncateRunes(s string, limit int) string // at most limit runes
func Clean(s string) string                    // valid UTF-8 without control characters

// Position returns the 1-based line and the 1-based rune column of the byte at
// offset. ok is false when offset is outside [0, len(s)] or inside a rune.
func Position(s string, offset int) (line, col int, ok bool)
```

Suggested order: `TruncateBytes`, then `TruncateRunes`, then `Clean`, then `Position`. Steps 1 to
4 each test 1 function, so you can also take them in any other order.

## Rules

### Bytes, runes and invalid bytes

- `len(s)` counts bytes. A rune is 1 Unicode code point, and UTF-8 encodes it in 1 to 4 bytes:
  `a` is 1 byte, `é` (U+00E9) is 2, `…` is 3, `🌙` is 4.
- A byte is **invalid** when it is not part of a valid UTF-8 encoding. Go's decoder decides what is
  valid: a lone byte from `0x80` to `0xFF`, an emoji with its last bytes cut off (`\xf0\x9f\x8c`),
  an overlong form (`\xc0\xaf`) and an encoded surrogate half (`\xed\xa0\x80`) are all invalid.
- Everywhere in this package, **an invalid byte counts as a rune of 1 byte**. A `for range` loop
  over a string sees it that way, and `utf8.RuneCountInString` counts it that way. So in
  `"ab\xf0\x9f\x8ccd"` every byte is the start of a rune, and the text has 7 runes.
- A **combining mark** is a rune of its own. `"Caf\u00e9"` has a precomposed `é` and 4 runes.
  `"Cafe\u0301"` has an `e` and the combining acute accent U+0301. It looks the same, but it has 5
  runes. This README and the tests write combining marks as `\u` escapes, so that you can see them.

### TruncateBytes

- If `len(s) <= limit`, it returns `s` unchanged.
- Otherwise, if `limit >= 3`, it returns the longest prefix of `s` that holds only whole runes and
  at most `limit - 3` bytes, followed by `Ellipsis`. The result has at most `limit` bytes.
- Otherwise, if `limit` is 1 or 2, there is no room for `Ellipsis`. It returns the longest prefix
  of whole runes that fits in `limit` bytes, without `Ellipsis`. That can be `""`: `🌙` does not
  fit in 2 bytes.
- If `limit <= 0`, it returns `""`.
- It never cuts a valid rune in half. An invalid byte is a rune of 1 byte, so a cut may fall
  between 2 invalid bytes.
- It does not repair text: invalid bytes in the result stay as they are. Call `Clean` first if you
  need valid UTF-8 from any input. For a valid `s` the result is valid UTF-8.

### TruncateRunes

- If `s` has at most `limit` runes, it returns `s` unchanged.
- Otherwise it returns the first `limit - 1` runes of `s`, followed by `Ellipsis`. `Ellipsis` is 1
  rune, so the result has exactly `limit` runes. For `limit == 1` the result is `"…"`.
- If `limit <= 0`, it returns `""`.
- It counts runes as above: an invalid byte is 1 rune and stays as it is, and a combining mark is
  a rune of its own. So the cut may separate an accent from its letter:
  `TruncateRunes("Cafe\u0301 au lait", 5)` is `"Cafe…"`.

### Clean

- Each **run** of invalid bytes (1 or more invalid bytes in a row in `s`) becomes 1 U+FFFD, the
  replacement character `�`. `"a\xff\xfe\xfdb"` becomes `"a�b"`.
- A valid U+FFFD that is already in `s` is an ordinary rune. `Clean` keeps it, and it does not join
  a run: `"�\xff"` becomes `"��"`.
- `Clean` removes the control characters U+0000 to U+001F and U+007F, except tab (U+0009) and
  newline (U+000A). So it removes `\r`, NUL and ESC.
- `Clean` finds the runs in `s` itself, before it removes anything. In `"\xff\x00\xff"` the NUL
  separates 2 runs, so the result is `"��"`.
- `Clean` keeps every other rune unchanged, also U+0080 to U+009F and invisible runes such as
  U+200B. Text that is valid and has no control characters comes back unchanged.

### Position

- `offset` is a byte offset into `s`. `Position` returns the line and the column of the rune that
  starts at that byte, and `ok == true`.
- Line numbers start at 1. A line ends at `\n` only. A `\r` is an ordinary character, so in
  `"ab\r\ncd"` the `\r` is column 3 of line 1. The `\n` itself is the last character of its line.
- Column numbers start at 1 and count **runes**, not bytes: in `"é\n🌙x"` the `x` at byte 7 is
  line 2, column 2. A tab is 1 column. An invalid byte is a rune of 1 byte, so it is 1 column.
- `offset == len(s)` is valid: it is the position right after the last character. For `""` and
  offset 0 that is line 1, column 1. For `"ab\n"` and offset 3 it is line 2, column 1.
- If `offset < 0`, `offset > len(s)`, or `offset` points at the second, third or fourth byte of a
  valid multi-byte rune, it returns `0, 0, false`.

### FuzzTruncate

The test file has a fuzz test. Plain `go test` runs it on its seeds. It checks 5 properties that
follow from the rules above, for any `s` and any `limit`. For `TruncateBytes`:

- A text that fits comes back unchanged.
- `limit <= 0` gives `""`.
- The result has at most `limit` bytes.
- The result is valid UTF-8 when `s` is.
- The result is a prefix of `s`, maybe followed by `Ellipsis`.

For `TruncateRunes` it checks the same 5 properties, with runes instead of bytes.

## Examples

```text
TruncateBytes("HUSH resolved 3 incidents while you slept 🌙 Have a calm day.", 49)
    → "HUSH resolved 3 incidents while you slept 🌙…"  (46 + 3 bytes)
TruncateBytes("HUSH resolved 3 incidents while you slept 🌙 Have a calm day.", 48)
    → "HUSH resolved 3 incidents while you slept …"   (🌙 does not fit in 45)
TruncateBytes("Varga Pékség", 14)    → "Varga Pékség"  (14 bytes: it fits)
TruncateBytes("Varga Pékség", 12)    → "Varga Pé…"     (room for 9 bytes; "Varga Pék" needs 10)
TruncateBytes("Ọjà Funmi", 8)        → "Ọj…"           (room for 5 bytes; Ọ is 3 bytes, à is 2)
TruncateBytes("مخبز الأمل", 10)      → "مخب…"          (room for 7 bytes; each letter is 2)
TruncateBytes("Ọjà Funmi", 3)        → "…"
TruncateBytes("Ọjà Funmi", 2)        → ""              (no room for the ellipsis, and Ọ needs 3)
TruncateBytes("Varga Pékség", 2)     → "Va"
TruncateBytes("🌙🌙", 2)             → ""
TruncateBytes("ab\xf0\x9f\x8ccd", 6) → "ab\xf0…"       (3 invalid bytes are 3 runes of 1 byte)

TruncateRunes("🌙🌙🌙🌙", 3)           → "🌙🌙…"
TruncateRunes("مخبز الأمل", 10)        → "مخبز الأمل"  (10 runes in 19 bytes: it fits)
TruncateRunes("Ọjà Funmi", 4)          → "Ọjà…"
TruncateRunes("ab", 1)                 → "…"
TruncateRunes("Cafe\u0301", 5)         → "Cafe\u0301"  (5 runes: it fits)
TruncateRunes("Cafe\u0301 au lait", 5) → "Cafe…"       (the accent is cut off)

Clean("ab\xf0\x9f\x8c")       → "ab�"
Clean("line 1\r\nline 2\x00") → "line 1\nline 2"
Clean("\xff\x00\xff")         → "��"

Position("ab\ncd", 3) → 2, 1, true
Position("é\n🌙x", 7) → 2, 2, true
Position("é\n🌙x", 4) → 0, 0, false  (inside 🌙)
Position("abc", 4)    → 0, 0, false
```

The tests also use a push text of 2 lines, `pushText`: a cluster name of 132 bytes (124 runes) on
line 1, and `HUSH resolved 3 incidents while you slept 🌙 Have a calm day.` on line 2. The 🌙 starts
at byte 175, so `pushText[:178]` keeps 3 of its 4 bytes. `TruncateBytes(pushText, 178)` ends in
`while you slept …`, `Position(pushText, 175)` is line 2, column 43, and `Clean(pushText[:178])`
ends in `while you slept �`.

## Run the tests

    go test ./exercises/10-pushtext/
    go test -run TestTruncateBytes -v ./exercises/10-pushtext/      # step 1: TruncateBytes
    go test -run TestTruncateRunes -v ./exercises/10-pushtext/      # step 2: TruncateRunes
    go test -run TestClean -v ./exercises/10-pushtext/              # step 3: Clean
    go test -run TestPosition -v ./exercises/10-pushtext/           # step 4: Position
    go test -run FuzzTruncate -v ./exercises/10-pushtext/           # step 5: the fuzz seeds
    go test -run TestTruncateBytesEveryCut -v ./exercises/10-pushtext/   # one test
    go test -fuzz=FuzzTruncate ./exercises/10-pushtext/  # optional: let Go invent inputs, Ctrl-C to stop

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The 5 filters above do not overlap. Step 5 needs steps 1 and 2. The example
(`ExampleTruncateBytes`) runs only in a plain `go test` and needs `TruncateBytes` and `Clean`. An
example cannot recover a panic, so a plain run crashes until both work.

Read `fuzz_test.go` once you have step 2. It is a short fuzz test, and later exercises such as
24-durations, 25-syslog and 26-logfmt have one too. A fuzz test states properties instead of
examples. With `-fuzz`, Go mutates the seeds. When a property fails, Go saves the input under
`testdata/fuzz/FuzzTruncate/`, and every later plain `go test` runs it again.

## Docs you can use

- [Strings, bytes, runes and characters in Go](https://go.dev/blog/strings) (Go blog): read this
  one first
- [The Go spec: For statements with range clause](https://go.dev/ref/spec#For_range), the row for
  strings, and [Conversions to and from a string type](https://go.dev/ref/spec#Conversions_to_and_from_a_string_type)
- [`unicode/utf8`](https://pkg.go.dev/unicode/utf8): `DecodeRuneInString`, `RuneLen`,
  `RuneCountInString`, `ValidString`, `RuneStart` and the constant `RuneError`
- [`strings`](https://pkg.go.dev/strings): `Builder`, `Map`, `ToValidUTF8`
- [Go fuzzing](https://go.dev/security/fuzz/) and the [fuzzing tutorial](https://go.dev/doc/tutorial/fuzz)
- [UTF-8 on Wikipedia](https://en.wikipedia.org/wiki/UTF-8), the table of byte patterns under
  "Encoding"
- [Text normalization in Go](https://go.dev/blog/normalization) (Go blog), on combining marks

## Stretch goals

- Cut at a word: if there is a space in the last 20 bytes before the cut, cut there instead, so
  that a push never ends in `incid…`. Marisol asks for this one the morning after the fix.
- Count what a person sees: `TruncateRunes` separates `e` from its accent and splits 👍🏽 into 2.
  Normalize to NFC first with `golang.org/x/text/unicode/norm`, or cut at grapheme clusters with
  `github.com/rivo/uniseg`, and compare the results on the combining-mark tests.
- Editors and the Language Server Protocol count columns in UTF-16 code units by default, where
  🌙 is 2 columns. Write `PositionUTF16` with `unicode/utf16.RuneLen`.
- Benchmark `TruncateBytes` on a text of 1 MB. Then write a version that starts at the cut and
  steps back with `utf8.RuneStart`. Is it faster? For which inputs from `TestTruncateBytesInvalid`
  does it give a different answer, and why?

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 10 hint 1
    go run ./cmd/peek 10 hint 2
    go run ./cmd/peek 10 hint 3
    go run ./cmd/peek 10 explanation
    go run ./cmd/peek 10 walkthrough
    go run ./cmd/peek 10 solution
