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

	omci "github.com/opencord/omci-sim"
	"github.com/opencord/voltha-bbsim/common/logger"
	openolt "github.com/opencord/voltha-protos/go/openolt"
)

// RunOmciResponder starts a go routine to process/respond to OMCI messages from VOLTHA
func (s *Server) RunOmciResponder(ctx context.Context, omciOut chan openolt.OmciMsg, omciIn chan openolt.OmciIndication, errch chan error) {
	go func() {
		defer logger.Debug("Omci response process was done")

		var resp openolt.OmciIndication

		for {
			select {
			case m := <-omciOut:
				respPkt, err := omci.OmciSim(m.IntfId, m.OnuId, HexDecode(m.Pkt))
				switch err := err.(type) {
				case nil:
					// Success
					resp.IntfId = m.IntfId
					resp.OnuId = m.OnuId
					resp.Pkt = respPkt
					omciIn <- resp
					s.handleOmciAction(resp.Pkt, resp.IntfId, resp.OnuId)

				case *omci.OmciError:
					// Error in processing omci message. Log and carry on.
					logger.Debug("%s", err.Msg)
					continue
				default:
					// Fatal error, exit.
					errch <- err
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// HexDecode converts the hex encoding to binary
func HexDecode(pkt []byte) []byte {
	// TODO - Change openolt adapter to send raw binary instead of hex encoded
	p := make([]byte, len(pkt)/2)
	for i, j := 0, 0; i < len(pkt); i, j = i+2, j+1 {
		// Go figure this ;)
		u := (pkt[i] & 15) + (pkt[i]>>6)*9
		l := (pkt[i+1] & 15) + (pkt[i+1]>>6)*9
		p[j] = u<<4 + l
	}
	logger.Debug("Omci decoded: %x.", p)
	return p
}

func (s *Server) handleOmciAction(pkt []byte, IntfID uint32, OnuID uint32) {
	logger.Debug("handleOmciAction invoked")
	MEClass := omci.OmciClass(uint16(pkt[5]) | uint16(pkt[4])<<8)
	msgType := omci.OmciMsgType(pkt[2] & 0x1F)
	logger.Debug("ME Class %d, msgType %d", MEClass, msgType)

	if MEClass == omci.ONUG {
		switch msgType {
		case omci.Reboot:
			logger.Info("ONU reboot recieved")
			s.handleONUSoftReboot(IntfID, OnuID)
		}
	} else if MEClass == omci.GEMPortNetworkCTP {
		switch msgType {
		case omci.Create:
			logger.Info("GEMPort created")
			gemport, err := omci.GetGemPortId(IntfID, OnuID)
			if err != nil {
				logger.Error("error in getting gemport %v", err)
				return
			}
			logger.Info("GEM Port %d created at ONU %d intf-id %d", gemport, OnuID, IntfID)
		}
	}
}
