package checkdisk_test

import (
	"errors"
	"testing"
	"testing/iotest"

	checkdisk "github.com/yornik/go-practice/exercises/15-checkdisk"
)

func TestRunInputAccepted(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			"blank lines are skipped",
			"\n" + header + "\n\n" + row("/", 95) + "\n   \n\t\n" + row("/var", 40) + "\n\n",
			"DISK CRITICAL - / 95% | /=95%;80;90;0;100 /var=40%;80;90;0;100",
		},
		{
			"no newline after the last line",
			header + "\n" + row("/", 95),
			"DISK CRITICAL - / 95% | /=95%;80;90;0;100",
		},
		{
			"tabs and runs of spaces between fields",
			header + "\n/dev/sda1\t1000  950\t\t50    95%\t/\n",
			"DISK CRITICAL - / 95% | /=95%;80;90;0;100",
		},
		{
			"Windows line endings",
			header + "\r\n" + row("/", 95) + "\r\n" + row("/var", 40) + "\r\n",
			"DISK CRITICAL - / 95% | /=95%;80;90;0;100 /var=40%;80;90;0;100",
		},
		{
			"only the first field of the header is checked",
			"Filesystem 512-blocks Used Available Capacity Mounted on\n" + row("/", 95) + "\n",
			"DISK CRITICAL - / 95% | /=95%;80;90;0;100",
		},
		{
			"fields 1 to 4 are not checked",
			df("tmpfs - - - 95% /run"),
			"DISK CRITICAL - /run 95% | /run=95%;80;90;0;100",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runText(t, tc.input)
			checkLine(t, call(nil, tc.input), res, checkdisk.Critical, tc.want)
		})
	}
}

func TestRunInputNoFileSystems(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty input", ""},
		{"only blank lines", "\n  \n\n"},
		{"no header", row("/", 40) + "\n" + row("/var", 30) + "\n"},
		{"header only", header + "\n"},
		{"header and blank lines", header + "\n\n\n"},
		{"lowercase header", "filesystem 1024-blocks Used Available Capacity Mounted on\n" + row("/", 40) + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runText(t, tc.input)
			checkUnknown(t, call(nil, tc.input), res)
		})
	}
}

func TestRunInputMalformed(t *testing.T) {
	// line is the number of the bad line in the input. The header and blank
	// lines count.
	cases := []struct {
		name  string
		input string
		args  []string
		line  int
	}{
		{"5 fields", df(row("/", 40), "/dev/sdb1 1000 400 600 40%"), nil, 3},
		{"7 fields", df("/dev/sdc1 1000 400 600 40% /mnt/usb stick", row("/", 40)), nil, 2},
		{"1 field", df(row("/", 40), row("/var", 40), "/dev/mapper/vg0-photos"), nil, 4},
		{"capacity without %", df("/dev/sda1 1000 950 50 95 /"), nil, 2},
		{"capacity above 100", df("/dev/sda1 1000 950 50 120% /"), nil, 2},
		{"capacity negative", df("/dev/sda1 1000 950 50 -1% /"), nil, 2},
		{"capacity not a number", df("/dev/sda1 1000 950 50 abc% /"), nil, 2},
		{"capacity only %", df("/dev/sda1 1000 950 50 % /"), nil, 2},
		{"capacity is a dash", df("/dev/sda1 1000 950 50 - /"), nil, 2},
		{"capacity in field 6", df("/dev/sda1 1000 950 50 / 95%"), nil, 2},
		{"blank lines count", "\n" + header + "\n\n" + row("/", 40) + "\n\n/dev/sdb1 1000 400 600 40%\n", nil, 6},
		{"the first bad line is reported", df(row("/", 40), row("/var", 40), "bad line", row("/home", 40), "another bad line"), nil, 4},
		{"a bad line of an excluded mount", df(row("/", 40), "/dev/sda1 1000 950 50 120% /boot"), []string{"-x", "/boot"}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			what := call(tc.args, tc.input)
			res := runText(t, tc.input, tc.args...)
			reason := checkUnknown(t, what, res)
			checkLineNumber(t, what, reason, tc.line)
		})
	}
}

func TestRunInputReadError(t *testing.T) {
	errDisk := errors.New("read /dev/stdin: input/output error")
	cases := []struct {
		name string
		data string
	}{
		{"before any data", ""},
		{"after the header", header + "\n"},
		{"after valid lines", df(row("/", 40), row("/var", 30))},
		{"in the middle of a line", header + "\n" + row("/", 40) + "\n/dev/sdb1 1000 4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			what := call(nil, tc.data) + "\nand then the read error " + errDisk.Error()
			var res result
			if tc.data == "" {
				res = run(t, iotest.ErrReader(errDisk))
			} else {
				res = run(t, errReader(tc.data, errDisk))
			}
			checkUnknown(t, what, res)
		})
	}
}
