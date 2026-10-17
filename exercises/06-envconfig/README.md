# 06 · Typed config from the environment

**Tier 2 · about 2.5 h** · Go: `reflect` (Type vs Value, Kind, `CanSet`, `reflect.New`, `Implements`), struct tags, `encoding.TextUnmarshaler`, `errors.Join` with `errors.Is`/`errors.As`, function injection · Problem solving: a recursive walk over a type; dispatch from the most specific rule to the most general

A twelve-factor service reads its config from environment variables; in Kubernetes they come from
the Deployment's `env`, a ConfigMap or a Secret. The usual `main` has 30 lines of
`strconv.Atoi(os.Getenv("PORT"))` and calls `log.Fatal` on the first error. A deploy with 3
mistakes then crash-loops on the first one; you fix it, wait for the rollout, and it crash-loops
on the second. And one day the log says `strconv.Atoi: parsing "hunter2"`, and the database
password sits in your log pipeline for 30 days. You write the loader that ends this: declare a
struct with tags, call `Load` once, get every mistake in one error, never see a secret.

## Task

Implement the stubs in `envconfig.go`:

```go
func UpperSnake(name string) string                              // "HTTPPort" → "HTTP_PORT"
func Load(dst any, lookup func(key string) (string, bool)) error // fill *dst, report all problems

type FieldError struct {
	Field  string // Go path of the field: "DB.Port"
	Key    string // variable name: "DB_PORT"
	Value  string // raw text that failed to parse; "" if there was none
	Secret bool   // the field has secret:"true"
	Err    error  // the cause
}
func (e *FieldError) Error() string // one line, never shows a secret
func (e *FieldError) Unwrap() error // returns Err

var ErrInvalidTarget, ErrRequired, ErrUnsupportedType error // given
```

Work in the order of the steps in "Run the tests".

## Rules

### Variable names

- `UpperSnake` puts `_` before an upper-case letter that is not the first character when the
  character before it is lower case or a digit, **or** the character before it is upper case and
  the one after it is lower case. Then it upper-cases everything: `ReadTimeout` → `READ_TIMEOUT`,
  `HTTPPort` → `HTTP_PORT`, `UserID` → `USER_ID`, `S3Bucket` → `S3_BUCKET`, `MaxConns2` →
  `MAX_CONNS2`, `""` → `""`. Tests use ASCII names. Some names come out odd
  (`IPv6Addr` → `I_PV6_ADDR`); that is what the `env` tag is for.
- A field's name is its `env` tag, or `UpperSnake(field name)` without one. Its **key**, the
  variable Load looks up, is the prefix of the struct it is in plus that name. The root prefix
  is `""`.
- Unexported fields are ignored, even with tags.

| Tag | Meaning |
|---|---|
| `env:"NAME"` | The field's name (the prefix still applies). |
| `env:"-"` | Skip the field; for a nested struct, all its fields. Never checked, never changed. |
| `default:"text"` | Used when the variable has no value. Parsed like a value from the environment. |
| `required:"true"` | No value and no default: a `FieldError` with `Err` = `ErrRequired`. A default satisfies it. |
| `secret:"true"` | Sets `FieldError.Secret`: messages never show the value. |
| `prefix:"TEXT"` | On a nested struct field: the prefix for its fields. |

### Nested structs

- A struct-typed field is a nested config, **unless** its type parses itself (see Types:
  `netip.Addr` and `time.Time` are structs but single values). Load fills its fields; it does not
  look up a variable for the struct field itself.
- The prefix of a nested struct's fields is the outer prefix plus the `prefix` tag if there is
  one, **even an empty one**, else the nested struct field's name plus `_`.

| Root struct declares | Inner field | Key |
|---|---|---|
| `DB database` | `Port int` | `DB_PORT` |
| `DB database` | `` Host string `env:"HOSTNAME"` `` | `DB_HOSTNAME` |
| `` Replica database `env:"DB_RO"` `` | `Port int` | `DB_RO_PORT` |
| `` Cache cache `prefix:"REDIS_"` `` | `TTL time.Duration` | `REDIS_TTL` |
| `` HTTP httpConfig `prefix:""` `` | `Port int` | `PORT` |
| `Upstream struct{ TLS tlsFiles }` | `CertFile string` | `UPSTREAM_TLS_CERT_FILE` |
| ``Upstream struct{ MTLS tlsFiles `prefix:"CLIENT_"` }`` | `CertFile string` | `UPSTREAM_CLIENT_CERT_FILE` |

Pointers to nested structs and embedded structs are not tested.

### Values

- A variable that is not set **or is set to `""`** has no value. (A Helm template that renders
  `value: ""` must not count as configured.) Then the `default` tag is used; an empty default is
  no default.
- Still no value: `required:"true"` is an error, even if `dst` already holds a value for the
  field; otherwise the field keeps what `dst` holds (values set in code before `Load` survive, a
  nil pointer stays nil).
- A value, from the variable or the default, is parsed and replaces what `dst` holds.
- A nil `lookup` means `os.LookupEnv`.

### Types

| Field type | Parsed with | Examples |
|---|---|---|
| `T` where `*T` implements `encoding.TextUnmarshaler` | `UnmarshalText`, **whatever the kind of `T`** | `netip.Addr`, `time.Time` (RFC 3339), `slog.Level` (an int: `warn`, `ERROR`) |
| `time.Duration` | `time.ParseDuration`, not as an integer | `1m30s`, `250ms`; `30` is an error |
| kind string | as is | `string`, `type region string` |
| bool | `strconv.ParseBool` | `true`, `1`, `FALSE`; `yes` is an error |
| `int`, `int8` … `int64` | base 10, must fit the size | `-1`; `2.5` is an error |
| `uint`, `uint8` … `uint64` | base 10, must fit the size | `-1` is an error; `uint16`: `70000` is an error, not 4464 |
| `float32`, `float64` | `strconv.ParseFloat` | `0.25`; `0,25` is an error |
| `[]string` | split on `,`, items trimmed, empty items dropped | `" a, b,, ,c ,"` → `["a" "b" "c"]` |
| `*T`, `T` any type above | parse into a **new** `T`, store its pointer | `*int`, `*netip.Addr`: `LIMIT=0` → pointer to 0 |
| anything else | unsupported, see Errors | `map[string]string`, `[]int` |

- A `[]string` value replaces the slice in `dst`; it does not append.
- A pointer field with a value gets a new pointer. Load never writes through the pointer `dst`
  already holds: the caller's variable may be shared. With no value, the pointer in `dst` is kept.

### Errors

- If `dst` is not a non-nil pointer to a struct (`nil`, a struct value, a nil `*Config`, a
  `**Config`, a pointer to an int, a map), `Load` returns an error wrapping `ErrInvalidTarget`.
- Otherwise it tries **every** field and returns `errors.Join` of one `*FieldError` per bad
  field, in field order (a nested struct's fields at the position of the nested struct field),
  or `nil`.
- An unsupported type is an error **even when its variable is not set**: it is a bug in the code
  and must show up on the first run.
- When `Load` returns an error, do not use `dst`: which fields it filled is not specified.

| Problem | `Value` | `Err` |
|---|---|---|
| a value does not parse | the raw text, from the variable or the default | the error the parser returned, unchanged |
| required, no value | `""` | `ErrRequired` |
| unsupported type | `""` | an error wrapping `ErrUnsupportedType` |

- `Field` is the Go path from the root (`DB.Password`, `Upstream.TLS.CertFile`), `Key` the full
  variable name. `Value` holds the raw text for secret fields too, and `Unwrap` returns `Err`
  for every field: only the message is masked.
- `Error()` returns one line. For a secret it leaves out the message of `Err` too, because parsers
  quote their input: `parse "postgres://app:hunter2@db:5432x/orders": invalid port ...`.

| Case | `Error()` | Example |
|---|---|---|
| `Value` is `""` | `<Key>: <Err>` | `DB_URL: required variable is not set` |
| `Value` set, not secret | `<Key>=<Value with %q>: <Err>` | `PORT="80a": strconv.ParseInt: parsing "80a": invalid syntax` |
| `Value` set, `Secret` | `<Key>: invalid value (secret, not shown)` | `DB_URL: invalid value (secret, not shown)` |

## Examples

```go
type Config struct {
	Port        int           `default:"8080"`
	ReadTimeout time.Duration `default:"5s"`
	LogLevel    slog.Level    `default:"info"`
	Bind        netip.Addr    `env:"BIND_ADDR" default:"0.0.0.0"`
	DB          struct {
		URL      string `required:"true" secret:"true"`
		MaxConns int    `default:"10"`
	}
	Logger *slog.Logger `env:"-"` // set in code
}
var cfg Config
if err := envconfig.Load(&cfg, os.LookupEnv); err != nil {
	log.Fatalf("invalid configuration:\n%v", err)
}
// Reads PORT, READ_TIMEOUT, LOG_LEVEL, BIND_ADDR, DB_URL, DB_MAX_CONNS.
// With PORT=9090 and DB_URL=postgres://db/orders: Port 9090, ReadTimeout 5s, LogLevel INFO, ...
```

```text
env: PORT=80a  TIMEOUT=30  DEBUG=yes  DB_URL=""  BROKERS=kafka-0:9092
type Config struct {
	Port    int           `default:"8080"`
	Timeout time.Duration `default:"5s"`
	Workers int           `default:"many"`
	DB      struct {
		URL      string `required:"true"`
		Password string `required:"true" secret:"true"`
	}
	Debug   bool
	Brokers []string `required:"true"`
}
Load → an error that prints as 6 lines, in field order:
PORT="80a": strconv.ParseInt: parsing "80a": invalid syntax
TIMEOUT="30": time: missing unit in duration "30"
WORKERS="many": strconv.ParseInt: parsing "many": invalid syntax
DB_URL: required variable is not set
DB_PASSWORD: required variable is not set
DEBUG="yes": strconv.ParseBool: parsing "yes": invalid syntax
```

The text after `<Key>="<Value>": ` depends on the parser you call (`strconv.Atoi` writes
`strconv.Atoi: ...`). Only `ExampleLoad` checks one, from `time.ParseDuration`.

## Run the tests

    go test ./exercises/06-envconfig/
    go test -run TestUpperSnake -v ./exercises/06-envconfig/            # step 1
    go test -run TestFieldError -v ./exercises/06-envconfig/            # step 2
    go test -run 'TestTarget|TestBasic' -v ./exercises/06-envconfig/    # step 3: string and int
    go test -run TestTypes -v ./exercises/06-envconfig/                 # step 4: all types
    go test -run TestNested -v ./exercises/06-envconfig/                # step 5: nested structs
    go test -run 'TestErrors|ExampleLoad' -v ./exercises/06-envconfig/  # step 6: all errors

The tests recover panics, so a `reflect` panic ("call of reflect.Value.SetInt on string Value")
fails one test with its message. When a value is wrong, the failure lists every key your `Load`
looked up, in order.

## Docs you can use

- [The Laws of Reflection](https://go.dev/blog/laws-of-reflection) (Go blog)
- [`reflect`](https://pkg.go.dev/reflect): `ValueOf`, `Value.Elem`, `Value.Field`, `Value.CanSet`,
  `New`, `PointerTo`, `TypeFor`, `Type.Implements`, `Type.Bits`, `StructTag.Get`/`Lookup`
- [`encoding.TextUnmarshaler`](https://pkg.go.dev/encoding#TextUnmarshaler),
  [`strconv`](https://pkg.go.dev/strconv), [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration)
- [`errors.Join`](https://pkg.go.dev/errors#Join), [`errors.Is`](https://pkg.go.dev/errors#Is),
  [`errors.As`](https://pkg.go.dev/errors#As), [`os.LookupEnv`](https://pkg.go.dev/os#LookupEnv)
- The Go spec: [Struct types](https://go.dev/ref/spec#Struct_types) (tags),
  [Method sets](https://go.dev/ref/spec#Method_sets)
- [The Twelve-Factor App: Config](https://12factor.net/config)

## Stretch goals

- Embedded structs like `encoding/json`: the fields of `Common` in `struct{ Common; Port int }`
  at the outer level, without a prefix.
- Slices of any supported type (`[]netip.Addr`, `[]time.Duration`) and a `sep:";"` tag.
- `Usage(dst any) string`: a table of every key, type, default and required flag for `--help`.
- Keep secrets out of the struct too: `json.Marshal` or `%#v` of a `FieldError` still shows `Value`.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 06 hint 1
    go run ./cmd/peek 06 hint 2
    go run ./cmd/peek 06 hint 3
    go run ./cmd/peek 06 explanation
    go run ./cmd/peek 06 walkthrough
    go run ./cmd/peek 06 solution
