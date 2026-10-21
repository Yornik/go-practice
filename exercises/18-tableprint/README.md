# 18 · After the second curly brace

**Tier 4 · about 2 h** · Go: `reflect` (`TypeOf` and `ValueOf`, `Kind`, `Elem`, `NumField`, `StructField.Tag`, `IsExported`, `IsNil`), struct tags, a type assertion to `fmt.Stringer`, `utf8.RuneCountInString`, `io.Writer` · Problem solving: work out the columns from the type before you look at a value; make an empty list still print its header

In the Launch Week war room on floor 61, with the light show flickering on the glass, every tool
prints the state of the fleet its own way, and most of them print Go structs with `%+v`:
`{Name:chkpt-store-2 Status:{Phase:Degraded Since:2026-11-27 21:12:09 +0100 CET} Disk:0xc000014098}`.
The runbook explains how to read it: "the phase is after the second curly brace". The disk usage
is a memory address, because `%+v` prints a pointer inside a struct as an address. The only clean
table on the wall is the console of HUSH, the on-call AI that Vesper Labs sells, and this week
nobody in the room trusts what it prints. Noor Haddad, the principal SRE, counts the braces of 12
such lines aloud. Then she asks for what `kubectl get pods` gives her, for every tool: 1 header
row, aligned columns, `<none>` where a value is missing, and rows that a tired person can scan and
a screen reader can read cell by cell. In this exercise you write the printer that turns any slice
of structs into that table.

## Task

Implement the stubs in `tableprint.go`. The file already declares the 2 errors.

```go
var ErrNotStructs = errors.New("tableprint: rows must be a slice of structs") // given
var ErrNilRow = errors.New("tableprint: nil row")                            // given

func Headers(rows any) ([]string, error) // the column headers, from the type of rows
func Render(w io.Writer, rows any) error // the table: a header row, then 1 row per element
```

Suggested order: `Headers` first, then `Render` for string and int fields, then the other cell
rules, then pointers and errors. These are the 4 test steps.

## Rules

### What rows may be

- `rows` must be a **slice** whose element type is a **struct** (`[]Pod`) or a **pointer to a
  struct** (`[]*Pod`). A nil slice of such a type is fine: it has 0 elements.
- The element type decides, not the values: a `[]any` is rejected, even when it holds structs.
- Anything else is an error: `nil`, a struct, a pointer to a struct, an array, a map, `[]int`,
  `[]*int`, `[]string`, `[]any`, and a pointer to a slice of structs (`&rows`).
- That error wraps `ErrNotStructs` (`errors.Is` works) and contains the type of `rows` as `%T`
  prints it, for example `[]int`, `<nil>` or `*[]main.Pod`. A message such as
  `tableprint: rows must be a slice of structs, got []int` works; the rest of the wording is yours.
- With that error `Headers` returns nil headers, and `Render` does not call `w.Write`.

### Columns

- The columns are the **exported** fields of the struct, in the order of their declaration.
  Unexported fields are never columns, not even with a `table` tag.
- The header of a column comes from the struct tag with the key `table`:

  | Field | Header |
  |---|---|
  | `Name string` | `NAME`: no tag gives the field name in upper case (`strings.ToUpper`) |
  | `LastSeen time.Duration` | `LASTSEEN` |
  | ``LastSeen time.Duration `table:"LAST SEEN"` `` | `LAST SEEN`: the tag value as it is written |
  | ``Restarts int `table:"Restarts"` `` | `Restarts`: also not upper-cased |
  | ``Token string `table:"-"` `` | no column |
  | ``Zone string `json:"zone"` `` | `ZONE`: only the key `table` counts |
  | ``Region string `json:"region" table:"AZ"` `` | `AZ` |
  | ``Node string `table:""` `` | `NODE`: an empty tag value counts as no tag |
  | `token string` | no column: unexported |

- An **embedded** field is 1 ordinary column, named after its type: `Meta` and `*Meta` both give
  the header `META`. The fields of `Meta` do not become columns of their own. An embedded field
  of an unexported type (`meta`) is unexported, so it is not a column.
- `Headers` works from the type of `rows` alone. It never looks at the elements: an empty slice, a
  nil slice and a slice that holds a nil pointer all give the headers and no error.
- Every struct in the tests has at least 1 column. A struct without columns is not tested.

### Cells

For each field value, the **first** rule that matches gives the text of the cell:

1. A nil pointer gives `<none>`.
2. A value that implements `fmt.Stringer` gives the result of its `String` method. Only the
   method set of the field's own type counts. If `String` has a pointer receiver,
   `func (g *Gauge) String() string`, then a field of type `*Gauge` uses it and a field of type
   `Gauge` does not. `fmt.Sprint` makes the same choice.
3. A non-nil pointer gives the cell of the value it points to: rules 1 to 5 again. So a pointer to
   a struct without `String` gives `{harbour-1 3}`, not `&{harbour-1 3}`, and a `**int` that
   points to a nil `*int` gives `<none>`.
4. These kinds, also as named types such as `type Phase string`:
   - a string as it is, also an empty one;
   - a bool as `true` or `false`;
   - every integer kind (`int`, `int8` to `int64`, `uint`, `uint8` to `uint64`, `uintptr`) in
     base 10;
   - a `float32` or `float64` as `strconv.FormatFloat(f, 'f', -1, bits)`, where `bits` is 32 for a
     `float32` and 64 for a `float64`. That gives no exponent and the fewest digits that read back
     as the same value: `1e21` gives `1000000000000000000000`, `1e-7` gives `0.0000001`, and
     `float32(0.1)` gives `0.1`.
5. Anything else gives `fmt.Sprint` of the value: a struct, a slice, an array, a map, a complex
   number.

| Field value | Rule | Cell |
|---|---|---|
| `(*int)(nil)`, `(*url.URL)(nil)` | 1 | `<none>` |
| `90 * time.Second` (a `time.Duration`) | 2 | `1m30s` |
| `time.Date(2026, 11, 27, 21, 40, 0, 0, time.FixedZone("CET", 3600))` | 2 | `2026-11-27 21:40:00 +0100 CET` |
| a `*url.URL` (`String` has a pointer receiver) | 2 | `https://canary-h3-01.vesper-labs.example/healthz` |
| a `*int` that points to `87` | 3, then 4 | `87` |
| `Phase("Degraded")` | 4 | `Degraded` |
| `int8(-128)`, `uint64(18446744073709551615)` | 4 | `-128`, `18446744073709551615` |
| `0.5`, `2.0` | 4 | `0.5`, `2` |
| `Location{"harbour-1", 3}` (no `String`) | 5 | `{harbour-1 3}` |
| `[]string{"checkout", "orders"}`, `[]string(nil)` | 5 | `[checkout orders]`, `[]` |
| `map[string]int{"orders": 2, "checkout": 1}` | 5 | `map[checkout:1 orders:2]` |

### Layout

- The first row holds the headers. Then comes 1 row per element of `rows`, in slice order. An
  empty or nil slice gives only the header row.
- A column is as wide as its widest cell, the header included. The width counts **runes**
  (`utf8.RuneCountInString`), not bytes: `zoë` is 3 wide (4 bytes), `São Paulo` is 9 wide (10
  bytes), and the moon emoji `🌙` is 1 wide (4 bytes). A terminal shows that moon 2 columns
  wide; the width counts it as 1 anyway.
- In each row, write every cell except the last, then spaces up to the width of its column, then
  2 more spaces. Write the last cell as it is, without padding, and end the row with `\n`. So a
  row has no trailing spaces, unless its last cell is empty or ends with a space.
- You may use `text/tabwriter`. `tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)` gives exactly this
  layout when you separate the cells with `\t` and end each row with `\n`. The tests have no tab
  or newline inside a cell.

### Errors

- For `rows` that is not a slice of structs, `Render` returns the error that `Headers` returns.
- A slice of pointers may hold a nil pointer. Then `Render` returns an error that wraps
  `ErrNilRow` (`errors.Is` works). The error contains the index of the first nil element as
  `rows[i]`, for example `tableprint: nil row: rows[2]`.
- In both cases `Render` does **not call** `w.Write`: no header row, no rows before the nil one,
  and no empty write either.
- If `w` returns an error, `Render` returns it so that `errors.Is(err, thatError)` is true.
  Returning it as is or wrapping it with `%w` are both fine. On success `Render` returns nil.
- On success, `Render` may call `w.Write` 1 time or more.

## Examples

```go
type Node struct {
	Name     string
	Driver   string        `table:"DRV"`
	GPUUtil  *int          `table:"GPU UTIL"`
	LastSeen time.Duration `table:"LAST SEEN"`
	apiToken string
}

gpuUtil := 87
rows := []Node{
	{"canary-h3-01", "2.10.0", &gpuUtil, 12 * time.Second, "tok-8f3a91"},
	{"vl-gpu-0412", "2.9.3", nil, 41 * time.Minute, "tok-c27d40"},
}
```

```text
Headers(rows)          → ["NAME" "DRV" "GPU UTIL" "LAST SEEN"], nil
Headers([]Node{})      → ["NAME" "DRV" "GPU UTIL" "LAST SEEN"], nil
Headers([]*Node{nil})  → ["NAME" "DRV" "GPU UTIL" "LAST SEEN"], nil

Render(os.Stdout, rows) prints:
NAME          DRV     GPU UTIL  LAST SEEN
canary-h3-01  2.10.0  87        12s
vl-gpu-0412   2.9.3   <none>    41m0s

Render(os.Stdout, []Node{}) prints:
NAME  DRV  GPU UTIL  LAST SEEN
```

```text
Headers([]int{1, 2})               → nil, error "tableprint: rows must be a slice of structs, got []int"
Render(w, rows[0])                 → error wraps ErrNotStructs, contains "main.Node"; no Write call
Render(w, &rows)                   → error wraps ErrNotStructs, contains "*[]main.Node"; no Write call
Render(w, []*Node{&rows[0], nil})  → error "tableprint: nil row: rows[1]"; no Write call
```

## Run the tests

    go test ./exercises/18-tableprint/
    go test -run TestHeaders -v ./exercises/18-tableprint/                            # step 1: Headers
    go test -run 'TestRenderBasic|TestRenderWidth' -v ./exercises/18-tableprint/      # step 2: the layout
    go test -run TestRenderCells -v ./exercises/18-tableprint/                        # step 3: cell rules 2, 4 and 5
    go test -run 'TestRenderPointers|TestRenderErrors' -v ./exercises/18-tableprint/  # step 4: pointers and errors
    go test -run TestHeadersErrors -v ./exercises/18-tableprint/                      # one test

Run one step at a time; the 4 filters do not overlap. Step 2 uses only string and int fields.
Every test recovers a panic and reports it as a failure with the stack: the "TODO: implement" of
the stub, or a `reflect` call on the wrong kind of value. The other tests of the step still run.
`ExampleRender` runs the example above, only in a plain `go test`. An example cannot recover a
panic, so a plain run crashes until `Render` works.

## Docs you can use

- [The Laws of Reflection](https://go.dev/blog/laws-of-reflection) (Go blog): types, values and
  kinds, and why a value from `ValueOf` is not always settable
- [`reflect`](https://pkg.go.dev/reflect), in particular [`TypeOf`](https://pkg.go.dev/reflect#TypeOf),
  [`Type`](https://pkg.go.dev/reflect#Type) (`Kind`, `Elem`, `NumField`, `Field`),
  [`StructField`](https://pkg.go.dev/reflect#StructField) (`IsExported`, `Tag`),
  [`StructTag.Get`](https://pkg.go.dev/reflect#StructTag.Get), and
  [`Value`](https://pkg.go.dev/reflect#Value) (`Len`, `Index`, `Field`, `Elem`, `IsNil`, `Interface`)
- The Go spec: [Struct types](https://go.dev/ref/spec#Struct_types) (embedded fields and tags),
  [Method sets](https://go.dev/ref/spec#Method_sets), [Type assertions](https://go.dev/ref/spec#Type_assertions)
- [Effective Go: Interface conversions and type assertions](https://go.dev/doc/effective_go#interface_conversions)
- [`fmt.Stringer`](https://pkg.go.dev/fmt#Stringer) and the
  [printing rules of `fmt`](https://pkg.go.dev/fmt#hdr-Printing) for `%v`
- [`strconv`](https://pkg.go.dev/strconv): `FormatInt`, `FormatUint`, `FormatFloat`, `FormatBool`
- [`utf8.RuneCountInString`](https://pkg.go.dev/unicode/utf8#RuneCountInString) and
  [Strings, bytes, runes and characters in Go](https://go.dev/blog/strings) (Go blog)
- [`io.Writer`](https://pkg.go.dev/io#Writer), [`strings.Builder`](https://pkg.go.dev/strings#Builder),
  [`text/tabwriter`](https://pkg.go.dev/text/tabwriter)
- [`kubectl get`](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_get/), the output
  this exercise copies

## Stretch goals

- Add a wide mode, as `kubectl get -o wide`: a tag option `table:"NODE,wide"` shows the column
  only when the caller asks for it.
- Add `RenderSorted(w, rows, column string)`, as `kubectl get --sort-by`. Sort by the cell text,
  or by the value when the column is a number.
- Right-align the columns of integer and float fields, so that the digits line up.
- Count display width instead of runes, so that `🌙` and CJK characters take 2 columns. The
  standard library has no width table; `golang.org/x/text/width` has one.
- Make it a tool: read JSON lines from stdin, 1 object per line, and print them as a table with
  the keys of the first line as the columns.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 18 hint 1
    go run ./cmd/peek 18 hint 2
    go run ./cmd/peek 18 hint 3
    go run ./cmd/peek 18 explanation
    go run ./cmd/peek 18 walkthrough
    go run ./cmd/peek 18 solution
