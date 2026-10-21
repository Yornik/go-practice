package checkdisk_test

import (
	"fmt"
	"os"
	"strings"

	checkdisk "github.com/yornik/go-practice/exercises/15-checkdisk"
)

func ExampleRun() {
	df := `Filesystem     1024-blocks      Used Available Capacity Mounted on
/dev/sda2         61611820  24173397  34357832      42% /
/dev/sda1           999320    262021    687333      28% /boot
/dev/sdb1       3844640564 3637798900  14609635     100% /srv/models
/dev/sdc1        102687672  84090934  13462354      87% /var/log
`
	code := checkdisk.Run([]string{"-x", "/boot"}, strings.NewReader(df), os.Stdout, os.Stderr)
	fmt.Println("exit code:", code)
	// Output:
	// DISK CRITICAL - /srv/models 100%, /var/log 87% | /=42%;80;90;0;100 /srv/models=100%;80;90;0;100 /var/log=87%;80;90;0;100
	// exit code: 2
}
