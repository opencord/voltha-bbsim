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
	intvl_test  int
	Mode        Mode
	KafkaBroker string
}

func GetOptions() *option {
	o := new(option)
	addressport := flag.String("H", ":50060", "IP address:port")
	oltid := flag.Int("id", 0, "OLT-ID")
	npon := flag.Int("i", 1, "Number of PON-IF ports")
	nonus := flag.Int("n", 1, "Number of ONUs per PON-IF port")
	modeopt := flag.String("m", "default", "Emulation mode (default, aaa, both (aaa & dhcp))")
	aaawait := flag.Int("aw", 30, "Wait time (sec) for activation WPA supplicants")
	dhcpwait := flag.Int("dw", 50, "Wait time (sec) for activation DHCP clients")
	dhcpservip := flag.String("s", "182.21.0.1", "DHCP Server IP Address")
	intvl := flag.Int("v", 1, "Interval each Indication")
	intvl_test := flag.Int("V", 1, "Interval each Indication")
	kafkaBroker := flag.String("k", "", "Kafka broker")
	o.Mode = DEFAULT
	flag.Parse()
	if *modeopt == "aaa" {
		o.Mode = AAA
	} else if *modeopt == "both" {
		o.Mode = BOTH
	}
	o.oltid = uint32(*oltid)
	o.npon = uint32(*npon)
	o.nonus = uint32(*nonus)
	o.aaawait = *aaawait
	o.dhcpwait = *dhcpwait
	o.dhcpservip = *dhcpservip
	o.intvl = *intvl
	o.intvl_test = *intvl_test
	o.KafkaBroker = *kafkaBroker
	o.address = (strings.Split(*addressport, ":")[0])
	tmp, _ := strconv.Atoi(strings.Split(*addressport, ":")[1])
	o.port = uint32(tmp)
	return o
}

type stateMachine struct {
	handlers []*handler
	state    coreState
}

type handler struct {
	dst    coreState
	src    coreState
	method func(s *Server) error
}

func (sm *stateMachine) transit(next coreState) func(s *Server) error {
	for _, handler := range sm.handlers {
		if handler.src == sm.state && handler.dst == next {
			logger.Debug("Hit (src:%d, dst:%d)", handler.src, handler.dst)
			sm.state = next
			return handler.method
		}
	}
	sm.state = next
	return nil
}

type mediator struct {
	opt    *option
	sm     *stateMachine
	server *Server
	tester *Tester
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

	tester := NewTester(opt)
	m.server = server
	m.tester = tester
	m.sm = &stateMachine{
		state: INACTIVE,
		handlers: []*handler{
			&handler{src: PRE_ACTIVE, dst: ACTIVE, method: m.tester.Start},
			&handler{src: ACTIVE, dst: PRE_ACTIVE, method: m.tester.Stop},
		},
	}
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
			server.Stop()       //Non-blocking
			tester.Stop(server) //Non-blocking
			return
		}
	}()
	wg.Wait()
	logger.Debug("Reach to the end line")
}

func (m *mediator) Mediate() {
	wg := sync.WaitGroup{}
	defer logger.Debug("Mediate Done")
	for corestat := range m.server.stateChan {
		logger.Debug("Mediator receives state %d of server", corestat)
		method := m.sm.transit(corestat)
		if method != nil {
			wg.Add(1)
			defer wg.Done()
			go func() error {
				if err := method(m.server); err != nil { //blocking
					m.server.Stop()
					return err
				}
				return nil
			}()
		}
	}
	wg.Wait()
}
