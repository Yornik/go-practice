// Package checkdisk is a monitoring check for disk usage in the style of a
// Nagios plugin: it reads the output of `df -P`, prints 1 status line and
// returns an exit code that the monitoring system turns into a state.
package checkdisk

import "io"

// The exit codes of a check, as the Nagios plugin API defines them. Run
// returns one of them.
const (
	OK       = 0
	Warning  = 1
	Critical = 2
	Unknown  = 3
)

// Run is the whole command except the process around it. The main function
// of the command is 1 line:
//
//	os.Exit(checkdisk.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
//
// args are the command-line arguments without the program name. Run accepts
// the flags -w (warning threshold, default 80), -c (critical threshold,
// default 90) and -x (a mount to leave out; it may repeat), and no other
// arguments. The thresholds are whole percentages with 1 <= w < c <= 100.
//
// Run reads `df -P` output from stdin: a header line whose first field is
// "Filesystem", then 1 line per file system with exactly 6 fields, the 5th a
// capacity such as "95%". Blank lines are skipped.
//
// Run writes exactly 1 line to stdout and returns OK, Warning or Critical:
//
//	DISK CRITICAL - / 95%, /var 91% | /=95%;80;90;0;100 /var=91%;80;90;0;100 /home=40%;80;90;0;100
//
// For a bad flag or bad input it writes "DISK UNKNOWN - <reason>" and
// returns Unknown. The flag package writes its own messages and the usage to
// stderr. -h writes only the usage, to stderr, and returns Unknown. Nothing
// goes to stderr for OK, Warning and Critical.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	panic("TODO: implement")
}
