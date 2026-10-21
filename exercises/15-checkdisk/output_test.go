package checkdisk_test

import (
	"testing"

	checkdisk "github.com/yornik/go-practice/exercises/15-checkdisk"
)

func TestRunOutputLine(t *testing.T) {
	cases := []struct {
		name  string
		input string
		code  int
		want  string
	}{
		{
			"critical, the README example",
			df(row("/", 95), row("/var", 91), row("/home", 40)),
			checkdisk.Critical,
			"DISK CRITICAL - / 95%, /var 91% | /=95%;80;90;0;100 /var=91%;80;90;0;100 /home=40%;80;90;0;100",
		},
		{
			"critical lists the warning mounts too",
			df(row("/", 95), row("/var", 85), row("/home", 40)),
			checkdisk.Critical,
			"DISK CRITICAL - / 95%, /var 85% | /=95%;80;90;0;100 /var=85%;80;90;0;100 /home=40%;80;90;0;100",
		},
		{
			"warning",
			df(row("/", 42), row("/srv/models", 84)),
			checkdisk.Warning,
			"DISK WARNING - /srv/models 84% | /=42%;80;90;0;100 /srv/models=84%;80;90;0;100",
		},
		{
			"usage equal to the thresholds",
			df(row("/", 80), row("/var", 90)),
			checkdisk.Critical,
			"DISK CRITICAL - / 80%, /var 90% | /=80%;80;90;0;100 /var=90%;80;90;0;100",
		},
		{
			"ok",
			df(row("/", 42), row("/boot", 28), row("/srv/models", 72)),
			checkdisk.OK,
			"DISK OK - all 3 file systems below 80% | /=42%;80;90;0;100 /boot=28%;80;90;0;100 /srv/models=72%;80;90;0;100",
		},
		{
			"ok with 1 file system",
			df(row("/", 0)),
			checkdisk.OK,
			"DISK OK - all 1 file systems below 80% | /=0%;80;90;0;100",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runText(t, tc.input)
			checkLine(t, call(nil, tc.input), res, tc.code, tc.want)
		})
	}
}

func TestRunOutputInputOrder(t *testing.T) {
	// The summary and the perfdata keep the order of the input. They are
	// not sorted by mount or by usage.
	cases := []struct {
		name  string
		input string
		code  int
		want  string
	}{
		{
			"not sorted by usage",
			df(row("/var", 91), row("/", 95), row("/tmp", 85)),
			checkdisk.Critical,
			"DISK CRITICAL - /var 91%, / 95%, /tmp 85% | /var=91%;80;90;0;100 /=95%;80;90;0;100 /tmp=85%;80;90;0;100",
		},
		{
			"not sorted by mount",
			df(row("/var/log", 10), row("/home", 81), row("/boot", 20), row("/", 82)),
			checkdisk.Warning,
			"DISK WARNING - /home 81%, / 82% | /var/log=10%;80;90;0;100 /home=81%;80;90;0;100 /boot=20%;80;90;0;100 /=82%;80;90;0;100",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runText(t, tc.input)
			checkLine(t, call(nil, tc.input), res, tc.code, tc.want)
		})
	}
}

func TestRunOutputFromFile(t *testing.T) {
	input := readFile(t, "df-critical.txt")
	res := runText(t, input)
	checkLine(t, call(nil, input), res, checkdisk.Critical,
		"DISK CRITICAL - /srv/models 100%, /var/log 87% | "+
			"/dev=0%;80;90;0;100 /run=1%;80;90;0;100 /=42%;80;90;0;100 /dev/shm=0%;80;90;0;100 "+
			"/boot=28%;80;90;0;100 /srv/models=100%;80;90;0;100 /var/log=87%;80;90;0;100")
}
