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
	"github.com/google/gopacket/pcap"
	"gerrit.opencord.org/voltha-bbsim/common"
	"errors"
)

type Ioinfo struct {
	name    string
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

func (s *Server) identifyNniIoinfo(ioloc string) (*Ioinfo, error) {
	for _, ioinfo := range s.Ioinfos {
		if ioinfo.iotype == "nni" && ioinfo.ioloc == ioloc {
			return ioinfo, nil
		}
	}
	err := errors.New("No matched Ioinfo is found")
	logger.Error("%s", err)
	return nil, err
}

func (s *Server) getUniIoinfos(ioloc string) ([]*Ioinfo, error) {
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
