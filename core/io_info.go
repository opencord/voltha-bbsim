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
	"errors"
	"os/exec"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
)

// Ioinfo represents the input/output
type Ioinfo struct {
	Name    string
	iotype  string // nni or uni
	ioloc   string // inside or outside
	intfid  uint32
	onuid   uint32
	handler *pcap.Handle
}

func (s *Server) identifyUniIoinfo(ioloc string, intfid uint32, onuid uint32) (*Ioinfo, error) {
	for _, ioinfo := range s.Ioinfos {
		if ioinfo.iotype == "uni" && ioinfo.intfid == intfid && ioinfo.onuid == onuid && ioinfo.ioloc == ioloc {
			return ioinfo, nil
		}
	}
	err := errors.New("No matched Ioinfo is found")
	logger.Error("identifyUniIoinfo %s", err)
	return nil, err
}

// IdentifyNniIoinfo returns matched ioinfo
func (s *Server) IdentifyNniIoinfo(ioloc string) (*Ioinfo, error) {
	for _, ioinfo := range s.Ioinfos {
		if ioinfo.iotype == "nni" && ioinfo.ioloc == ioloc {
			return ioinfo, nil
		}
	}
	err := errors.New("No matched Ioinfo is found")
	logger.Error("IdentifyNniIoinfo %s", err)
	return nil, err
}

// CreateVethPairs creates veth pairs with given names
func CreateVethPairs(veth1 string, veth2 string) (err error) {
	err = exec.Command("ip", "link", "add", veth1, "type", "veth", "peer", "name", veth2).Run()
	if err != nil {
		logger.WithFields(log.Fields{
			"veth1": veth1,
			"veth2": veth2,
		}).Error("Fail to createVethPair()", err.Error())
		return
	}
	logger.Info("%s & %s was created.", veth1, veth2)
	err = exec.Command("ip", "link", "set", veth1, "up").Run()
	if err != nil {
		logger.Error("Fail to createVeth() veth1 up: %v", err)
		return
	}
	err = exec.Command("ip", "link", "set", veth2, "up").Run()
	if err != nil {
		logger.Error("Fail to createVeth() veth2 up: %v", err)
		return
	}
	logger.Info("%s & %s was up.", veth1, veth2)
	return
}

// RemoveVeth deletes veth by given name
func RemoveVeth(name string) error {
	err := exec.Command("ip", "link", "del", name).Run()
	if err != nil {
		logger.WithField("veth", name).Error("Fail to removeVeth()", err)
	}
	logger.WithField("veth", name).Info("Veth was removed.")
	return err
}

// RemoveVeths deletes veth
func RemoveVeths(names []string) {
	for _, name := range names {
		RemoveVeth(name)
	}
	logger.WithField("veths", names).Info("RemoveVeths(): ")
	return
}
