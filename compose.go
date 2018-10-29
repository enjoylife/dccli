/*
package dccli provides a Go wrapper around Docker Compose, useful for integration testing.
Check out the test cases for example usage
*/
package dccli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Compose is the main type exported by the package, used to interact with a running Docker Compose configuration.
type Compose struct {
	Containers  map[string]*ContainerInfo
	fileName    string
	projectName string
	logger      *log.Logger
}

var (
	defaultLogger   = log.New(os.Stdout, "compose: ", log.LstdFlags|log.Lshortfile)
	composeUpRegexp = regexp.MustCompile(`(?m)docker start <-.*\(u?'(.*)'\)`)
)

type internalCFG struct {
	forcePull   bool
	rmFirst     bool
	compose     Config
	projectName string
	logger      *log.Logger
}

// Option is the type used for defining optional configuration
type Option func(*internalCFG)

// If OptionForcePull is true, it attempts do pull newer versions of the images.
func OptionForcePull(b bool) Option {
	return func(c *internalCFG) {
		c.forcePull = b
	}
}

// If OptionRMFirst is true, it attempts to kill and delete containers before starting new ones.
func OptionRMFirst(b bool) Option {
	return func(c *internalCFG) {
		c.rmFirst = b
	}
}

// OptionWithCompose sets the compose file to use during start
func OptionWithCompose(o Config) Option {
	return func(c *internalCFG) {
		c.compose = o
	}
}

// OptionWithProjectName sets the project name to use
func OptionWithProjectName(p string) Option {
	return func(c *internalCFG) {
		c.projectName = p
	}
}

// OptionWithLogger sets the logger to use
func OptionWithLogger(l *log.Logger) Option {
	return func(c *internalCFG) {
		c.logger = l
	}
}

// Start starts a Docker Compose configuration.
// TODO(mclemens) accept an io.Reader or a set of options
func Start(opts ...Option) (*Compose, error) {
	cfg := internalCFG{
		projectName: "default", //randStringBytes(8),
		logger:      defaultLogger,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	cfg.logger.Println("initializing...")

	// we deep copy the config to not overwrite the passed in configuration
	bs, err := yaml.Marshal(&cfg.compose)
	if err != nil {
		return nil, err
	}
	var cmpCFG Config
	yaml.Unmarshal(bs, &cmpCFG)

	// we remove networks across the top level config as well as the services,
	// for we rely on the default network that is set per project
	cmpCFG.Networks = nil
	for k, svc := range cmpCFG.Services {
		updatedSVC := svc
		updatedSVC.Networks = nil
		cmpCFG.Services[k] = updatedSVC
	}

	bsMod, err := yaml.Marshal(&cmpCFG)
	if err != nil {
		return nil, err
	}
	fileStr := string(bsMod)
	fName, err := writeTmp(fileStr)
	if err != nil {
		return nil, err
	}

	if cfg.forcePull {
		cfg.logger.Println("pulling images...")
		if _, err := composeRun(fName, cfg.projectName, "pull"); err != nil {
			return nil, fmt.Errorf("compose: error pulling images: %v", err)
		}
	}

	if cfg.rmFirst {
		if err := composeKill(fName, cfg.projectName); err != nil {
			return nil, err
		}
		if err := composeRm(fName, cfg.projectName); err != nil {
			return nil, err
		}
	}

	cfg.logger.Println("starting containers...")
	out, err := composeRun(fName, cfg.projectName, "--verbose", "up", "-d")
	if err != nil {
		return nil, fmt.Errorf("compose: error starting containers: %v", err)
	}
	cfg.logger.Println("containers started")

	matches := composeUpRegexp.FindAllStringSubmatch(out, -1)
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match[1])
	}

	containers := make(map[string]*ContainerInfo)

	for _, id := range ids {
		container, err := Inspect(id)
		if err != nil {
			return nil, err
		}
		if !container.State.Running {
			return nil, fmt.Errorf("compose: container '%v' is not running", container.Name)
		}
		key := container.Name[1:]
		key = findKey(key, cmpCFG.Services)
		//if cfg.projectName != "" {
		//	key = strings.TrimPrefix(key, strings.ToLower(cfg.projectName)+ "_")
		//}
		if key == "" {
			return nil, fmt.Errorf("compose: could not map container name: %s, to list of services", key)
		}
		containers[key] = container
	}
	c := &Compose{fileName: fName, Containers: containers, projectName: cfg.projectName, logger: cfg.logger}
	c.logger.Println("done initializing...")

	return c, nil
}

func findKey(dockerName string, serviceNames map[string]Service) string {
	for k := range serviceNames {
		if strings.Contains(dockerName, k) {
			return k
		}
	}
	return ""
}

// MustStart is like Start, but panics on error.
func MustStart(opts ...Option) *Compose {
	compose, err := Start(opts...)
	if err != nil {
		panic(err)
	}
	return compose
}

// Cleanup will try and kill then remove any running containers for the current configuration.
func (c *Compose) Cleanup() error {
	c.logger.Println("removing stale containers...")
	netName := strings.ToLower(c.projectName) + "_default"
	return combineErr(composeKill(c.fileName, c.projectName),
		composeRm(c.fileName, c.projectName),
		composeRMNetwork(netName))
}

// MustCleanup is like Cleanup, but panics on error.
func (c *Compose) MustCleanup() {
	if err := c.Cleanup(); err != nil {
		panic(err)
	}
}

func runCmd(name string, args ...string) (string, error) {
	var outBuf bytes.Buffer

	cmd := exec.Command(name, args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	cmdErr := cmd.Run()
	out := outBuf.String()
	if cmdErr == nil {
		return out, nil
	}
	err := fmt.Errorf("failed running %s %v: %s", name, args, cmdErr)

	// the output from docker is very noisy, therefore to aide in diagnosing
	// the errors we only show the log lines which containing meaningful error messages
	scanner := bufio.NewScanner(bytes.NewReader(outBuf.Bytes()))
	errCount := 0
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "ERROR:") ||
			strings.HasPrefix(scanner.Text(), "compose.cli.errors") {
			errCount++
			err = combineErr(err, errors.New(scanner.Text()))
		}
	}
	if errScan := scanner.Err(); errScan != nil {
		err = combineErr(err, fmt.Errorf("could not output error lines: %s", errScan))
	}
	if errCount == 0 {
		err = combineErr(err, errors.New(out))
	}

	return out, err
}

func composeKill(fName string, pName string) error {
	_, err := composeRun(fName, pName, "kill")
	if err != nil {
		return fmt.Errorf("compose: error killing stale containers: %v", err)
	}
	return err
}

func composeRm(fName string, pName string) error {
	out, err := composeRun(fName, pName, "rm", "--force")
	if err != nil {
		return fmt.Errorf("compose: error removing stale containers: %s, %v", out, err)
	}
	return nil
}

func composeRMNetwork(netName string) error {
	//out, err := dockerRun("network", "inspect", netName)
	//if strings.Contains(out,"No such network") {
	//	return nil
	//}
	//defaultLogger.Println(out)
	out, err := dockerRun("network", "rm", netName)
	if err != nil {
		return fmt.Errorf("compose: error removing network %s: %s, %v", netName, out, err)
	}
	return nil
}

func composeRun(fName string, projectName string, otherArgs ...string) (string, error) {
	args := []string{"-f", fName, "-p", projectName}
	args = append(args, otherArgs...)
	return runCmd("docker-compose", args...)
}

func dockerRun(cmdAndArgs ...string) (string, error) {
	return runCmd("docker", cmdAndArgs...)
}
