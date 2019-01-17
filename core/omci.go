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
	"time"

	"context"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"gerrit.opencord.org/voltha-bbsim/device"
	"gerrit.opencord.org/voltha-bbsim/protos"
	omci "github.com/opencord/omci-sim"
)

func RunOmciResponder(ctx context.Context, omciOut chan openolt.OmciMsg, omciIn chan openolt.OmciIndication, onumap map[uint32][]*device.Onu, errch chan error) {
	go func() { //For monitoring the OMCI states TODO: This part should be eliminated because it is out of scope of this library
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				logger.Debug("Monitor omci init state")
				if isAllOmciInitDone(onumap) {
					logger.Info("OmciRun - All the omci initialization wes done")
					close(errch)
					return
				}
			case <-ctx.Done():
				logger.Debug("Omci Monitoring process was done")
				return
			}
		}
	}()

	go func() {
		defer logger.Debug("Omci response process was done")

		var resp openolt.OmciIndication

		for {
			select {
			case m := <-omciOut:
				resp_pkt, err := omci.OmciSim(m.IntfId, m.OnuId, HexDecode(m.Pkt))
				switch err := err.(type) {
				case nil:
					// Success
					resp.IntfId = m.IntfId
					resp.OnuId = m.OnuId
					resp.Pkt = resp_pkt
					omciIn <- resp
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

func HexDecode(pkt []byte) []byte {
	// Convert the hex encoding to binary
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

func isAllOmciInitDone(onumap map[uint32][]*device.Onu) bool {
	for _, onus := range onumap {
		for _, onu := range onus {
			if omci.GetOnuOmciState(onu.OnuID, onu.IntfID) == omci.INCOMPLETE {
				return false
			}
		}
	}
	return true
}
