package dccli

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	"net/http"
	"os"
	"sync"
	"testing"
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
	if host := MustInferDockerHost(); host != "localhost" {
		t.Errorf("found '%v', expected 'localhost'", host)
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
	wg := sync.WaitGroup{}
	wg.Add(1)
	MustConnectWithDefaults(func() error {
		defaultLogger.Print("attempting to connect to mockserver...", mockServerURL)
		_, err := http.Get(mockServerURL)
		if err == nil {
			wg.Done()
			defaultLogger.Print("connected to mockserver")
		}
		return err
	})

	wg.Wait()
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

	MustConnectWithDefaults(func() error {
		defaultLogger.Print("attempting to connect to mockserver 1...", mockServerURL)
		_, err := http.Get(mockServerURL)
		if err == nil {
			wg.Done()
			defaultLogger.Print("connected to mockserver compose1")
		}
		return err
	})

	mockServerURL2 := fmt.Sprintf("http://%v:%v", MustInferDockerHost(), compose2.Containers["ms"].MustGetFirstPublicPort(3000, "tcp"))

	MustConnectWithDefaults(func() error {
		defaultLogger.Print("attempting to connect to mockserver 2...", mockServerURL2)
		_, err := http.Get(mockServerURL2)
		if err == nil {
			wg.Done()
			defaultLogger.Print("connected to mockserver compose2")
		}
		return err
	})

	wg.Wait()

}

func TestComplexDepends(t *testing.T) {
	fileOut :=
		`
version: '3'
services:
  cassandra:
    image: cassandra:3.11
    ports:
    - "9042:9042"
  statsd:
    image: hopsoft/graphite-statsd
    ports:
    - "8080:80"
    - "2003:2003"
    - "8125:8125"
    - "8126:8126"
  cadence:
    image: ubercadence/server:0.4.0
    ports:
    - "7933:7933"
    - "7934:7934"
    - "7935:7935"
    environment:
    - "CASSANDRA_SEEDS=cassandra"
    - "STATSD_ENDPOINT=statsd:8125"
    depends_on:
    - cassandra
    - statsd
  cadence-web:
    image: ubercadence/web:3.1.2
    environment:
    - "CADENCE_TCHANNEL_PEERS=cadence:7933"
    ports:
    - "8088:8088"
    depends_on:
    - cadence
`

	var cfg Config
	err := yaml.Unmarshal([]byte(fileOut), &cfg)
	require.NoError(t, err)

	c := MustStart(OptionWithCompose(cfg),
		OptionWithProjectName(t.Name()),
		OptionWithLogger(defaultLogger),
		OptionStartRetries(2),
		OptionForcePull(true), OptionKeepAround(true))
	defer c.MustCleanup()
	require.NotNil(t, c.Containers)

}
