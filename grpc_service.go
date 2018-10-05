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

package main

import (
	"log"
  "gerrit.opencord.org/voltha-bbsim/openolt"
	"golang.org/x/net/context"
	"time"
)

// gRPC Service
func (s *server) DisableOlt(c context.Context, empty *openolt.Empty) (*openolt.Empty, error){
	log.Printf("OLT receives DisableOLT()\n")
	return new(openolt.Empty), nil
}

func (s *server) ReenableOlt(c context.Context, empty *openolt.Empty)(*openolt.Empty, error){
	log.Printf("OLT receives Reenable()\n")
	return new(openolt.Empty), nil
}

func (s *server) ActivateOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error){
	log.Printf("OLT receives ActivateONU()\n")
	result := validateONU(*onu, s.onus)
	if result == true {
		log.Printf("ONU %d activated succesufully.\n", onu.OnuId)
		s.onus[onu.IntfId][onu.OnuId].internalState = ONU_ACTIVATED
		//log.Printf("ActivateOnu(): %v", s.onus)
	}
	return new(openolt.Empty), nil
}

func (s *server) DeactivateOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error){
	log.Printf("OLT receives DeactivateONU()\n")
	return new(openolt.Empty), nil
}

func (s *server) DeleteOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error){
	log.Printf("OLT receives DeleteONU()\n")
	return new(openolt.Empty), nil
}

func (s *server)OmciMsgOut(c context.Context, msg *openolt.OmciMsg)(*openolt.Empty, error){
	//log.Printf("OLT %d receives OmciMsgOut to IF %v (ONU-ID: %v): %v.\n", s.olt.ID, msg.IntfId, msg.OnuId, msg.Pkt)
	log.Printf("OLT %d receives OmciMsgOut to IF %v (ONU-ID: %v).\n", s.olt.ID, msg.IntfId, msg.OnuId)
	return new(openolt.Empty), nil
}

func (s *server) OnuPacketOut(c context.Context, packet *openolt.OnuPacket)(*openolt.Empty, error){
	log.Printf("OLT %d receives OnuPacketOut ().\n", s.olt.ID)
	return new(openolt.Empty), nil
}

func (s *server) UplinkPacketOut(c context.Context, packet *openolt.UplinkPacket)(*openolt.Empty, error){
	log.Printf("OLT %d receives UplinkPacketOut().\n", s.olt.ID)
	return new(openolt.Empty), nil
}

func (s *server) FlowAdd(c context.Context, flow *openolt.Flow)(*openolt.Empty, error){
	log.Printf("OLT %d receives FlowAdd().\n", s.olt.ID)
	return new(openolt.Empty), nil
}

func (s *server) HeartbeatCheck(c context.Context, empty *openolt.Empty) (*openolt.Heartbeat, error){
	log.Printf("OLT %d receives HeartbeatCheck().\n", s.olt.ID)
	signature := new(openolt.Heartbeat)
	signature.HeartbeatSignature = s.olt.HeartbeatSignature
	return signature, nil
}

func (s *server) EnablePonIf(c context.Context, intf *openolt.Interface) (*openolt.Empty, error){
	log.Printf("OLT %d receives EnablePonIf().\n", s.olt.ID)
	return new(openolt.Empty), nil
}

func (s *server) DisablePonIf(c context.Context, intf *openolt.Interface) (*openolt.Empty, error){
	log.Printf("OLT %d receives DisablePonIf().\n", s.olt.ID)
	return new(openolt.Empty), nil
}

func (s *server) Reboot(c context.Context, empty *openolt.Empty,) (*openolt.Empty, error){
	log.Printf("OLT %d receives Reboot ().\n", s.olt.ID)
	return new(openolt.Empty), nil
}

func (s *server) EnableIndication(empty *openolt.Empty, stream openolt.Openolt_EnableIndicationServer) error {
	log.Printf("OLT receives EnableInd.\n")
	if err := activateOLT(s, stream); err != nil {
		log.Printf("Failed to activate OLT: %v\n", err)
		return err
	}
	// Infinite loop TODO: This should be a queue processing.
	for {
		time.Sleep(1 * time.Second)
	}
	return nil
}
