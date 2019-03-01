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
	"context"
	"os/exec"
	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"golang.org/x/sync/errgroup"
	"time"
	"strconv"
	"gerrit.opencord.org/voltha-bbsim/device"
	"fmt"
)

const (
	DEFAULT Mode = iota
	AAA
	BOTH
)

type Mode int

type TestManager struct {
	DhcpServerIP string
	Pid          []int
	testers      map[device.Devkey]*Tester
	ctx          context.Context
	cancel       context.CancelFunc
}

type Tester struct {
	Key device.Devkey
	Mode Mode
	ctx          context.Context
	cancel       context.CancelFunc
}

func NewTestManager(opt *option) *TestManager {
	t := new(TestManager)
	t.DhcpServerIP = opt.dhcpservip
	return t
}

func (*TestManager) CreateTester(opt *option, key device.Devkey) *Tester{
	logger.Debug("CreateTester() called")
	t := new(Tester)
	t.Mode = opt.Mode
	t.Key = key
	return t
}

//Blocking
func (tm *TestManager) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	tm.ctx = ctx
	tm.cancel = cancel
	tm.testers = map[device.Devkey]*Tester{}
	logger.Info("TestManager start")
	return nil
}

func (tm *TestManager) Stop() error {
	if tm.cancel != nil {
		tm.cancel()
	}
	tm.Initialize()
	logger.Debug("TestManager Done")
	return nil
}

func (tm *TestManager) StartTester (key device.Devkey, t *Tester) error {
	logger.Debug("StartTester called with key:%v", key)
	if t.Mode == DEFAULT {
		//Empty
	} else if t.Mode == AAA || t.Mode == BOTH {
		eg, child := errgroup.WithContext(tm.ctx)
		child, cancel := context.WithCancel(child)
		t.ctx = child
		t.cancel = cancel
		eg.Go(func() error {
			err := activateWPASupplicant(key)
			if err != nil {
				return err
			}
			return nil
		})

		if t.Mode == BOTH {
			waitForDHCP := 3
			eg.Go(func() error {
				tick := time.NewTicker(time.Second)
				counter := 0
				defer func() {
					tick.Stop()
					logger.Debug("exeDHCPTest Done")
				}()

			L:
				for counter < waitForDHCP {
					select{
					case <-tick.C:
						counter ++
						if counter == waitForDHCP {	// TODO: This should be fixed
							break L
						}
					case <-child.Done():
						return nil
					}
				}
				err := activateDHCPClient(key)
				if err != nil {
					return err
				}
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			return err
		}
	}
	tm.testers[key] = t
	return nil
}

func (tm *TestManager) StopTester (key device.Devkey) error {
	ts := tm.testers[key]
	ts.cancel()
	delete(tm.testers, key)
	return nil
}

func (tm *TestManager) Initialize() {
	logger.Info("TestManager Initialize () called")
	pids := tm.Pid
	logger.Debug("Runnig Process: %v", pids)
	KillProcesses(pids)
	exec.Command("rm", "/var/run/dhcpd.pid").Run()    //This is for DHCP server activation
	exec.Command("touch", "/var/run/dhcpd.pid").Run() //This is for DHCP server activation
}

func KillProcesses(pids []int) error {
	for _, pname := range pids {
		killProcess(pname)
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

func activateDHCPServer (veth string, serverip string) error {
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
	cmd := "/usr/local/bin/dhcpd"
	conf := "/etc/dhcp/dhcpd.conf"
	logfile := "/tmp/dhcplog"
	err = exec.Command(cmd, "-cf", conf, veth, "-tf", logfile).Run()
	if err != nil {
		logger.Error("Fail to activateDHCP Server (): %s", err)
		return err
	}
	logger.Info("DHCP Server is successfully activated !")
	return err
}