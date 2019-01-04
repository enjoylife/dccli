package dccli

import (
	"fmt"
	"github.com/gocql/gocql"
	"github.com/stretchr/testify/require"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

var cfg = Config{
	Version: "3",
	Services: map[string]Service{
		"mysql": {
			Image: "mysql:5.7",
			Ports: []string{"3306"},
			Environment: []string{
				"MYSQL_ROOT_PASSWORD=root",
				"MYSQL_DATABASE=test",
			},
		},
		"ms": {
			Image: "ubuntu:trusty",
			Ports: []string{"3000", "1090"},
			Command: []string{"python3", "-c", `import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
PORT = 3000
class HelloWorld(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-type","text/plain")
        self.end_headers()
 
        self.wfile.write(bytes("Hello world!", "utf8"))
        return
httpd = HTTPServer(("", PORT), HelloWorld)
print("serving at port", PORT)
sys.stdout.flush()
httpd.serve_forever()
`},
		},
	},
}

func TestGoodYML(t *testing.T) {
	c := MustStart(OptionWithCompose(cfg),
		OptionWithProjectName("TestGoodYML"),
		OptionWithLogger(defaultLogger),
		OptionStartRetries(2),
		OptionForcePull(true), OptionRMFirst(true))
	defer c.MustCleanup()
	require.NotNil(t, c.Containers)
	if c.Containers["ms"].Name != "/testgoodyml_ms_1" {
		t.Errorf("found name '%v', expected '/ms", c.Containers["ms"].Name)
	}
	if c.Containers["mysql"].Name != "/testgoodyml_mysql_1" {
		t.Errorf("found name '%v', expected '/mysql", c.Containers["mysql"].Name)
	}
	//if port := compose.Containers["ms"].MustGetFirstPublicPort(3000, "tcp"); port != 10000 {
	//	t.Fatalf("found port %v, expected 10000", port)
	//}
}

func TestRestart(t *testing.T) {
	TestGoodYML(t)
}

func TestBadYML(t *testing.T) {
	t.Skip("figure out a how to create a bad config")
	c, err := Start(OptionForcePull(true), OptionRMFirst(true))
	if err == nil {
		defer c.MustCleanup()
		t.Error("expected error")
	}
}

func TestMustInferDockerHost(t *testing.T) {
	envHost := os.Getenv("DOCKER_HOST")
	defer os.Setenv("DOCKER_HOST", envHost)

	os.Setenv("DOCKER_HOST", "")
	if host := MustInferDockerHost(); host != "127.0.0.1" {
		t.Errorf("found '%v', expected '127.0.0.1'", host)
	}
	os.Setenv("DOCKER_HOST", "tcp://192.168.99.100:2376")
	if host := MustInferDockerHost(); host != "192.168.99.100" {
		t.Errorf("found '%v', expected '192.168.99.100'", host)
	}
}

func TestMustConnectWithDefaults(t *testing.T) {
	usedCFG := cfg
	c := MustStart(OptionWithCompose(usedCFG),
		OptionForcePull(true), OptionRMFirst(true))
	defer c.MustCleanup()
	require.NotNil(t, c.Containers)
	require.NotNil(t, c.Containers["ms"])
	mockServerURL := fmt.Sprintf("http://%v:%v", MustInferDockerHost(), c.Containers["ms"].MustGetFirstPublicPort(3000, "tcp"))
	err := c.Connect(NewSimpleRetryPolicy(3, time.Second), func() error {
		defaultLogger.Print("attempting to connect to mockserver...", mockServerURL)
		_, err := http.Get(mockServerURL)
		if err == nil {
			defaultLogger.Print("connected to mockserver")
		}
		return err
	})
	require.NoError(t, err)
}

func TestPolicyBadConnect(t *testing.T) {
	c := MustStart(OptionWithCompose(cfg),
		OptionForcePull(true), OptionRMFirst(true))
	defer c.MustCleanup()

	//mockServerURL := fmt.Sprintf("http://%v:%v", MustInferDockerHost(), c.Containers["ms"].MustGetFirstPublicPort(3000, "tcp"))
	badMockServerURL := fmt.Sprintf("http://%v/:%v", MustInferDockerHost(), c.Containers["ms"].MustGetFirstPublicPort(1090, "tcp"))

	retries := 3
	actualTries := 0
	err := c.Connect(&SimpleRetryPolicy{
		NumRetries: retries,
		Wait:       time.Second * 1,
	}, func() error {
		defaultLogger.Print("attempting bad connect to mockserver. Should fail...", badMockServerURL)
		actualTries++
		_, err := http.Get(badMockServerURL)
		if err == nil {
			t.Fatal("should not have connected!")
		}
		return err
	})

	if err == nil {
		t.Errorf("should have failed to connect")
	}

	shouldHaveAttempted := retries + 1

	if shouldHaveAttempted != actualTries {
		t.Errorf("should have made the correct number of attempts %d != %d", shouldHaveAttempted, actualTries)
	}
}

func TestInspectUnknownContainer(t *testing.T) {
	_, err := Inspect("bad")
	if err == nil {
		t.Error("expected error")
	}
}

func TestMustInspect(t *testing.T) {
	c := MustStart(OptionWithCompose(cfg),
		OptionForcePull(true), OptionRMFirst(true))
	defer c.MustCleanup()

	ms := MustInspect(c.Containers["ms"].ID)
	if ms.Name != "/dccli_ms_1" {
		t.Errorf("found '%v', expected '/dccli_ms_1", ms.Name)
	}
}

func TestScyllaDB(t *testing.T) {

	var cfgDB = Config{
		Version: "3.7",
		Services: map[string]Service{
			"scylla": {
				Image:   "scylladb/scylla:2.3.1",
				Ports:   []string{"7000", "7001", "7199", "9042", "9160"},
				Command: []string{"--smp=1", "--developer-mode=1", "--overprovisioned=1"},
				Volumes: []*Volume{
					{
						Target: "/var/lib/scylla/",
						Type:   "tmpfs",
					},
				},
			},
		},
	}

	c := MustStart(OptionWithCompose(cfgDB),
		OptionForcePull(true), OptionRMFirst(true))
	defer c.MustCleanup()

	err := c.Connect(&SimpleRetryPolicy{
		NumRetries: 7,
		Wait:       time.Second * 3,
	}, func() error {

		b := gocql.NewCluster(MustInferDockerHost())
		b.ProtoVersion = 4
		b.Keyspace = "system"
		b.DisableInitialHostLookup = true
		b.Compressor = &gocql.SnappyCompressor{}
		b.Port = int(c.Containers["scylla"].MustGetFirstPublicPort(9042, "tcp"))

		_, err := b.CreateSession()
		return err
	})

	if err != nil {
		t.Errorf("should have connected")
	}
}

func TestParallelMustConnectWithDefaults(t *testing.T) {

	compose1 := MustStart(OptionWithCompose(cfg), OptionWithProjectName("compose1"),
		OptionForcePull(true), OptionRMFirst(true))
	defer compose1.MustCleanup()
	compose2 := MustStart(OptionWithCompose(cfg), OptionWithProjectName("compose2"),
		OptionForcePull(true), OptionRMFirst(true))
	defer compose2.MustCleanup()
	wg := sync.WaitGroup{}
	wg.Add(2)

	mockServerURL := fmt.Sprintf("http://%v:%v", MustInferDockerHost(), compose1.Containers["ms"].MustGetFirstPublicPort(3000, "tcp"))

	go func() {
		err1 := compose1.Connect(NewSimpleRetryPolicy(3, time.Second), func() error {
			defaultLogger.Print("attempting to connect to mockserver 1...", mockServerURL)
			_, err := http.Get(mockServerURL)
			if err == nil {
				wg.Done()
				defaultLogger.Print("connected to mockserver compose1")
			}
			return err
		})
		require.NoError(t, err1)
	}()

	go func() {
		mockServerURL2 := fmt.Sprintf("http://%v:%v", MustInferDockerHost(), compose2.Containers["ms"].MustGetFirstPublicPort(3000, "tcp"))

		err2 := compose2.Connect(NewSimpleRetryPolicy(3, time.Second), func() error {
			defaultLogger.Print("attempting to connect to mockserver 2...", mockServerURL2)
			_, err := http.Get(mockServerURL2)
			if err == nil {
				wg.Done()
				defaultLogger.Print("connected to mockserver compose2")
			}
			return err
		})

		require.NoError(t, err2)
	}()

	wg.Wait()

}
