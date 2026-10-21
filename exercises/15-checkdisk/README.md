# 15 · A disk check that can fail

**Tier 3 · about 1.5 h** · Go: `flag.NewFlagSet` with `ContinueOnError`, `flag.Func` for a repeated flag, a testable `Run(args, stdin, stdout, stderr) int`, exit codes, `bufio.Scanner`, `fmt.Fprintf` · Problem solving: separate parsing, deciding and printing; define the result of every bad input

On a Sunday in November, at 06:40, the photo disk of `photo-store-3` filled up. Until noon, not 1
photo from the 40 000 Peckish bird feeders reached the app, and the monitoring dashboard stayed
green. The disk check came with the server template, and its last line is
`check_disk.sh | tee -a /var/log/checks.log`. A shell pipeline returns the exit code of its last
command. Here that was `tee`, and `tee` had nothing to complain about: every 15 minutes it wrote
`DISK CRITICAL - /srv/photos 100%` to the log and exited with 0. By noon the log held 21 of those
lines, and Dot, the support lead, had forwarded 164 tickets. The first had the subject "where are
my birds??". Ines, the senior SRE, read the log and said: "A check that cannot fail is a
decoration." In this exercise you write the check again in Go, as a command with a test for every
exit code.

## Task

Implement the stub in `checkdisk.go`. The constants are given.

```go
const (
	OK       = 0
	Warning  = 1
	Critical = 2
	Unknown  = 3
)

// Run reads `df -P` output from stdin, writes 1 status line to stdout and
// returns the exit code. -h is the 1 exception: see Rules.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int
```

The command around it is 1 line, and you do not write it here:

```go
func main() { os.Exit(checkdisk.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
```

Nagios, Icinga, Naemon and Sensu all run checks this way. They start the command, read its exit
code as the state (0 OK, 1 WARNING, 2 CRITICAL, 3 UNKNOWN) and show the first line of its stdout.
The part after `|` is performance data, which they turn into graphs.

Suggested order: follow the 4 steps in "Run the tests".

## Rules

### Flags

- `-w <percent>` is the warning threshold, default `80`. `-c <percent>` is the critical
  threshold, default `90`. Both are whole numbers.
- The thresholds must satisfy `1 <= w < c <= 100`. Otherwise the result is UNKNOWN. A flag that
  is not given keeps its default, so `-w 95` alone is UNKNOWN (95 is not below 90), and so is
  `-c 70` alone.
- `-x <mount>` leaves out the file system whose mount is exactly `<mount>`. It may be given any
  number of times. A mount that is not in the input changes nothing. `-x /var` does not leave out
  `/var/log`.
- The flags may come in any order. There are no positional arguments: an argument left over after
  the flags (`checkdisk /srv/photos`, `checkdisk -w 80 /srv/photos`) is UNKNOWN.
- The flag package rejects some arguments: an unknown flag such as `-p`, `-w abc`, `-c 90%`, or
  `-w` without a value. It then writes its message and the usage to stderr. `Run` writes the
  UNKNOWN line to stdout and returns 3.
- `-h` writes the usage to stderr and nothing to stdout, and `Run` returns 3: a check that printed
  its usage did not check anything. The usage names the flags `-w`, `-c` and `-x`. The usage that
  the flag package writes by default does that.
- `Run` checks the flags before it reads stdin. When the flags give UNKNOWN or `-h`, `Run` does not
  read stdin at all, so a bad flag never waits for `df`.

### Input

- Stdin is the output of `df -P`. `Run` splits it into lines on `\n`. A last line without `\n` is
  still a line.
- Any run of whitespace separates the fields of a line, as `strings.Fields` splits them: spaces,
  tabs and the `\r` of a Windows line ending.
- A **blank** line has no fields. `Run` skips it wherever it is, also before the header.
- The first line that is not blank is the **header**. Its first field must be exactly
  `Filesystem`. `Run` does not check the rest of the header (`df -P` on macOS writes
  `512-blocks`).
- Every later line that is not blank describes 1 file system and has exactly 6 fields:
  `<filesystem> <blocks> <used> <available> <capacity> <mount>`. `Run` uses only the 5th and the
  6th field and does not check fields 1 to 4.
- The **capacity** is a whole number from 0 to 100, directly followed by `%`: `0%`, `95%`, `100%`.
  `95`, `120%`, `-1%`, `abc%`, `%` and `-` are not capacities. `df` never writes a sign or a
  leading zero, and the tests do not use them: your check may accept or reject `+5%` and `095%`.
- Line numbers start at 1 and count every line of the input: the header and blank lines too. They
  are the numbers that `sed -n 6p` and your editor use.
- The **checked** file systems are the valid lines whose mount is not left out with `-x`. A line
  of a left-out mount must still be valid.

### State

- CRITICAL if the capacity of any checked file system is at least `c`.
- Otherwise WARNING if the capacity of any checked file system is at least `w`.
- Otherwise OK.
- A capacity equal to a threshold reaches it: `80%` is WARNING and `90%` is CRITICAL with the
  defaults. The order of the lines does not matter.

### The status line

- For OK, WARNING and CRITICAL, `Run` writes exactly 1 line to stdout, ending with `\n`:
  `DISK <STATE> - <summary> | <perfdata>`, where `<STATE>` is `OK`, `WARNING` or `CRITICAL`.
- For WARNING and CRITICAL, the summary lists every checked file system whose capacity is at
  least `w`, as `<mount> <N>%`, in input order, joined with `, ` (a comma and a space). A
  CRITICAL summary lists the file systems at or above `w` too, not only those at or above `c`.
- For OK, the summary is `all <n> file systems below <w>%`, where `<n>` is the number of checked
  file systems. The text does not change for 1 file system: `all 1 file systems below 80%`.
- The perfdata has `<mount>=<N>%;<w>;<c>;0;100` for every checked file system, also those below
  `w`, in input order, joined with 1 space. This is the Nagios format
  `label=value[unit];warn;crit;min;max`.

### UNKNOWN

- For every problem below, `Run` writes exactly 1 line to stdout, `DISK UNKNOWN - <reason>`,
  ending with `\n`, and returns 3. The reason is not empty. Its wording is yours, except that the
  reason for a malformed line contains `line <N>`.
- `Run` stops at the first problem. It never writes a status line for part of the input.

| Problem | Example |
|---|---|
| A flag the flag package rejects | `-p /`, `-w abc` |
| Thresholds outside `1 <= w < c <= 100` | `-w 90 -c 90`, `-w 0`, `-c 101` |
| A positional argument | `checkdisk /srv/photos` |
| No header: the first line that is not blank does not start with the field `Filesystem` | a data line first, `filesystem ...` |
| A malformed line; the reason contains `line <N>` of the first one | 5 or 7 fields, a capacity of `120%` |
| No checked file systems | empty input, a header only, every mount left out with `-x` |
| A read error from stdin, at any point | the disk under the log fails |

### Stdout and stderr

- Nothing goes to stderr for OK, WARNING and CRITICAL.
- For UNKNOWN, the tests check stderr only for a flag the flag package rejects (stderr is not
  empty) and for `-h` (stderr holds the usage). In the other cases, what goes to stderr is up to
  you.
- `Run` uses only its parameters. It does not call `os.Exit` and does not touch `os.Args`,
  `os.Stdin`, `os.Stdout`, `os.Stderr` or the global `flag.CommandLine`: the tests call `Run`
  many times in 1 process.

## Examples

```text
stdin:
Filesystem     1024-blocks      Used Available Capacity Mounted on
/dev/sda2         61611820  58530000   3081820      95% /
/dev/sda3         20511356  18665000   1846356      91% /var
/dev/sdb1        103081248  41232499  61848749      40% /home

Run([], stdin)
  stdout: DISK CRITICAL - / 95%, /var 91% | /=95%;80;90;0;100 /var=91%;80;90;0;100 /home=40%;80;90;0;100
  exit code 2

Run(["-x", "/", "-x", "/var"], stdin)
  stdout: DISK OK - all 1 file systems below 80% | /home=40%;80;90;0;100
  exit code 0

Run(["-w", "92", "-c", "96"], stdin)
  stdout: DISK WARNING - / 95% | /=95%;92;96;0;100 /var=91%;92;96;0;100 /home=40%;92;96;0;100
  exit code 1

Run(["-w", "95"], stdin)
  stdout: DISK UNKNOWN - thresholds -w 95 -c 90: want 1 <= w < c <= 100   (wording is yours)
  exit code 3

Run(["-h"], stdin)
  stdout: nothing
  stderr: the usage
  exit code 3
```

Line 3 of this input is blank, and line 4 has `87` without `%`:

```text
stdin:
Filesystem     1024-blocks      Used Available Capacity Mounted on
/dev/sda2         61611820  58530000   3081820      95% /

/dev/sdc1        102687672  84090934  13462354      87 /var/log

Run([], stdin)
  stdout: DISK UNKNOWN - line 4: capacity "87" does not end with %   (only "line 4" is checked)
  exit code 3
```

## Run the tests

    go test ./exercises/15-checkdisk/
    go test -run TestRunStates -v ./exercises/15-checkdisk/    # step 1: the state and the exit code
    go test -run TestRunOutput -v ./exercises/15-checkdisk/    # step 2: the status line
    go test -run TestRunFlags -v ./exercises/15-checkdisk/     # step 3: -w, -c, -x, -h and bad flags
    go test -run TestRunInput -v ./exercises/15-checkdisk/     # step 4: bad input and read errors

Run 1 step at a time; the 4 filters do not overlap. Step 1 checks only the exit code and the start
of the line (`DISK WARNING - `), with the default thresholds. Step 2 checks the whole line, and
that stderr stays empty. Every call to `Run` runs with a 5 s timeout and a recover. A
"TODO: implement" panic or a loop that never ends then fails 1 test with a message, and the run
goes on. Step 1 reads the 3 files `testdata/df-*.txt`, which hold the `df -P` output of a Linux
server with 7 file systems. `ExampleRun` runs only in a plain `go test`. An example cannot recover
a panic, so a plain run crashes until `Run` works.

## Docs you can use

- [`flag`](https://pkg.go.dev/flag), in particular
  [`flag.NewFlagSet`](https://pkg.go.dev/flag#NewFlagSet),
  [`ErrorHandling`](https://pkg.go.dev/flag#ErrorHandling) (`ContinueOnError`),
  [`FlagSet.SetOutput`](https://pkg.go.dev/flag#FlagSet.SetOutput),
  [`FlagSet.IntVar`](https://pkg.go.dev/flag#FlagSet.IntVar),
  [`FlagSet.Func`](https://pkg.go.dev/flag#FlagSet.Func),
  [`FlagSet.Args`](https://pkg.go.dev/flag#FlagSet.Args) and
  [`flag.ErrHelp`](https://pkg.go.dev/flag#pkg-variables)
- [`os.Exit`](https://pkg.go.dev/os#Exit): read why deferred functions do not run
- [`bufio.Scanner`](https://pkg.go.dev/bufio#Scanner) and
  [`Scanner.Err`](https://pkg.go.dev/bufio#Scanner.Err)
- [`strings.Fields`](https://pkg.go.dev/strings#Fields),
  [`strings.CutSuffix`](https://pkg.go.dev/strings#CutSuffix) and
  [`strings.Join`](https://pkg.go.dev/strings#Join)
- [`strconv.Atoi`](https://pkg.go.dev/strconv#Atoi)
- [`fmt.Fprintf`](https://pkg.go.dev/fmt#Fprintf) and
  [`fmt.Fprintln`](https://pkg.go.dev/fmt#Fprintln)
- [`errors.Is`](https://pkg.go.dev/errors#Is)
- [`testing/iotest`](https://pkg.go.dev/testing/iotest): `ErrReader` is the failing stdin of step 4
- [Monitoring Plugins Development Guidelines](https://www.monitoring-plugins.org/doc/guidelines.html):
  the sections "Plugin Return Codes" and "Performance data"
- [POSIX `df`](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/df.html): what `-P`
  promises about the output format

## Stretch goals

- Make it a real command: put the `main` from "Task" in `cmd/checkdisk/main.go` of a module of
  your own, and run `df -P | go run ./cmd/checkdisk -x /boot`. Check the exit code with `echo $?`.
- Watch a pipeline hide an exit code: run `go run ./cmd/checkdisk < df.txt | tee -a /tmp/checks.log`
  once with and once without `set -o pipefail`, and compare `echo $?`.
- Run `df` yourself with `os/exec` instead of reading stdin. Keep `Run` testable: pass it a
  function that returns the `df` output.
- Check inodes too: `df -Pi` has the same 6 columns. A disk with free space and no free inodes
  cannot store 1 more file.
- Accept the Nagios range syntax for thresholds (`-w 10:20`, `-c @0:5`) from the guidelines above.
- Add a `-t tmpfs` flag that leaves out every file system of a type. `df -PT` adds a type column,
  so the line rules change.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 15 hint 1
    go run ./cmd/peek 15 hint 2
    go run ./cmd/peek 15 hint 3
    go run ./cmd/peek 15 explanation
    go run ./cmd/peek 15 walkthrough
    go run ./cmd/peek 15 solution
