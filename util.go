package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"strings"
)

// Panicf takes the return value of recover() and outputs data to the log with
// the stack trace appended. Arguments are handled in the manner of
// fmt.Printf. Arguments should format to a string which identifies what the
// panic code was doing. Returns a non-nil error if it recovered from a panic.
func Panicf(r interface{}, format string, v ...interface{}) error {
	if r != nil {
		// Same as net/http
		const size = 64 << 10
		buf := make([]byte, size)
		buf = buf[:runtime.Stack(buf, false)]
		id := fmt.Sprintf(format, v...)
		log.Printf("panic serving %s: %v\n%s", id, r, string(buf))
		return fmt.Errorf("unexpected panic: %v", r)
	}
	return nil
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s\n", err.Error())
		os.Exit(1)
	}
}

func callerPrettyfier(f *runtime.Frame) (string, string) {
	s := strings.Split(f.Function, ".")
	funcname := s[len(s)-1]
	_, filename := path.Split(f.File)
	return funcname, filename
}

func userHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home
	} else if runtime.GOOS == "linux" {
		home := os.Getenv("XDG_CONFIG_HOME")
		if home != "" {
			return home
		}
	}
	return os.Getenv("HOME")
}
