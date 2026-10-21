package checkdisk_test

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	checkdisk "github.com/yornik/go-practice/exercises/15-checkdisk"
)

func TestRunFlagsThresholds(t *testing.T) {
	cases := []struct {
		args  []string
		input string
		code  int
		want  string
	}{
		{
			[]string{"-w", "50", "-c", "60"},
			df(row("/", 55), row("/var", 65), row("/home", 40)),
			checkdisk.Critical,
			"DISK CRITICAL - / 55%, /var 65% | /=55%;50;60;0;100 /var=65%;50;60;0;100 /home=40%;50;60;0;100",
		},
		{
			[]string{"-c", "60", "-w", "50"},
			df(row("/", 55), row("/home", 40)),
			checkdisk.Warning,
			"DISK WARNING - / 55% | /=55%;50;60;0;100 /home=40%;50;60;0;100",
		},
		{
			[]string{"-c", "95"},
			df(row("/", 92)),
			checkdisk.Warning,
			"DISK WARNING - / 92% | /=92%;80;95;0;100",
		},
		{
			[]string{"-w", "70"},
			df(row("/", 65)),
			checkdisk.OK,
			"DISK OK - all 1 file systems below 70% | /=65%;70;90;0;100",
		},
		{
			[]string{"-w", "1", "-c", "100"},
			df(row("/", 0), row("/var", 1), row("/srv/models", 100)),
			checkdisk.Critical,
			"DISK CRITICAL - /var 1%, /srv/models 100% | /=0%;1;100;0;100 /var=1%;1;100;0;100 /srv/models=100%;1;100;0;100",
		},
		{
			[]string{"-w", "1", "-c", "100"},
			df(row("/", 0)),
			checkdisk.OK,
			"DISK OK - all 1 file systems below 1% | /=0%;1;100;0;100",
		},
	}
	for _, tc := range cases {
		res := runText(t, tc.input, tc.args...)
		checkLine(t, call(tc.args, tc.input), res, tc.code, tc.want)
	}
}

func TestRunFlagsExclude(t *testing.T) {
	input := df(row("/", 40), row("/boot", 95), row("/run", 85), row("/var", 30), row("/var/log", 20))
	cases := []struct {
		args []string
		code int
		want string
	}{
		{
			[]string{"-x", "/boot"},
			checkdisk.Warning,
			"DISK WARNING - /run 85% | /=40%;80;90;0;100 /run=85%;80;90;0;100 /var=30%;80;90;0;100 /var/log=20%;80;90;0;100",
		},
		{
			[]string{"-x", "/boot", "-x", "/run"},
			checkdisk.OK,
			"DISK OK - all 3 file systems below 80% | /=40%;80;90;0;100 /var=30%;80;90;0;100 /var/log=20%;80;90;0;100",
		},
		{
			[]string{"-x", "/run", "-w", "50", "-x", "/boot", "-x", "/"},
			checkdisk.OK,
			"DISK OK - all 2 file systems below 50% | /var=30%;50;90;0;100 /var/log=20%;50;90;0;100",
		},
		{
			// A mount that is not in the input changes nothing.
			[]string{"-x", "/nowhere"},
			checkdisk.Critical,
			"DISK CRITICAL - /boot 95%, /run 85% | /=40%;80;90;0;100 /boot=95%;80;90;0;100 /run=85%;80;90;0;100 /var=30%;80;90;0;100 /var/log=20%;80;90;0;100",
		},
		{
			// -x compares the whole mount: /var does not leave out /var/log.
			[]string{"-x", "/var", "-x", "/boot", "-x", "/run"},
			checkdisk.OK,
			"DISK OK - all 2 file systems below 80% | /=40%;80;90;0;100 /var/log=20%;80;90;0;100",
		},
		{
			// A longer -x does not leave out a shorter mount either.
			[]string{"-x", "/boot/efi", "-x", "/var/log"},
			checkdisk.Critical,
			"DISK CRITICAL - /boot 95%, /run 85% | /=40%;80;90;0;100 /boot=95%;80;90;0;100 /run=85%;80;90;0;100 /var=30%;80;90;0;100",
		},
	}
	for _, tc := range cases {
		res := runText(t, input, tc.args...)
		checkLine(t, call(tc.args, input), res, tc.code, tc.want)
	}
}

func TestRunFlagsExcludeAll(t *testing.T) {
	input := df(row("/", 40), row("/boot", 95))
	args := []string{"-x", "/", "-x", "/boot"}
	res := runText(t, input, args...)
	checkUnknown(t, call(args, input), res)
}

func TestRunFlagsInvalid(t *testing.T) {
	input := df(row("/", 40))
	cases := []struct {
		name string
		args []string
	}{
		{"w equal to c", []string{"-w", "90", "-c", "90"}},
		{"w above c", []string{"-w", "95", "-c", "90"}},
		{"w above the default c", []string{"-w", "95"}},
		{"c below the default w", []string{"-c", "70"}},
		{"w 0", []string{"-w", "0", "-c", "90"}},
		{"w negative", []string{"-w", "-5"}},
		{"c above 100", []string{"-w", "80", "-c", "101"}},
		{"positional argument", []string{"/srv/models"}},
		{"positional argument after flags", []string{"-w", "80", "/srv/models"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runText(t, input, tc.args...)
			checkUnknown(t, call(tc.args, input), res)
		})
	}
}

func TestRunFlagsRejected(t *testing.T) {
	// The flag package rejects these. It writes its message and the usage
	// to stderr.
	input := df(row("/", 40))
	cases := []struct {
		name string
		args []string
	}{
		{"unknown flag", []string{"-p", "/"}},
		{"w not a number", []string{"-w", "abc"}},
		{"c with a percent sign", []string{"-c", "90%"}},
		{"w without a value", []string{"-w"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			what := call(tc.args, input)
			res := runText(t, input, tc.args...)
			checkUnknown(t, what, res)
			if res.stderr == "" {
				t.Errorf("%s\nwrote nothing to stderr, want the flag package's message and the usage", what)
			}
		})
	}
}

func TestRunFlagsHelp(t *testing.T) {
	input := df(row("/", 40))
	args := []string{"-h"}
	what := call(args, input)
	res := runText(t, input, args...)
	if res.code != checkdisk.Unknown {
		t.Errorf("%s\nreturned exit code %d, want %d (Unknown)", what, res.code, checkdisk.Unknown)
	}
	if res.stdout != "" {
		t.Errorf("%s\nwrote to stdout %q, want nothing", what, res.stdout)
	}
	for _, flag := range []string{"-w", "-c", "-x"} {
		if !strings.Contains(res.stderr, flag) {
			t.Errorf("%s\nwrote to stderr\n%s\nwant a usage that names the flag %s", what, indent(res.stderr), flag)
		}
	}
}

// watchedReader fails every Read and records that Read was called.
type watchedReader struct {
	read atomic.Bool
}

func (r *watchedReader) Read([]byte) (int, error) {
	r.read.Store(true)
	return 0, errors.New("stdin was read")
}

func TestRunFlagsBeforeStdin(t *testing.T) {
	// For these arguments the result is clear before any input, so Run
	// must not read stdin at all.
	cases := []struct {
		name string
		args []string
	}{
		{"help", []string{"-h"}},
		{"unknown flag", []string{"-p", "/"}},
		{"w not a number", []string{"-w", "abc"}},
		{"w above c", []string{"-w", "95", "-c", "90"}},
		{"positional argument", []string{"/srv/models"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdin := &watchedReader{}
			res := run(t, stdin, tc.args...)
			what := fmt.Sprintf("Run(%s) with a stdin that records every Read", showArgs(tc.args))
			if res.code != checkdisk.Unknown {
				t.Errorf("%s\nreturned exit code %d, want %d (Unknown)", what, res.code, checkdisk.Unknown)
			}
			if stdin.read.Load() {
				t.Errorf("%s\nread stdin, want Run to reject the flags without reading stdin", what)
			}
		})
	}
}
