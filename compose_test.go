package dccli

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
)

var goodYML = `
test_mockserver:
  container_name: ms
  image: ubuntu:trusty
  ports:
    - "10000:1080"
    - "1090"
test_postgres:
  container_name: pg
  image: postgres
  ports:
    - "5432"
`

var cfg = Config{
	Version: "3.7",
	Services: map[string]Service{
		"pg": {
			//ContainerName: "pg",
			Image: "postgres",
			Ports: []string{"5432"},
		},
		"ms": {
			Image: "ubuntu:trusty",
			//ContainerName: "ms",
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
		OptionForcePull(true), OptionRMFirst(true))
	defer c.MustCleanup()

	if c.Containers["ms"].Name != "/testgoodyml_ms_1" {
		t.Fatalf("found name '%v', expected '/ms", c.Containers["ms"].Name)
	}
	if c.Containers["pg"].Name != "/testgoodyml_pg_1" {
		t.Fatalf("found name '%v', expected '/pg", c.Containers["pg"].Name)
	}
	//if port := compose.Containers["ms"].MustGetFirstPublicPort(3000, "tcp"); port != 10000 {
	//	t.Fatalf("found port %v, expected 10000", port)
	//}
}

func TestRestartGoodYML(t *testing.T) {
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
	c := MustStart(OptionWithCompose(cfg),
		OptionForcePull(true), OptionRMFirst(true))
	defer c.MustCleanup()

	mockServerURL := fmt.Sprintf("http://%v:%v", MustInferDockerHost(), c.Containers["ms"].MustGetFirstPublicPort(3000, "tcp"))

	MustConnectWithDefaults(func() error {
		defaultLogger.Print("attempting to connect to mockserver...", mockServerURL)
		_, err := http.Get(mockServerURL)
		if err == nil {
			defaultLogger.Print("connected to mockserver")
		}
		return err
	})
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
	if ms.Name != "/default_ms_1" {
		t.Errorf("found '%v', expected '/default_ms_1", ms.Name)
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
