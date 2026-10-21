# 05 · Read a .env file without bash

**Tier 1 · about 1.5 h** · Go: indexing a string, `strings.IndexByte`, `strings.Cut`, `strings.CutPrefix`, `strings.Trim`, maps that overwrite, errors with line numbers and `%q` · Problem solving: write the line format down as rules; decide what each line can look like before you code

In your second week at Peckish, the company that puts a camera in a bird feeder, a routine deploy
of the photo upload service fails at 10:40, because the service cannot log in to its database. The
password in `.env` is right: `DB_PASSWORD=pa$$w0rd`. But the deploy script loads that file with
`source .env`. So bash reads the file as code, and in bash `$$` is the process ID of the shell.
The service tried `pa48213w0rd`, and after the retry `pa48377w0rd`: a new password on every try.
Ines Okafor, the senior SRE, wrote that script in the first week of the company. She looks at the 2
passwords, laughs, and writes 1 line under "Cause" in the postmortem: `source .env`. "Config is
data," she says. "Do not run it." In this exercise you write a reader for `.env` files that never
runs anything: it turns the text into a map and names the first line it cannot read.

The `.env` format has no standard. Bash, Docker Compose, systemd, `kubectl` and the dotenv
libraries all read it a little differently. The rules below are the ones Peckish uses: the common
part of those readers, without escapes and without variable expansion.

## Task

Implement the stubs in `envfile.go`:

```go
// ValidKey reports whether key is a valid variable name: an ASCII letter or _,
// then ASCII letters, digits or _.
func ValidKey(key string) bool

// Parse reads the content of a .env file and returns its variables.
func Parse(text string) (map[string]string, error)
```

Suggested order: `ValidKey`, then `Parse` for plain lines, then comments and quotes, then the
errors. `Parse` uses `ValidKey`.

## Rules

### ValidKey

- A valid key has at least 1 byte. The first byte is an ASCII letter (`A` to `Z`, `a` to `z`) or
  `_`. Every other byte is an ASCII letter, an ASCII digit (`0` to `9`) or `_`.
- Lowercase letters are valid. `PORT`, `_x`, `_`, `DB_HOST2`, `db_host` and `export` are valid
  keys.
- `""`, `2FA`, `db-host`, `DB HOST`, `A.B` and `ключ` are not. A letter outside ASCII, such as
  `é` or `к`, is never valid.

### Parse: lines

- In these rules, a **blank** is a space or a tab.
- `Parse` splits the text into lines at every `\n`. If a line ends with `\r` (a Windows line
  ending), `Parse` removes that `\r` before anything else. The last line counts even when the text
  does not end with `\n`.
- `Parse` numbers the lines from 1. Every line counts, even a line that `Parse` skips.
- `Parse` skips 3 kinds of lines: an empty line, a line of only blanks, and a comment line. A
  comment line is a line whose first byte that is not a blank is `#`. So `# settings`,
  `   # indented` and `#PORT=80` are comment lines.
- Every other line defines 1 variable: `KEY=value`. Blanks may come before the key, and before
  `export`.
- An optional `export` in lowercase, followed by at least 1 blank, may come before the key.
  `Parse` drops it: `export PORT=8080` defines `PORT`. A tab works as well as a space, and so do
  several blanks: `  export   PORT=8080` defines `PORT` too. Without a blank after it, `export`
  is part of the key: `export=1` defines the key `export`, and `exported=1` defines `exported`.
  `EXPORT PORT=1` has the key `EXPORT PORT`, which is not valid.
- The **first** `=` in the line separates the key from the value. A line without `=` is an error.
- The key is the text before that `=`, without the blanks around it. It must be valid by
  `ValidKey`: `PORT = 8080` defines `PORT`.
- Keys are case-sensitive: `port=1` and `PORT=2` define 2 different keys.
- If the same key appears on more than 1 line, the **last** definition wins.

### Parse: values

- The value comes from the text after the first `=`. Look at the first byte of that text that is
  not a blank. If it is `'` or `"`, the value is **quoted**; otherwise it is **unquoted**.
- An **unquoted** value runs to the end of the line, with 1 exception: a `#` that comes right
  after a blank starts a comment. The comment runs to the end of the line. The value is the text
  before the comment, without the blanks at its start and at its end. The blanks inside it stay.
- So `MSG=hello world` is `hello world`, `LOG_LEVEL=info # quiet` is `info` and `A==b` is `=b`.
  `A=`, `A=   ` and `KEY= #c` are all `""`. In `KEY= #c` the `#` comes right after a blank.
- A `#` anywhere else is data: `COLOR=#ff0000` is `#ff0000` (the `#` comes right after the `=`),
  and `A=b#c` is `b#c`.
- A quote that is not the first byte of the value is data: `A=x"y` is `x"y`, and `A=it's` is
  `it's`.
- A **quoted** value runs from its opening quote to the next quote of the same kind. The value is
  the text between the 2 quotes, exactly as written: blanks, `#`, `=`, `\` and the other kind of
  quote are all data. `A="it's"` is `it's`, `A='say "hi"'` is `say "hi"`, `A=  "x"` is `x` and
  `A="  a # b  "` is `  a # b  `. `A=""` and `A=''` are `""`.
- There are no escapes. A `\` is an ordinary byte: `A="C:\new"` is `C:\new`, and in
  `A="say \"hi\""` the second `"` closes the value.
- After the closing quote, only blanks may follow, then optionally a comment that starts with `#`.
  `A="x" # note`, `A="x"   ` and `A="x"#note` are all `x`. Anything else is an error.
- A quoted value ends on its own line. If the line has no closing quote, that is an error, even
  when a later line has one.
- `$` is always an ordinary byte, quoted or not. `Parse` never expands variables and never runs
  commands: `pa$$w0rd`, `${HOME}/feeders` and `$(date)` stay exactly as written.

### Parse: the result and errors

- On success the error is `nil` and the map is **not nil**. A text without variables (empty, or
  only blank and comment lines) gives an empty map.
- `Parse` reads the lines in order and stops at the first problem. Then it returns a **nil** map
  and an error.
- The error message starts with `line N: `, where `N` is the number of the line with the problem.
  It also contains the line or the key, formatted with `%q`:

  | Problem | Example line | The message contains |
  |---|---|---|
  | No `=` in the line | `DB_HOST localhost` | the line: `"DB_HOST localhost"` |
  | The key is not valid (even when it is empty) | `db-host=x`, `=x` | the key: `"db-host"`, `""` |
  | A quoted value without a closing quote | `DB_PASSWORD="pa$$w0rd` | the key: `"DB_PASSWORD"` |
  | Text after the closing quote that is not a blank or a comment | `A="x"y` | the key: `"A"` |

- "The line" is the line without its `\r` and without the blanks at its start and at its end. "The
  key" is the key without the blanks around it. For a problem with a quote the message names the
  key, not the value, because the value may be a password.
- The rest of the wording is yours, for example `line 4: no "=" in "DB_HOST localhost"`.

## Examples

```text
Parse("# upload service\nexport PORT=8080\nLOG_LEVEL = info # quiet\n\nDB_PASSWORD=pa$$w0rd\n")
  → {"PORT": "8080", "LOG_LEVEL": "info", "DB_PASSWORD": "pa$$w0rd"}, nil

Parse("COLOR=#ff0000\nA=b#c\nKEY= #c\nURL=http://x/?a=1&b=2\n")
  → {"COLOR": "#ff0000", "A": "b#c", "KEY": "", "URL": "http://x/?a=1&b=2"}, nil

Parse(`GREETING="Hello # world"` + "\n" + `PUSH_TEXT='A "great tit" is here'` + "\n" + `DATA_DIR=${HOME}/photos`)
  → {"GREETING": "Hello # world", "PUSH_TEXT": `A "great tit" is here`, "DATA_DIR": "${HOME}/photos"}, nil

Parse("A=1\r\nA=2\r\n")  → {"A": "2"}, nil
Parse("")                → {}, nil   (an empty map, not nil)
```

```text
Parse("PORT=8080\n\n# database\nDB_HOST localhost\n")
  → nil, error `line 4: no "=" in "DB_HOST localhost"`

Parse("export db-host=x\n")             → nil, error `line 1: invalid key "db-host"`
Parse("A=1\nDB_PASSWORD=\"pa$$w0rd\n")  → nil, error `line 2: key "DB_PASSWORD": unterminated quote`
Parse("A='x' y\n")                      → nil, error `line 1: key "A": text after the closing quote`
Parse("CERT=\"-----BEGIN\nabc\n-----END\"\n")
  → nil, error `line 1: key "CERT": unterminated quote`
```

`testdata/prod.env` is the `.env` file of a production service, with Windows line endings.
`TestParseIncident` checks every variable in it. `TestParseIncidentBrokenLine` inserts a line
without `=` as line 13 and checks the error.

## Run the tests

    go test ./exercises/05-envfile/
    go test -run TestValidKey -v ./exercises/05-envfile/                          # step 1: ValidKey
    go test -run TestParseLines -v ./exercises/05-envfile/                        # step 2: lines, export, =
    go test -run 'TestParseComments|TestParseQuotes' -v ./exercises/05-envfile/   # step 3: comments and quotes
    go test -run TestParseErrors -v ./exercises/05-envfile/                       # step 4: errors
    go test -run TestParseIncident -v ./exercises/05-envfile/                     # step 5: the real file
    go test -run TestParseQuotesNoExpansion -v ./exercises/05-envfile/            # part of step 3: only `$`

A stub that panics stops the whole test binary, so while you work, run only the tests for the
step you are on. The 5 step filters do not overlap. Steps 2 to 5 call `Parse`, and each step uses
the rules of the steps before it. A plain `go test` also runs the example (`ExampleParse`). While
a stub still panics, a plain run crashes at the first test that calls it.

## Docs you can use

- [`strings`](https://pkg.go.dev/strings): [`Split`](https://pkg.go.dev/strings#Split),
  [`Cut`](https://pkg.go.dev/strings#Cut), [`CutPrefix`](https://pkg.go.dev/strings#CutPrefix),
  [`IndexByte`](https://pkg.go.dev/strings#IndexByte), [`Trim`](https://pkg.go.dev/strings#Trim),
  [`TrimLeft`](https://pkg.go.dev/strings#TrimLeft) and
  [`TrimSuffix`](https://pkg.go.dev/strings#TrimSuffix)
- [Strings, bytes, runes and characters in Go](https://go.dev/blog/strings) (Go blog): what
  `s[i]` gives you
- [The Go spec: Index expressions](https://go.dev/ref/spec#Index_expressions) and
  [Slice expressions](https://go.dev/ref/spec#Slice_expressions), for `s[i]` and `s[i:j]` on a
  string
- [Go maps in action](https://go.dev/blog/maps) (Go blog)
- [`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf) and the [`%q` verb](https://pkg.go.dev/fmt#hdr-Printing)
- [Effective Go: Errors](https://go.dev/doc/effective_go#errors)
- [Testable Examples in Go](https://go.dev/blog/examples) (Go blog), on how `ExampleParse` checks
  its output
- [Bash manual: Special Parameters](https://www.gnu.org/software/bash/manual/html_node/Special-Parameters.html),
  where `$$` comes from
- [Docker Compose: env file syntax](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/#env-file-syntax),
  1 of the readers whose rules these are based on
- [The Twelve-Factor App: Config](https://12factor.net/config)

## Stretch goals

- Expand `${VAR}` in unquoted and double-quoted values from keys defined on earlier lines, and
  keep single-quoted values literal, as Docker Compose does. Decide what an unknown `${VAR}` gives.
- Accept the escapes `\n`, `\"` and `\\` inside double quotes, and quoted values that span lines,
  such as a PEM certificate.
- Report every problem instead of only the first, with `errors.Join` (08-deploynote), so that a
  CI check lists all broken lines at once.
- Turn it into a command: `envcheck prod.env` prints the keys in sorted order, without their
  values, and exits with 1 on the first error.
- The error for a line without `=` prints the whole line. If somebody pastes a bare password on
  its own line, the error prints it. Change that message so that it never shows the text.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 05 hint 1
    go run ./cmd/peek 05 hint 2
    go run ./cmd/peek 05 hint 3
    go run ./cmd/peek 05 explanation
    go run ./cmd/peek 05 walkthrough
    go run ./cmd/peek 05 solution
