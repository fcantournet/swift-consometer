package main

import (
	"strings"
)

func failOnError(msg string, err error) {
	if err != nil {
		log.Fatal(msg, err)
	}
}

func countErrors(failed <-chan map[error]string) map[string]int {
	c := make(map[string][]string)
	s := make(map[string]int)
	for failure := range failed {
		for errkey := range failure {
			strkey := strings.SplitN(errkey.Error(), ":", 4)
			key := strkey[len(strkey)-1]
			c[key] = append(c[key], failure[errkey])
		}
	}
	for key := range c {
		s[key] = len(c[key])
	}
	for key := range s {
		log.Error(key, "  #", s[key], "#")
	}
	return s
}
