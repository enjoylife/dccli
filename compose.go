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
	"time"
)

// Compose is the main type exported by the package, used to interact with a running Docker Compose configuration.
type Compose struct {
	ids         []string
	publicCfg   Config
	containers  map[string]*ContainerInfo
	fileName    string
	projectName string
	logger      *log.Logger
	cfg         internalCFG
}

var (
	defaultLogger   = log.New(os.Stdout, "[dccli] ", log.LstdFlags|log.Lshortfile)
	composeUpRegexp = regexp.MustCompile(`(?m)docker start|inspect_container <-.*\(u?'(.*)'\)`)
)

type internalCFG struct {
	forcePull    bool
	rmFirst      bool
	compose      Config
	projectName  string
	logger       *log.Logger
	connectTries int
	keeparound   bool
	preventStop  bool
	outFile      string
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

// If OptionKeepAround is true, it will not remove the containers after cleanup
func OptionKeepAround(b bool) Option {
	return func(c *internalCFG) {
		c.keeparound = b
	}
}

// If OptionPreventStop is true, it will not stop the containers during cleanup
func OptionPreventStop(b bool) Option {
	return func(c *internalCFG) {
		c.preventStop = b
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
		if p != "" {
			// lowercases everything and strips out all underscores
			p = strings.Replace(strings.ToLower(p), "_", "", -1)
		}
		c.projectName = p
	}
}

// OptionWithLogger sets the logger to use
func OptionWithLogger(l *log.Logger) Option {
	return func(c *internalCFG) {
		c.logger = l
	}
}

// OptionStartRetries sets the number of times to retry the starting docker-compose
func OptionStartRetries(count int) Option {
	return func(c *internalCFG) {
		c.connectTries = count
	}
}

func OptionWriteToFile(path string) Option {
	return func(c *internalCFG) {
		c.outFile = path
	}
}

// Start starts a Docker Compose configuration.
// TODO(mclemens) accept an io.Reader or a set of options
func Start(opts ...Option) (*Compose, error) {
	cfg := internalCFG{
		projectName:  "dccli",
		logger:       defaultLogger,
		connectTries: 3,
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

	// use given file if provided
	if cfg.outFile != "" {
		f, err := os.Create(cfg.outFile)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		_, err = f.WriteString(fileStr)
		if err != nil {
			return nil, err
		}

	} else {
		cfg.outFile, err = writeTmp(fileStr)
		if err != nil {
			return nil, err
		}
	}

	cfg.logger.Printf("wrote docker-compose.yaml configuration to file: %s", cfg.outFile)

	if cfg.forcePull {
		cfg.logger.Println("pulling images...")
		if _, err := composeRun(cfg.outFile, cfg.projectName, "pull"); err != nil {
			return nil, fmt.Errorf("compose: error pulling images: %v", err)
		}
	}

	if cfg.rmFirst {
		cfg.logger.Println("WARN: OptionRMFirst is slow and wasteful, don't use it.")
		cfg.logger.Println("killing and removing images...")
		if err := composeKill(cfg.outFile, cfg.projectName); err != nil {
			return nil, err
		}
		if err := composeRm(cfg.outFile, cfg.projectName); err != nil {
			return nil, err
		}
	}

	c := &Compose{
		ids:         nil, // will be filled in via connect
		publicCfg:   cmpCFG,
		containers:  make(map[string]*ContainerInfo),
		fileName:    cfg.outFile,
		projectName: cfg.projectName,
		logger:      cfg.logger,
		cfg:         cfg,
	}

	err = connect(cfg.connectTries, time.Second*2, func() error {
		out, err := composeRun(cfg.outFile, cfg.projectName, "--verbose", "up", "-d")
		if err != nil {
			return err
		}
		cfg.logger.Println("containers started")

		matches := composeUpRegexp.FindAllStringSubmatch(out, -1)
		var ids []string
		for _, match := range matches {
			if match[1] != "" {
				ids = append(ids, match[1])
			}
		}
		c.ids = ids

		if err := c.updateContainers(); err != nil {
			cfg.logger.Printf("retrying after: %s\n", err)
			return err
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("compose: error starting containers: %v", err)
	}

	var containerNames []string
	for k := range c.containers {
		containerNames = append(containerNames, k)
	}

	c.logger.Println("done initializing...")
	c.logger.Printf("Tail logs via: docker-compose -p %s -f %s logs -f %s\n",
		cfg.projectName,
		cfg.outFile,
		strings.Join(containerNames, " "))

	return c, nil
}

func (c *Compose) updateContainers() error {

	for _, id := range c.ids {
		container, err := Inspect(id)
		if err != nil {
			return err
		}
		//if !container.State.Running {
		//	c.logger.Printf("compose: container '%v' is not yet running\n", container.Name)
		//}
		key := container.Name[1:]
		key = findKey(key, c.publicCfg.Services)
		if key == "" {
			return fmt.Errorf("could not map key: %s, to list of services", key)
		}
		c.containers[key] = container
	}
	return nil
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

func (c *Compose) GetContainer(key string) (*ContainerInfo, error) {
	if err := c.updateContainers(); err != nil {
		return nil, err
	}
	i, ok := c.containers[key]
	if !ok {
		return nil, fmt.Errorf("no container %s found", key)
	}
	return i, nil
}

// Cleanup will try and kill then remove any running containers for the current configuration.
func (c *Compose) Cleanup() error {
	if !c.cfg.preventStop {
		if err := composeStop(c.fileName, c.projectName); err != nil {
			return err
		}
	}
	if c.cfg.keeparound {
		return nil
	}

	c.logger.Println("removing stale containers, images, volumes, and networks...")
	// cleaning based on docker network normalization, which lowercases everything
	// and strips out all underscores
	//netName := c.projectName + "_default"
	return combineErr(composeKill(c.fileName, c.projectName),
		composeDown(c.fileName, c.projectName),
		dockerPrune())
}

// MustCleanup is like Cleanup, but panics on error.
func (c *Compose) MustCleanup() {
	if err := c.Cleanup(); err != nil {
		panic(err)
	}
}

// Connect
func (c *Compose) Connect(policy RetryPolicy, connectFunc func() error) error {
	var err error
	var tryAgain bool
	var wait time.Duration

	for {
		err = connectFunc()
		if err == nil {
			return nil
		}

		tryAgain, wait = policy.AttemptAgain(err)
		if !tryAgain {
			return err
		}
		c.logger.Printf("connect failed, retrying in %d second(s): %v\n", int64(wait.Seconds()), err)
		time.Sleep(wait)

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
	out, err := composeRun(fName, pName, "rm", "--force", "-v")
	if err != nil {
		return fmt.Errorf("compose: error removing stale containers: %s, %v", out, err)
	}
	return nil
}

func composeStop(fName string, pName string) error {
	out, err := composeRun(fName, pName, "stop")
	if err != nil {
		return fmt.Errorf("compose: error stopping stale containers: %s, %v", out, err)
	}
	return nil
}

func composeDown(fName string, pName string) error {
	out, err := composeRun(fName, pName, "down", "-v", "--remove-orphans")
	if err != nil {
		return fmt.Errorf("compose: error downing stale containers: %s, %v", out, err)
	}
	return nil
}

func composeRMNetwork(netName string) error {
	var out string
	err := connect(3, time.Second*2, func() error {
		o, err := dockerRun("network", "rm", netName)
		out = o
		return err
	})

	if err != nil {
		return fmt.Errorf("compose: error removing network %s: %s, %v", netName, out, err)
	}
	return nil
}

func dockerPrune() error {
	var out string
	err := connect(3, time.Second*2, func() error {
		o, err := dockerRun("volume", "prune", "-f")
		out = o
		return err
	})

	if err != nil {
		return fmt.Errorf("compose: error system prune: %s, %v", out, err)
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
