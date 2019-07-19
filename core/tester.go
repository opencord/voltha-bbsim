/*
 * Copyright 2018-present Open Networking Foundation

 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package core

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/opencord/voltha-bbsim/common/logger"
	"github.com/opencord/voltha-bbsim/device"
)

// TestManager is the structure for test manager
type TestManager struct {
	DhcpServerIP string
	Pid          []int
	testers      map[string]map[device.Devkey]*Tester
	ctx          context.Context
	cancel       context.CancelFunc
}

// Tester is the structure for Test
type Tester struct {
	Type     string
	Key      device.Devkey
	Testfunc func(device.Devkey) error
	Waitsec  int
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewTestManager returns new TestManager
func NewTestManager(opt *Option) *TestManager {
	t := new(TestManager)
	t.DhcpServerIP = opt.dhcpservip
	return t
}

// CreateTester creates instance of Tester
func (*TestManager) CreateTester(testtype string, opt *Option, key device.Devkey, fn func(device.Devkey) error, waitsec int) *Tester {
	logger.Debug("CreateTester() called")
	t := new(Tester)
	t.Type = testtype
	t.Key = key
	t.Testfunc = fn
	t.Waitsec = waitsec
	return t
}

// Start does starting action - Blocking Call
func (tm *TestManager) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	tm.ctx = ctx
	tm.cancel = cancel
	tm.testers = make(map[string]map[device.Devkey]*Tester)
	tm.testers["AAA"] = map[device.Devkey]*Tester{}
	tm.testers["DHCP"] = map[device.Devkey]*Tester{}
	logger.Info("TestManager start")
	return nil
}

// Stop does stopping action
func (tm *TestManager) Stop() error {
	if tm.cancel != nil {
		tm.cancel()
	}
	tm.Initialize()
	logger.Debug("TestManager Done")
	return nil
}

// StartTester starts the test
func (tm *TestManager) StartTester(t *Tester) error {
	testtype := t.Type
	key := t.Key
	waitsec := t.Waitsec

	// TODO use timers instead of this crazy loop: https://gobyexample.com/timers

	logger.Debug("StartTester type:%s called with key:%v", testtype, key)
	child, cancel := context.WithCancel(tm.ctx)
	t.ctx = child
	t.cancel = cancel
	go func() error {
		tick := time.NewTicker(time.Second)
		counter := 0
		defer func() {
			tick.Stop()
			logger.Debug("Tester type:%s with key %v Done", testtype, key)
		}()

	L:
		for counter < waitsec {
			select {
			case <-tick.C:
				counter++
				if counter == waitsec { // TODO: This should be fixed
					break L
				}
			case <-child.Done():
				return nil
			}
		}
		err := t.Testfunc(key)
		if err != nil {
			return err
		}
		return nil
	}()

	tm.testers[testtype][key] = t
	return nil
}

// StopTester stops the test
func (tm *TestManager) StopTester(testtype string, key device.Devkey) error {
	ts := tm.testers[testtype][key]
	ts.cancel()
	delete(tm.testers[testtype], key)
	return nil
}

// Initialize test manager
func (tm *TestManager) Initialize() {
	logger.Info("TestManager Initialize () called")
	pids := tm.Pid
	logger.Debug("Runnig Process: %v", pids)

	err := KillProcesses(pids)
	if err != nil {
		logger.Error("%v", err)
	}

	err = exec.Command("rm", "/var/run/dhcpd.pid").Run() // This is for DHCP server activation
	if err != nil {
		logger.Error("%v", err)
	}

	err = exec.Command("touch", "/var/run/dhcpd.pid").Run() // This is for DHCP server activation
	if err != nil {
		logger.Error("%v", err)
	}
}

// KillProcesses kill process by specified pid
func KillProcesses(pids []int) error {
	for _, pname := range pids {
		err := killProcess(pname)
		if err != nil {
			logger.Error("%v", err)
		}
	}
	return nil
}

func killProcess(pid int) error {
	err := exec.Command("kill", strconv.Itoa(pid)).Run()
	if err != nil {
		logger.Error("Fail to kill %d: %v", pid, err)
		return err
	}
	logger.Info("Successfully killed %d", pid)
	return nil
}

func activateWPASupplicant(key device.Devkey) (err error) {
	if err = startEAPClient(key.Intfid, key.ID); err != nil {
		errmsg := fmt.Sprintf("Failed to activate WPA Supplicant intfid: %d onuid: %d", key.Intfid, key.ID)
		logger.Error(errmsg)
	}
	logger.Info("Successfuly activateWPASupplicant() for intfid:%d onuid:%d", key.Intfid, key.ID)
	return nil
}

func activateDHCPClient(key device.Devkey) (err error) {
	if err = startDHCPClient(key.Intfid, key.ID); err != nil {
		errmsg := fmt.Sprintf("Failed to activate DHCP client intfid: %d onuid: %d", key.Intfid, key.ID)
		logger.Error(errmsg)
	}
	return nil
}

func activateDHCPServer(veth string, serverip string) error {
	err := exec.Command("ip", "addr", "add", serverip, "dev", veth).Run()
	if err != nil {
		logger.Error("Fail to add ip to %s address: %s", veth, err)
		return err
	}
	err = exec.Command("ip", "link", "set", veth, "up").Run()
	if err != nil {
		logger.Error("Fail to set %s up: %s", veth, err)
		return err
	}
	dhcp := "/usr/local/bin/dhcpd"
	conf := "/etc/dhcp/dhcpd.conf"
	logfile := "/tmp/dhcplog"
	var stderr bytes.Buffer
	cmd := exec.Command(dhcp, "-cf", conf, veth, "-tf", logfile)
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		logger.Error("Fail to activateDHCP Server (): %s, %s", err, stderr.String())
		return err
	}
	logger.Info("DHCP Server is successfully activated !")
	return err
}
