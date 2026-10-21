# 16 · The password that would not print

**Tier 4 · about 1.5 h** · Go: interfaces and implicit satisfaction, the interfaces that `fmt`, `encoding/json` and `log/slog` look for, method sets of values and pointers, `net/url` · Problem solving: list every path by which a value becomes text; test each one

At 02:00, in the middle of the load test for the HUSH 5 launch, somebody added 1 debug line:
`log.Printf("config: %+v", cfg)`. The commit message says only `wip`, and nobody asks who wrote
it. The config struct holds the database password, and `%+v` prints every field. The line went to
the log store, to the on-call channel and, through a screenshot, into a ticket that Marisol Quispe
Mamani, the support lead, forwarded to the whole team with the subject "is this bad?". It was
worse: HUSH, the on-call AI that Vesper Labs sells, reads every log line the lab writes, and its
morning report thanked the team "for the extra context". Bex Moreau, the security engineer,
rotated the password for the third time this year. Nobody wants to ban `%+v`, so Noor Haddad, the
principal SRE, wants a type that refuses to show its value, whatever the code around it does:
print it, log it or encode it as JSON. In this exercise you write that type, and a function that
hides the password in a database URL.

## Task

Implement the stubs in `redact.go`, and add the methods that `Secret` needs:

```go
const Redacted = "[REDACTED]"     // given

type Secret string
func (s Secret) Reveal() string   // the real value, for the 1 place that needs it
// You add the methods that make fmt, encoding/json and log/slog show Redacted.

func RedactURL(raw string) string // hides the password and the secret query values in raw
```

Which methods? That is the first half of the work. The Rules below list every printer and encoder
that must show `[REDACTED]`. The docs of `fmt`, `encoding/json` and `log/slog` say which method
each of them looks for.

Suggested order: `fmt` first, then `encoding/json` and `Reveal`, then `log/slog`, then
`RedactURL`.

## Rules

### Secret with fmt

- For a `Secret` and for a non-nil `*Secret`:
  - `%v`, `%+v`, `%s` and `%#v` give exactly `[REDACTED]`.
  - `%q` gives `"[REDACTED]"`, with the quotes.
  - `%x` gives `5b52454441435445445d` and `%X` gives `5B52454441435445445D`: the hex of the 10
    bytes of `[REDACTED]`.
  - `fmt.Print`, `fmt.Println` and their `Sprint` and `Fprint` variants show `[REDACTED]`.
    `fmt.Sprintln(s)` is `"[REDACTED]\n"`, as usual for `Println`.
- This holds for every value, also for the empty `Secret("")`. The output does not tell whether a
  secret is set.
- `fmt` hides a `Secret` inside other values too. With `%v`, `%+v`, `%#v` and the Print
  functions, the output contains `[REDACTED]` and never the value, for:
  - an exported struct field of type `Secret`, in a struct value and in a pointer to a struct;
  - an exported struct field of type `*Secret`;
  - a field of a struct that is itself a field (nested structs);
  - an element of a `[]Secret` and a value of a `map[string]Secret`.
- `log.Printf` and `fmt.Errorf` format with the same rules, so they hide the value too. The tests
  check the line from the story: `log.Printf("config: %+v", cfg)`.
- For values inside other values, the tests check only that `[REDACTED]` is in the output and the
  value is not. The rest of the output is up to `fmt`.

### Secret with encoding/json

- `json.Marshal` of a `Secret` or a non-nil `*Secret` gives the JSON string `"[REDACTED]"`, also
  for `Secret("")`. The same holds for a struct field of type `Secret` or `*Secret`, an element of
  a slice and a value of a map: `{"host":"db-1.vesper-labs.example","password":"[REDACTED]"}`.
- `json.Unmarshal` of a JSON string into a `Secret`, alone or as a struct field, stores the
  string: `Reveal` returns it.
- `json.Unmarshal` of a JSON value that is not a string (`42`, `true`, an object, an array) into a
  `Secret` returns an error.
- So a round trip loses the value, on purpose: marshal and unmarshal again, and the `Secret` holds
  `[REDACTED]`. The tests do not check this.

### Reveal

- `Reveal` returns the value unchanged, for a `Secret` and a `*Secret`. For `Secret("")` it
  returns `""`.

### Secret with log/slog

- `slog.NewTextHandler` and `slog.NewJSONHandler` show `[REDACTED]` for a `Secret`:
  - as an attribute value: `logger.Info("db connect", "password", s)`, also for `&s`;
  - in an attribute added with `Logger.With`;
  - in an attribute inside `slog.Group`;
  - in an exported field of a struct that is an attribute value:
    `logger.Info("config loaded", "db", cfg)`.
- slog does not look inside a struct. The `TextHandler` prints a struct with `fmt` (`%+v`), and
  the `JSONHandler` encodes it with `encoding/json`. So the struct case follows from the `fmt` and
  JSON rules above.
- For a `Secret` and a `*Secret`, `slog.AnyValue(s).Resolve()` has the kind `slog.KindString` and
  the string `[REDACTED]`.
- A handler receives each attribute as the caller passed it, with the `Secret` inside. The
  `slog.Handler` docs ask every handler to call `Value.Resolve` on a value before it uses it. The
  built-in handlers do so before they call `ReplaceAttr`, so a `ReplaceAttr` function receives
  the string value `[REDACTED]`, not the `Secret`.
- This matters for a handler that resolves values, as the docs ask, but then encodes them in its
  own way, such as a log vendor's handler that reads the underlying string with reflection. After
  `Resolve`, no `Secret` is left to read. A handler that skips `Resolve` still sees the `Secret`.
  The tests do not check vendor handlers.

### Known limits of Secret

These hold for the simplest correct solution. The tests do not check them.

- An unexported struct field of type `Secret` prints its value: `fmt` reads unexported fields
  with reflection but cannot call their methods.
- A verb that is not for strings, such as `%d`, prints `%!d(redact.Secret=hunter2)`, with the
  value. A stretch goal closes that hole.
- `string(s)` is a conversion, not a method call. It gives the value, like `Reveal`.

### RedactURL

- If `url.Parse` rejects `raw`, or `url.ParseQuery` rejects the raw query of the parsed URL, the
  result is `[invalid URL]`. Never return `raw` or the error text: the error from `url.Parse`
  quotes the whole input, password included.
- `url.Parse` accepts the empty string and a relative URL such as `/healthz?verbose=1`. The rules
  below apply to them like to any other URL, so `""` comes back as `""`.
- A URL has a secret if its user info has a password or its query has a secret parameter:
  - Its user info has a password: a `:` after the user name, also when the password is empty
    (`postgres://orders_app:@db-1/orders`) or the user name is empty
    (`redis://:hunter2@cache-1:6379/0`). This is what `URL.User.Password()` reports.
  - Its query has a parameter named `password`, `token`, `secret`, `key` or `sig`, in any letter
    case (`Token`, `SIG`). The whole name must match: `monkey`, `keys` and `tokens` are not
    secret.
- A URL without a secret comes back unchanged, byte for byte. This is not `String()` of the
  parsed URL: `String()` writes the scheme in lower case, for example.
- For a URL with a secret:
  - The password becomes `xxxxx`. The user name stays.
  - If the query has a secret parameter, each value of each secret parameter becomes `xxxxx`. A
    parameter that appears twice gets 2 values `xxxxx`. An empty value becomes `xxxxx` too.
  - In that case the raw query becomes `url.Values.Encode` of the result. `Encode` sorts the keys
    by byte value, so `Token` comes before `team`. It keeps the values of 1 key in their order. It
    writes its own escapes, so `%20` becomes `+`.
  - If only the password is secret, the query stays exactly as it was.
  - The result is `String()` of the changed URL.
- A keyword DSN as libpq writes it (`host=db-1 user=orders_app password=hunter2`) is not a URL.
  `url.Parse` accepts it as a path, it has no secret by the rules above, and it comes back
  unchanged. The tests do not check this; a stretch goal handles it.

## Examples

```text
s := Secret("hunter2")

fmt.Sprintf("%v", s)       → [REDACTED]
fmt.Sprintf("%#v", &s)     → [REDACTED]
fmt.Sprintf("%q", s)       → "[REDACTED]"
fmt.Sprintf("%x", s)       → 5b52454441435445445d
fmt.Sprintln("pw:", s)     → pw: [REDACTED]\n
fmt.Sprintf("%+v", dbConfig{Host: "db-1.vesper-labs.example", User: "orders_app", Password: s})
                           → {Host:db-1.vesper-labs.example User:orders_app Password:[REDACTED]}
s.Reveal()                 → hunter2

json.Marshal(s)            → "[REDACTED]"
json.Unmarshal([]byte(`"hunter2"`), &s); s.Reveal() → hunter2
```

With `ReplaceAttr` removing the time:

```text
logger.Info("db connect", "password", s)

TextHandler: level=INFO msg="db connect" password=[REDACTED]
JSONHandler: {"level":"INFO","msg":"db connect","password":"[REDACTED]"}

logger.Info("config loaded", "db", dbConfig{...})

TextHandler: level=INFO msg="config loaded" db="{Host:db-1.vesper-labs.example User:orders_app Password:[REDACTED]}"
JSONHandler: {"level":"INFO","msg":"config loaded","db":{"Host":"db-1.vesper-labs.example","User":"orders_app","Password":"[REDACTED]"}}
```

| `raw` | `RedactURL(raw)` |
|---|---|
| `postgres://orders_app:hunter2@db-1.vesper-labs.example:5432/orders?sslmode=require&connect_timeout=5` | `postgres://orders_app:xxxxx@db-1.vesper-labs.example:5432/orders?sslmode=require&connect_timeout=5` |
| `postgres://orders_app:p%40ss@db-1.vesper-labs.example/orders` | `postgres://orders_app:xxxxx@db-1.vesper-labs.example/orders` |
| `https://hooks.vesper-labs.example/deploy?team=sre&Token=abc123` | `https://hooks.vesper-labs.example/deploy?Token=xxxxx&team=sre` |
| `https://cdn.vesper-labs.example/checkpoints/hush-4.2.bin?sig=a1b2&expires=1764000000&sig=c3d4` | `https://cdn.vesper-labs.example/checkpoints/hush-4.2.bin?expires=1764000000&sig=xxxxx&sig=xxxxx` |
| `https://search.vesper-labs.example/find?token=abc&q=calm%20score` | `https://search.vesper-labs.example/find?q=calm+score&token=xxxxx` |
| `HTTPS://Status.Vesper-Labs.example/api/v2/summary.json?q=a%20b&page=2` | unchanged |
| `postgres://orders_app@db-1.vesper-labs.example:5432/orders` | unchanged (no password) |
| `/healthz?verbose=1` | unchanged (a relative URL without a secret) |
| `postgres://orders_app:hun ter@db-1.vesper-labs.example/orders` | `[invalid URL]` (`url.Parse` rejects the space) |
| `https://hooks.vesper-labs.example/deploy?token=abc123%zz` | `[invalid URL]` (`url.ParseQuery` rejects `%zz`) |
| `https://hooks.vesper-labs.example/deploy?token=abc123;team=sre` | `[invalid URL]` (`url.ParseQuery` rejects `;`) |

## Run the tests

    go test ./exercises/16-redact/
    go test -run 'TestSecretFmt|TestSecretNested' -v ./exercises/16-redact/    # step 1: fmt
    go test -run 'TestSecretJSON|TestSecretReveal' -v ./exercises/16-redact/   # step 2: encoding/json and Reveal
    go test -run TestSecretSlog -v ./exercises/16-redact/                      # step 3: log/slog
    go test -run TestRedactURL -v ./exercises/16-redact/                       # step 4: RedactURL

The stub `Secret` has no methods yet, so the `fmt`, JSON and slog tests fail on their checks: the
output shows the secret where `[REDACTED]` should be. The slog handlers print through `fmt` and
`encoding/json`, so after steps 1 and 2, `TestSecretSlogText` and `TestSecretSlogJSON` already
pass. `TestSecretSlogResolve` still fails until step 3.

`Reveal` and `RedactURL` panic until you write them. A panic stops the whole test binary, so run
only the step you are on. The 4 filters do not overlap. No filter selects the 2 examples
(`ExampleSecret`, `ExampleRedactURL`), so only a plain `go test` runs them. It crashes until all
4 steps work.

## Docs you can use

- [`fmt`, section Printing](https://pkg.go.dev/fmt#hdr-Printing): read the part about operands
  that implement certain interfaces, and the order in which `fmt` checks them
- [`fmt.Stringer`](https://pkg.go.dev/fmt#Stringer) and [`fmt.GoStringer`](https://pkg.go.dev/fmt#GoStringer)
- [`json.Marshal`](https://pkg.go.dev/encoding/json#Marshal) and [`json.Unmarshal`](https://pkg.go.dev/encoding/json#Unmarshal):
  the first paragraphs say which interfaces they look for
- [`encoding.TextMarshaler`](https://pkg.go.dev/encoding#TextMarshaler) and [`json.Marshaler`](https://pkg.go.dev/encoding/json#Marshaler)
- [`log/slog`, section Customizing a type's logging behavior](https://pkg.go.dev/log/slog#hdr-Customizing_a_type_s_logging_behavior),
  [`slog.LogValuer`](https://pkg.go.dev/log/slog#LogValuer), [`Value.Resolve`](https://pkg.go.dev/log/slog#Value.Resolve)
  and [`slog.Handler`](https://pkg.go.dev/log/slog#Handler)
- [Structured Logging with slog](https://go.dev/blog/slog) (Go blog)
- [Effective Go: Interfaces](https://go.dev/doc/effective_go#interfaces)
- [The Go spec: Method sets](https://go.dev/ref/spec#Method_sets) and the FAQ
  [Why do T and *T have different method sets?](https://go.dev/doc/faq#different_method_sets)
- [`net/url`](https://pkg.go.dev/net/url): `Parse`, `URL`, `Userinfo`, `UserPassword`,
  `ParseQuery`, `Values.Encode`, `URL.String`
- [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html),
  the section on data to exclude

## Stretch goals

- Close the `%d` hole: implement [`fmt.Formatter`](https://pkg.go.dev/fmt#Formatter), so that
  every verb (`%d`, `%10s`, `%-12v`) prints `[REDACTED]` in some form. Keep all tests green.
  `go vet` already reports `fmt.Printf("%d", s)` as a wrong type; `Format` also covers the
  format strings that vet cannot see, such as one built at run time.
- Implement `encoding.TextUnmarshaler`, so that `flag.TextVar` can read a `Secret` from a
  command-line flag. What does `-help` show as the default value?
- Find more secret parameters: `access_token`, `api_key`, `client_secret`, any name that ends in
  `_token`, and an OAuth token in the fragment (`#access_token=...`).
- Hide the password in a keyword DSN: `host=db-1 user=orders_app password=hunter2` becomes
  `host=db-1 user=orders_app password=xxxxx`. Watch out for quoted values (`password='a b'`).
- Write a canary test for a whole service: start it with a fake secret such as
  `canary-7f3e1b`, run every code path that logs, and fail if `canary-7f3e1b` appears anywhere in
  the output. Bex now adds a canary secret to every load test.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 16 hint 1
    go run ./cmd/peek 16 hint 2
    go run ./cmd/peek 16 hint 3
    go run ./cmd/peek 16 explanation
    go run ./cmd/peek 16 walkthrough
    go run ./cmd/peek 16 solution
