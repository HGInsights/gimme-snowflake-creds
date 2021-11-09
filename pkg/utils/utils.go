package utils

import (
	"io/ioutil"
	"os"
	"strings"
)

func hasEnv() bool {
	_, err := os.Stat("/.dockerenv")
	return !os.IsNotExist(err)
}

func hasCGroup() bool {
	f, _ := ioutil.ReadFile("/proc/self/cgroup")
	return strings.Contains(string(f), "docker")
}

func InDocker() bool {
	return hasEnv() || hasCGroup()
}
