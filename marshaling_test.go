package dccli

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	"testing"
)

func TestStruct(t *testing.T) {
	fileOut :=
		`version: '3.7'
services:
  nats:
    image: 'nats:0.8.0'
    entrypoint: "/gnatsd -DV"
    expose:
    - "4222"
    ports:
    - "8222:8222"
    hostname: nats-server
    networks:
    - test-network
  ten-gallon:
    container_name: homedepot
    image : gcr.io/theia-bot/ten-gallon:0.0.1
    networks:
      - test-network
    command: ["serve"]
#    ports:
#      - "6600:6600"
    healthcheck:
      test: ["CMD", "/ten-gallon-linux-amd64", "health"]
      interval: 30s
      timeout: 5s
      start_period: 5s
      retries: 20
    depends_on:
      - nats
networks:
  test-network:`

	var cfg Config
	err := yaml.Unmarshal([]byte(fileOut), &cfg)
	require.NoError(t, err)
	//spew.Dump(cfg)
	assert.Contains(t, cfg.Services, "ten-gallon")
	assert.Contains(t, cfg.Services, "nats")

	assert.Contains(t, cfg.Networks, "test-network")

	tgSvc := cfg.Services["ten-gallon"]

	require.NotNil(t, tgSvc)
	//assert.Equal(t, "homedepot", tgSvc.ContainerName)
	assert.Equal(t, "gcr.io/theia-bot/ten-gallon:0.0.1", tgSvc.Image)
	assert.Contains(t, tgSvc.Command, "serve")
	assert.NotContains(t, tgSvc.Ports, "6600:6600")

	require.NotNil(t, tgSvc.HealthCheck)
	tgHealth := tgSvc.HealthCheck
	assert.Contains(t, tgHealth.Test, "CMD")
	assert.Contains(t, tgHealth.Test, "/ten-gallon-linux-amd64")
	assert.Contains(t, tgHealth.Test, "health")

	assert.Equal(t, "30s", tgHealth.Interval)
	assert.Equal(t, "5s", tgHealth.Timeout)
	assert.Equal(t, "5s", tgHealth.StartPeriod)
	assert.Equal(t, "20", tgHealth.Retries)

	assert.Contains(t, tgSvc.DependsOn, "nats")

}

func TestDependsOn(t *testing.T) {

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
}

func TestVolumes(t *testing.T) {
	const yamlSource = `
version: "3.7"
services:
  web:
    image: 'nginx:alpine'
    volumes:
      - type: volume
        source: mydata
        target: /data
        volume:
          nocopy: true
      - type: bind
        source: ./static
        target: /opt/app/static
  db:
    image: postgres:latest
    volumes:
      - "/var/run/postgres/postgres.sock:/var/run/postgres/postgres.sock"
      - "dbdata:/var/lib/postgresql/data"
volumes:
  mydata:
  dbdata:
`

	var cfg Config
	err := yaml.Unmarshal([]byte(yamlSource), &cfg)
	require.NoError(t, err)

	require.NotEmpty(t, cfg.Volumes)

	_, err = yaml.Marshal(cfg)
	require.NoError(t, err)

}
