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
