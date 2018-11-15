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
	"sync"
	"time"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"gerrit.opencord.org/voltha-bbsim/common/utils"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	DEFAULT Mode = iota
	AAA
	BOTH
)

type Mode int

type Tester struct {
	Mode         Mode
	AAAWait      int
	DhcpWait     int
	DhcpServerIP string
	Processes    []string
	Intvl        int
	cancel       context.CancelFunc
}

func NewTester(opt *option) *Tester {
	t := new(Tester)
	t.AAAWait = opt.aaawait
	t.DhcpWait = opt.dhcpwait
	t.DhcpServerIP = opt.dhcpservip
	t.Intvl = opt.intvl_test
	t.Mode = opt.Mode
	return t
}

//Blocking
func (t *Tester) Start(s *Server) error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	defer func() {
		cancel()
		t.Initialize()
		logger.Debug("Tester Done")
	}()
	logger.Info("Tester Run() start")
	if t.Mode == DEFAULT {
		//Empty
	} else if t.Mode == AAA || t.Mode == BOTH {
		eg, child := errgroup.WithContext(ctx)
		child, cancel := context.WithCancel(child)
		eg.Go(func() error {
			defer func() {
				logger.Debug("exeAAATest Done")
			}()
			err := t.exeAAATest(child, s, t.AAAWait)
			return err
		})

		if t.Mode == BOTH {
			eg.Go(func() error {
				defer func() {
					logger.Debug("exeDHCPTest Done")
				}()

				err := t.exeDHCPTest(ctx, s, t.DhcpWait)
				return err
			})
		}
		if err := eg.Wait(); err != nil {
			logger.Error("Error happened in tester: %s", err)
			cancel()
			return err
		} else {
			logger.Debug("Test successfully finished")
		}
	}
	return nil
}

func (t *Tester) Stop(s *Server) error {
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

func (t *Tester) Initialize() {
	logger.Info("Tester Initialize () called")
	processes := t.Processes
	logger.Debug("Runnig Process: %s", processes)
	KillProcesses(processes)
	exec.Command("rm", "/var/run/dhcpd.pid").Run()    //This is for DHCP server activation
	exec.Command("touch", "/var/run/dhcpd.pid").Run() //This is for DHCP server activation
}

type UniVeth struct {
	OnuId uint32
	Veth  string
}

func (t *Tester) exeAAATest(ctx context.Context, s *Server, wait int) error {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	logger.Info("exeAAATest stands by....")
	infos, err := s.GetUniIoinfos("outside")
	if err != nil {
		return err
	}

	univeths := []UniVeth{}
	for _, info := range infos {
		uv := UniVeth{
			OnuId: info.onuid,
			Veth:  info.Name,
		}
		univeths = append(univeths, uv)
	}

	for sec := 1; sec <= wait; sec++ {
		select {
		case <-ctx.Done():
			logger.Debug("exeAAATest thread receives close ")
			return nil
		case <-tick.C:
			logger.WithField("seconds", wait-sec).Info("exeAAATest stands by ... ", wait-sec)
			if sec == wait {
				wg := sync.WaitGroup{}
				wg.Add(1)
				go func() error {
					defer wg.Done()
					err = activateWPASups(ctx, univeths, t.Intvl, s)
					if err != nil {
						return err
					}
					logger.Info("WPA supplicants are successfully activated ")
					t.Processes = append(t.Processes, "wpa_supplicant")
					logger.Debug("Running Process:%s", t.Processes)
					return nil
				}()
				wg.Wait()
			}
		}
	}
	return nil
}

func (t *Tester) exeDHCPTest(ctx context.Context, s *Server, wait int) error {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	logger.Info("exeDHCPTest stands by....")
	info, err := s.IdentifyNniIoinfo("outside")
	if err != nil {
		return err
	}

	err = activateDHCPServer(info.Name, t.DhcpServerIP)
	if err != nil {
		return err
	}
	t.Processes = append(t.Processes, "dhcpd")
	logger.Debug("Running Process:%s", t.Processes)

	infos, err := s.GetUniIoinfos("outside")
	if err != nil {
		return err
	}

	univeths := []UniVeth{}
	for _, info := range infos {
		uv := UniVeth{
			OnuId: info.onuid,
			Veth:  info.Name,
		}
		univeths = append(univeths, uv)
	}

	for sec := 1; sec <= wait; sec++ {
		select {
		case <-ctx.Done():
			logger.Debug("exeDHCPTest thread receives close ")
			return nil
		case <-tick.C:
			logger.WithField("seconds", wait-sec).Info("exeDHCPTest stands by ... ", wait-sec)
			if sec == wait {
				wg := sync.WaitGroup{}
				wg.Add(1)
				go func() error {
					defer wg.Done()
					err = activateDHCPClients(ctx, univeths, t.Intvl, s)
					if err != nil {
						return err
					}
					logger.WithFields(log.Fields{
						"univeths": univeths,
					}).Info("DHCP clients are successfully activated")
					t.Processes = append(t.Processes, "dhclient")
					logger.Debug("Running Process: ", t.Processes)
					return nil
				}()
				wg.Wait()
			}
		}
	}
	return nil
}

func KillProcesses(pnames []string) error {
	for _, pname := range pnames {
		killProcess(pname)
	}
	return nil
}

func activateWPASups(ctx context.Context, vethnames []UniVeth, intvl int, s *Server) error {
	tick := time.NewTicker(time.Duration(intvl) * time.Second)
	defer tick.Stop()
	i := 0
	for {
		select {
		case <-tick.C:
			if i < len(vethnames) {
				vethname := vethnames[i]
				if err := activateWPASupplicant(vethname, s); err != nil {
					return err
				}
				i++
			}
		case <-ctx.Done():
			logger.Debug("activateWPASups was canceled by context.")
			return nil
		}
	}
	return nil
}

func activateDHCPClients(ctx context.Context, vethnames []UniVeth, intvl int, s *Server) error {
	tick := time.NewTicker(time.Duration(intvl) * time.Second)
	defer tick.Stop()
	i := 0
	for {
		select {
		case <-tick.C:
			if i < len(vethnames) {
				vethname := vethnames[i]
				if err := activateDHCPClient(vethname, s); err != nil {
					return err
				}
				i++
			}
		case <-ctx.Done():
			logger.Debug("activateDHCPClients was canceled by context.")
			return nil
		}
	}
	return nil
}

func killProcess(name string) error {
	err := exec.Command("pkill", name).Run()
	if err != nil {
		logger.Error("Fail to pkill %s: %v", name, err)
		return err
	}
	logger.Info("Successfully killed %s", name)
	return nil
}

func activateWPASupplicant(univeth UniVeth, s *Server) (err error) {
	cmd := "/sbin/wpa_supplicant"
	conf := "/etc/wpa_supplicant/wpa_supplicant.conf"
	err = exec.Command(cmd, "-D", "wired", "-i", univeth.Veth, "-c", conf).Start()
	onu, _ := s.GetOnuByID(univeth.OnuId)
	if err != nil {
		utils.LoggerWithOnu(onu).WithFields(log.Fields{
			"err":  err,
			"veth": univeth.Veth,
		}).Error("Fail to activateWPASupplicant()", err)
		return
	}
	logger.Info("activateWPASupplicant() for :%s", univeth.Veth)
	return
}

func activateDHCPClient(univeth UniVeth, s *Server) (err error) {
	onu, _ := s.GetOnuByID(univeth.OnuId)

	cmd := exec.Command("/usr/local/bin/dhclient", univeth.Veth)
	if err := cmd.Start(); err != nil {
		logger.Error("Fail to activateDHCPClient() for: %s", univeth.Veth)
		logger.Panic("activateDHCPClient %s", err)
	}
	utils.LoggerWithOnu(onu).WithFields(log.Fields{
		"veth": univeth.Veth,
	}).Infof("activateDHCPClient() start for: %s", univeth.Veth)
	return
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
	cmd := "/usr/local/bin/dhcpd"
	conf := "/etc/dhcp/dhcpd.conf"
	err = exec.Command(cmd, "-cf", conf, veth).Run()
	if err != nil {
		logger.Error("Fail to activateDHCP Server (): %s", err)
		return err
	}
	logger.Info("DHCP Server is successfully activated !")
	return err
}
