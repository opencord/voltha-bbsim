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
	"gerrit.opencord.org/voltha-bbsim/common"
	"github.com/google/gopacket/pcap"
	"os/exec"
)

type Ioinfo struct {
	Name    string
	iotype  string //nni or uni
	ioloc   string //inside or outsode
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
	logger.Error("%s", err)
	return nil, err
}

func (s *Server) IdentifyNniIoinfo(ioloc string) (*Ioinfo, error) {
	for _, ioinfo := range s.Ioinfos {
		if ioinfo.iotype == "nni" && ioinfo.ioloc == ioloc {
			return ioinfo, nil
		}
	}
	err := errors.New("No matched Ioinfo is found")
	logger.Error("%s", err)
	return nil, err
}

func (s *Server) GetUniIoinfos(ioloc string) ([]*Ioinfo, error) {
	ioinfos := []*Ioinfo{}
	for _, ioinfo := range s.Ioinfos {
		if ioinfo.iotype == "uni" && ioinfo.ioloc == ioloc {
			ioinfos = append(ioinfos, ioinfo)
		}
	}
	if len(ioinfos) == 0 {
		err := errors.New("No matched Ioinfo is found")
		logger.Error("%s", err)
		return nil, err
	}
	return ioinfos, nil
}

func CreateVethPairs(name1 string, name2 string) (err error) {
	err = exec.Command("ip", "link", "add", name1, "type", "veth", "peer", "name", name2).Run()
	if err != nil {
		logger.Error("Fail to createVeth() for %s and %s veth creation error: %s\n", name1, name2, err.Error())
		return
	}
	logger.Info("%s & %s was created.", name1, name2)
	err = exec.Command("ip", "link", "set", name1, "up").Run()
	if err != nil {
		logger.Error("Fail to createVeth() veth1 up", err)
		return
	}
	err = exec.Command("ip", "link", "set", name2, "up").Run()
	if err != nil {
		logger.Error("Fail to createVeth() veth2 up", err)
		return
	}
	logger.Info("%s & %s was up.", name1, name2)
	return
}

func RemoveVeth(name string) error {
	err := exec.Command("ip", "link", "del", name).Run()
	if err != nil {
		logger.Error("Fail to removeVeth()", err)
	}
	logger.Info("%s was removed.", name)
	return err
}

func RemoveVeths(names []string) {
	for _, name := range names {
		RemoveVeth(name)
	}
	logger.Info("RemoveVeths() :%s\n", names)
	return
}
