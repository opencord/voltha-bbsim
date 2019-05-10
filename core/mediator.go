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
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	log "github.com/sirupsen/logrus"
	"gerrit.opencord.org/voltha-bbsim/device"
	"reflect"
)

type option struct {
	address     string
	port        uint32
	oltid       uint32
	npon        uint32
	nonus       uint32
	aaawait     int
	dhcpwait    int
	dhcpservip  string
	intvl       int
	Mode        Mode
	KafkaBroker string
	Debuglvl	string
}

func GetOptions() *option {
	o := new(option)
	addressport := flag.String("H", ":50060", "IP address:port")
	oltid := flag.Int("id", 0, "OLT-ID")
	npon := flag.Int("i", 1, "Number of PON-IF ports")
	nonus := flag.Int("n", 1, "Number of ONUs per PON-IF port")
	modeopt := flag.String("m", "default", "Emulation mode (default, aaa, both (aaa & dhcp))")
	aaawait := flag.Int("aw", 10, "Wait time (sec) for activation WPA supplicants")
	dhcpwait := flag.Int("dw", 20, "Wait time (sec) for activation DHCP clients")
	dhcpservip := flag.String("s", "182.21.0.128", "DHCP Server IP Address")
	intvl := flag.Int("v", 1000, "Interval each Indication (ms)")
	kafkaBroker := flag.String("k", "", "Kafka broker")
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
	o.KafkaBroker = *kafkaBroker
	o.address = (strings.Split(*addressport, ":")[0])
	tmp, _ := strconv.Atoi(strings.Split(*addressport, ":")[1])
	o.port = uint32(tmp)
	return o
}

type mediator struct {
	opt         *option
	server      *Server
	testmanager *TestManager
}

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

func (m *mediator) Start() {
	var wg sync.WaitGroup
	opt := m.opt
	server := NewCore(opt)
	wg.Add(1)
	go func() {
		if err := server.Start(); err != nil { //Blocking
			logger.Error("Start %s", err)
		}
		wg.Done()
		return
	}()

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
			fmt.Println("SIGINT", sig)
			close(c)
			server.Stop() //Non-blocking
			tm.Stop()     //Non-blocking
			return
		}
	}()
	wg.Wait()
	logger.Debug("Reach to the end line")
}

func (m *mediator) Mediate() {
	defer logger.Debug("Mediate Done")
	for sr := range m.server.stateRepCh {
		next := sr.next
		current := sr.current
		dev := sr.device
		if reflect.TypeOf(dev) == reflect.TypeOf(&device.Olt{}){
			logger.Debug("Received OLT Device %v Current: %d Next: %d", dev, current, next)
			if err := transitOlt(current, next, m.testmanager, m.opt); err != nil {
				logger.Error("%v", err)
			}
		} else if reflect.TypeOf(dev) == reflect.TypeOf(&device.Onu{}) {
			logger.Debug("Received ONU Device %v Current: %d Next: %d", dev, current, next)
			key := dev.GetDevkey()
			if err := transitOnu(key, current, next, m.testmanager, m.opt); err != nil {
				logger.Error("%v", err)
			}
		}
	}
}

func transitOlt (current device.DeviceState, next device.DeviceState, tm *TestManager, o *option) error {
	logger.Debug("trnsitOlt called current:%d , next:%d", current, next)
	if current == device.OLT_PREACTIVE && next == device.OLT_ACTIVE {
		tm.Start()
		nniup, _ := makeNniName(o.oltid)
		activateDHCPServer(nniup, o.dhcpservip)
	} else if current == device.OLT_ACTIVE && next == device.OLT_PREACTIVE{
		tm.Stop()
	}
	return nil
}

func transitOnu (key device.Devkey, current device.DeviceState, next device.DeviceState, tm *TestManager, o *option) error {
	logger.Debug("trnsitOnu called with key: %v, current: %d, next: %d", key, current, next)
	if current == device.ONU_ACTIVE && next == device.ONU_OMCIACTIVE {
		t := tm.CreateTester(o, key)
		if err := tm.StartTester(key, t); err != nil {
			logger.Error("Cannot Start Executer error:%v", err)
		}
	} else if (current == device.ONU_OMCIACTIVE || current == device.ONU_ACTIVE) &&
		next == device.ONU_INACTIVE {
		if err := tm.StopTester(key); err != nil {
			logger.Error("Cannot Start Executer error:%v", err)
		}
	}
	return nil
}
