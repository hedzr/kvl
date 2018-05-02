package etcd

import (
	log "github.com/sirupsen/logrus"
	"github.com/hedzr/kvl/set"
)

func info(msg string) {
	if set.Verbose {
		log.Print(msg)
	}
}

func infof(fmt string, args ...interface{}) {
	if set.Verbose {
		log.Printf(fmt, args...)
	}
}

func warn(msg string) {
	if set.Verbose {
		log.Print(msg)
	}
}

func warnf(fmt string, args ...interface{}) {
	if set.Verbose {
		log.Printf(fmt, args...)
	}
}

func Error(msg string) {
	if set.Verbose {
		log.Print(msg)
	}
}

func Errorf(fmt string, args ...interface{}) {
	if set.Verbose {
		log.Printf(fmt, args...)
	}
}

func fatal(msg string) {
	log.Fatal(msg)
}

func fatalf(fmt string, args ...interface{}) {
	log.Fatalf(fmt, args...)
}

func vf(format string, args ... interface{}) {
	if set.Verbose {
		log.Debugf(format, args...)
	}
}

func v(args ... interface{}) {
	if set.Verbose {
		log.Debug(args...)
	}
}
