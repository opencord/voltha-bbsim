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
	"flag"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/opencord/voltha-bbsim/common/logger"
	"github.com/opencord/voltha-bbsim/device"
	log "github.com/sirupsen/logrus"
)

const (
	DEFAULT Mode = iota
	AAA
	BOTH
)

// Store emulation mode
type Mode int

// AutoONUActivate is flag for Auto ONU Add on/off.
var AutoONUActivate int

type option struct {
	address                  string
	port                     uint32
	mgmtGrpcPort             uint32
	mgmtRestPort             uint32
	oltid                    uint32
	npon                     uint32
	nonus                    uint32
	aaawait                  int
	dhcpwait                 int
	dhcpservip               string
	intvl                    int
	interactiveOnuActivation bool
	Mode                     Mode
	KafkaBroker              string
	Debuglvl                 string
}

// GetOptions receives command line options and stores them in option structure
func GetOptions() *option {
	o := new(option)
	addressport := flag.String("H", ":50060", "IP address:port")
	oltid := flag.Int("id", 0, "OLT-ID")
	npon := flag.Int("i", 1, "Number of PON-IF ports")
	nonus := flag.Int("n", 1, "Number of ONUs per PON-IF port")
	modeopt := flag.String("m", "default", "Emulation mode (default, aaa, both (aaa & dhcp))")
	aaawait := flag.Int("aw", 2, "Wait time (sec) for activation WPA supplicants after EAPOL flow entry installed")
	dhcpwait := flag.Int("dw", 2, "Wait time (sec) for activation DHCP clients after DHCP flow entry installed")
	dhcpservip := flag.String("s", "182.21.0.128", "DHCP Server IP Address")
	intvl := flag.Int("v", 1000, "Interval each Indication (ms)")
	kafkaBroker := flag.String("k", "", "Kafka broker")
	interactiveOnuActivation := flag.Bool("ia", false, "Enable interactive activation of ONUs")
	mgmtGrpcPort := flag.Int("grpc", 50061, "BBSim API server gRPC port")
	mgmtRestPort := flag.Int("rest", 50062, "BBSim API server REST port")
	o.Mode = DEFAULT
	debg := flag.String("d", "DEBUG", "Debug Level(TRACE DEBUG INFO WARN ERROR)")
	flag.Parse()
	if *modeopt == "aaa" {
		o.Mode = AAA
	} else if *modeopt == "both" {
		o.Mode = BOTH
	}
	o.Debuglvl = *debg
	o.oltid = uint32(*oltid)
	o.npon = uint32(*npon)
	o.nonus = uint32(*nonus)
	o.aaawait = *aaawait
	o.dhcpwait = *dhcpwait
	o.dhcpservip = *dhcpservip
	o.intvl = *intvl
	o.interactiveOnuActivation = *interactiveOnuActivation
	o.KafkaBroker = *kafkaBroker
	o.address = strings.Split(*addressport, ":")[0]
	tmp, _ := strconv.Atoi(strings.Split(*addressport, ":")[1])
	o.port = uint32(tmp)
	o.mgmtGrpcPort = uint32(*mgmtGrpcPort)
	o.mgmtRestPort = uint32(*mgmtRestPort)

	if o.interactiveOnuActivation == true {
		log.Info("Automatic ONU activation disabled: use BBSim API to activate ONUs")
	}

	return o
}

type mediator struct {
	opt         *option
	server      *Server
	testmanager *TestManager
}

// NewMediator returns a new mediator object
func NewMediator(o *option) *mediator {
	m := new(mediator)
	m.opt = o
	logger.WithFields(log.Fields{
		"ip":        o.address,
		"baseport":  o.port,
		"pon_ports": o.npon,
		"onus":      o.nonus,
		"mode":      o.Mode,
	}).Debug("New mediator")
	return m
}

// Start mediator
func (m *mediator) Start() {
	var wg sync.WaitGroup
	opt := m.opt
	server := NewCore(opt)
	server.wg = &sync.WaitGroup{}
	server.wg.Add(1)
	go server.StartServerActionLoop(&wg)
	server.serverActionCh <- "start"
	go server.startMgmtServer(&wg)

	tm := NewTestManager(opt)
	m.server = server
	m.testmanager = tm
	go func() {
		m.Mediate()
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		defer func() {
			logger.Debug("SIGINT catcher Done")
			wg.Done()
		}()
		for sig := range c {
			wg.Add(1)
			logger.Debug("SIGINT %v", sig)
			close(c)
			server.Stop() // Non-blocking
			tm.Stop()     // Non-blocking
			server.stopMgmtServer()
			server.wg.Done()
			return
		}
	}()
	wg.Wait()
	server.wg.Wait()
}

// Mediate method is invoked on OLT and ONU state change
func (m *mediator) Mediate() {
	defer logger.Debug("Mediate Done")
	for sr := range m.server.stateRepCh {
		next := sr.next
		current := sr.current
		dev := sr.device
		key := dev.GetDevkey()
		if reflect.TypeOf(dev) == reflect.TypeOf(&device.Olt{}) {
			logger.WithFields(log.Fields{
				"device": dev,
			}).Debugf("Received OLT Device state change %v Current: %d Next: %d", key, current, next)
			if err := transitOlt(current, next, m.testmanager, m.opt); err != nil {
				logger.Error("%v", err)
			}
		} else if reflect.TypeOf(dev) == reflect.TypeOf(&device.Onu{}) {
			logger.WithFields(log.Fields{
				"device": dev,
			}).Debugf("Received ONU Device state change %v Current: %d Next: %d", key, current, next)
			if err := transitOnu(key, current, next, m.testmanager, m.opt); err != nil {
				logger.Error("%v", err)
			}
		}
	}
}

func transitOlt(current device.DeviceState, next device.DeviceState, tm *TestManager, o *option) error {
	logger.Debug("trnsitOlt called current:%d , next:%d", current, next)
	if current == device.OLT_PREACTIVE && next == device.OLT_ACTIVE {
		tm.Start()
		nniup, _ := makeNniName(o.oltid)
		activateDHCPServer(nniup, o.dhcpservip)
	} else if current == device.OLT_ACTIVE && next == device.OLT_PREACTIVE {
		tm.Stop()
	} else if current == device.OLT_ACTIVE && next == device.OLT_INACTIVE {
		// Reboot case
		// TODO Point of discussion
	}
	return nil
}

func transitOnu(key device.Devkey, previous device.DeviceState, current device.DeviceState, tm *TestManager, o *option) error {
	logger.Debug("transitOnu called with key: %v, previous: %s, current: %s", key, device.ONUState[previous], device.ONUState[current])
	if o.Mode == AAA || o.Mode == BOTH {
		if previous == device.ONU_ACTIVE && current == device.ONU_OMCIACTIVE {
			logger.Debug("Starting WPASupplicant for device %v", key)
			t := tm.CreateTester("AAA", o, key, activateWPASupplicant, o.aaawait)
			if err := tm.StartTester(t); err != nil {
				logger.Error("Cannot Start AAA Executer error:%v", err)
			}
		} else if previous == device.ONU_OMCIACTIVE && current == device.ONU_INACTIVE {
			if err := tm.StopTester("AAA", key); err != nil {
				logger.Error("Cannot Stop AAA Executer error:%v", err)
			}
		}
	}

	if o.Mode == BOTH {
		if previous == device.ONU_OMCIACTIVE && current == device.ONU_AUTHENTICATED {
			logger.Debug("Starting DHCP client for device %v", key)
			t := tm.CreateTester("DHCP", o, key, activateDHCPClient, o.dhcpwait)
			if err := tm.StartTester(t); err != nil {
				logger.Error("Cannot Start DHCP Executer error:%v", err)
			}
		} else if previous == device.ONU_AUTHENTICATED && current == device.ONU_INACTIVE {
			if err := tm.StopTester("DHCP", key); err != nil {
				logger.Error("Cannot Stop DHCP Executer error:%v", err)
			}
		}
	}
	return nil
}
