package dccli

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
)

var dockerHostRegexp = regexp.MustCompile("://([^:]+):")

// InferDockerHost returns the current docker host based on the contents of the DOCKER_HOST environment variable.
// If DOCKER_HOST is not set, it returns "localhost".
func InferDockerHost() (string, error) {
	envHost := os.Getenv("DOCKER_HOST")
	if len(envHost) == 0 {
		return "127.0.0.1", nil
	}

	matches := dockerHostRegexp.FindAllStringSubmatch(envHost, -1)
	if len(matches) != 1 || len(matches[0]) != 2 {
		return "", fmt.Errorf("compose: cannot parse DOCKER_HOST '%v'", envHost)
	}
	return matches[0][1], nil
}

// MustInferDockerHost is like InferDockerHost, but panics on error.
func MustInferDockerHost() string {
	dockerHost, err := InferDockerHost()
	if err != nil {
		panic(err)
	}
	return dockerHost
}

func writeTmp(content string) (string, error) {
	f, err := ioutil.TempFile("", "docker-compose-*.yaml")
	if err != nil {
		return "", fmt.Errorf("compose: error creating temp file: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return "", fmt.Errorf("compose: error writing temp file: %v", err)
	}

	return f.Name(), nil
}

// combineErr takes in a variadic number of errors and returns a single error
// with its message being a concatenation of all the supplied errors.
// If all of the supplied errors are nil, a nil error will be returned.
func combineErr(errs ...error) error {
	root := ""
	for _, e := range errs {
		if e == nil {
			continue
		}
		if root != "" {
			root = fmt.Sprintf("%s: %s", root, e)
		} else {
			root = fmt.Sprintf("%s", e)
		}
	}

	if root == "" {
		return nil
	}

	return errors.New(root)
}
