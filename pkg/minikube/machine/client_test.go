/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package machine

import (
	"bufio"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/machine/drivers/virtualbox"
	"github.com/docker/machine/libmachine/drivers"

	"github.com/docker/machine/libmachine/drivers/plugin/localbinary"
	"k8s.io/minikube/pkg/minikube/constants"
)

var expectedDrivers = map[string]drivers.Driver{
	vboxConfig: virtualbox.NewDriver("", ""),
}

const vboxConfig = `
{
        "IPAddress": "192.168.99.101",
        "MachineName": "minikube",
        "SSHUser": "docker",
        "SSHPort": 33627,
        "SSHKeyPath": "/home/sundarp/.minikube/machines/minikube/id_rsa",
        "StorePath": "/home/sundarp/.minikube",
        "SwarmMaster": false,
        "SwarmHost": "",
        "SwarmDiscovery": "",
        "VBoxManager": {},
        "HostInterfaces": {},
        "CPU": 4,
        "Memory": 16384,
        "DiskSize": 20000,
        "NatNicType": "82540EM",
        "Boot2DockerURL": "file:///home/sundarp/.minikube/cache/iso/minikube-v1.0.6.iso",
        "Boot2DockerImportVM": "",
        "HostDNSResolver": false,
        "HostOnlyCIDR": "192.168.99.1/24",
        "HostOnlyNicType": "82540EM",
        "HostOnlyPromiscMode": "deny",
        "UIType": "headless",
        "HostOnlyNoDHCP": false,
        "NoShare": false,
        "DNSProxy": true,
        "NoVTXCheck": false
}
`

func TestGetDriver(t *testing.T) {
	var tests = []struct {
		description string
		driver      string
		rawDriver   []byte
		expected    drivers.Driver
		err         bool
	}{
		{
			description: "vbox correct",
			driver:      "virtualbox",
			rawDriver:   []byte(vboxConfig),
			expected:    virtualbox.NewDriver("", ""),
		},
		{
			description: "unknown driver",
			driver:      "unknown",
			rawDriver:   []byte("?"),
			expected:    nil,
			err:         true,
		},
		{
			description: "vbox bad",
			driver:      "virtualbox",
			rawDriver:   []byte("?"),
			expected:    nil,
			err:         true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.description, func(t *testing.T) {
			t.Parallel()
			driver, err := getDriver(test.driver, test.rawDriver)
			if err != nil && !test.err {
				t.Errorf("Unexpected error: %s", err)
			}
			if err == nil && test.err {
				t.Errorf("No error returned, but expected err")
			}
			if driver != nil && test.expected.DriverName() != driver.DriverName() {
				t.Errorf("Driver names did not match, actual: %s, expected: %s", driver.DriverName(), test.expected.DriverName())
			}
		})
	}
}

func TestLocalClientNewHost(t *testing.T) {
	f := clientFactories[ClientTypeLocal]
	c := f.NewClient("", "")

	var tests = []struct {
		description string
		driver      string
		rawDriver   []byte
		err         bool
	}{
		{
			description: "host vbox correct",
			driver:      "virtualbox",
			rawDriver:   []byte(vboxConfig),
		},
		{
			description: "host vbox incorrect",
			driver:      "virtualbox",
			rawDriver:   []byte("?"),
			err:         true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.description, func(t *testing.T) {
			t.Parallel()
			host, err := c.NewHost(test.driver, test.rawDriver)
			// A few sanity checks that we can do on the host
			if host != nil {
				if host.DriverName != test.driver {
					t.Errorf("Host driver name is not correct.  Expected: %s, got: %s", test.driver, host.DriverName)
				}
				if host.Name != host.Driver.GetMachineName() {
					t.Errorf("Host name is not correct.  Expected :%s, got: %s", host.Driver.GetMachineName(), host.Name)
				}
			}
			if err != nil && !test.err {
				t.Errorf("Unexpected error: %s", err)
			}
			if err == nil && test.err {
				t.Errorf("No error returned, but expected err")
			}
		})
	}
}

func TestNewAPIClient(t *testing.T) {
	var tests = []struct {
		description string
		clientType  ClientType
		err         bool
	}{
		{
			description: "Client type local",
			clientType:  ClientTypeLocal,
		},
		{
			description: "Client type RPC",
			clientType:  ClientTypeRPC,
		},
		{
			description: "Incorrect client type",
			clientType:  -1,
			err:         true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.description, func(t *testing.T) {
			t.Parallel()
			_, err := NewAPIClient(test.clientType)
			if err != nil && !test.err {
				t.Errorf("Unexpected error: %s", err)
			}
			if err == nil && test.err {
				t.Errorf("No error returned, but expected err")
			}
		})
	}
}

func makeTempDir() string {
	tempDir, err := ioutil.TempDir("", "minipath")
	if err != nil {
		log.Fatal(err)
	}
	tempDir = filepath.Join(tempDir, ".minikube")
	os.Setenv(constants.MinikubeHome, tempDir)
	return constants.GetMinipath()
}

func TestRunNotDriver(t *testing.T) {
	tempDir := makeTempDir()
	defer os.RemoveAll(tempDir)
	StartDriver()
	if !localbinary.CurrentBinaryIsDockerMachine {
		t.Fatal("CurrentBinaryIsDockerMachine not set. This will break driver initialization.")
	}
}

func TestRunDriver(t *testing.T) {
	// This test is a bit complicated. It verifies that when the root command is
	// called with the proper environment variables, we setup the libmachine driver.

	tempDir := makeTempDir()
	defer os.RemoveAll(tempDir)
	os.Setenv(localbinary.PluginEnvKey, localbinary.PluginEnvVal)
	os.Setenv(localbinary.PluginEnvDriverName, "virtualbox")

	// Capture stdout and reset it later.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	// Run the command asynchronously. It should listen on a port for connections.
	go StartDriver()

	// The command will write out what port it's listening on over stdout.
	reader := bufio.NewReader(r)
	addr, _, err := reader.ReadLine()
	if err != nil {
		t.Fatal("Failed to read address over stdout.")
	}
	os.Stdout = old

	// Now that we got the port, make sure we can connect.
	if _, err := net.Dial("tcp", string(addr)); err != nil {
		t.Fatal("Driver not listening.")
	}
}
