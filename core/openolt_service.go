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
	"gerrit.opencord.org/voltha-bbsim/common"
	"gerrit.opencord.org/voltha-bbsim/device"
	"gerrit.opencord.org/voltha-bbsim/protos"
	"time"
)

func sendOltIndUp(stream openolt.Openolt_EnableIndicationServer, olt *device.Olt) error {
	data := &openolt.Indication_OltInd{OltInd: &openolt.OltIndication{OperState: "up"}}
	if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
		logger.Error("Failed to send OLT UP indication: %v\n", err)
		return err
	}
	return nil
}

func sendOltIndDown(stream openolt.Openolt_EnableIndicationServer) error {
	data := &openolt.Indication_OltInd{OltInd: &openolt.OltIndication{OperState: "down"}}
	if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
		logger.Error("Failed to send OLT DOWN indication: %v\n", err)
		return err
	}
	return nil
}

func sendIntfInd(stream openolt.Openolt_EnableIndicationServer, olt *device.Olt) error {
	for i := uint32(0); i < olt.NumPonIntf+olt.NumNniIntf; i++ {
		intf := olt.Intfs[i]
		if intf.Type == "pon" { // There is no need to send IntfInd for NNI
			data := &openolt.Indication_IntfInd{&openolt.IntfIndication{IntfId: intf.IntfID, OperState: intf.OperState}}
			if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
				logger.Error("Failed to send Intf [id: %d] indication : %v\n", i, err)
				return err
			}
			logger.Info("SendIntfInd olt:%d intf:%d (%s)\n", olt.ID, intf.IntfID, intf.Type)
		}
	}
	return nil
}

func sendOperInd(stream openolt.Openolt_EnableIndicationServer, olt *device.Olt) error {
	for i := uint32(0); i < olt.NumPonIntf+olt.NumNniIntf; i++ {
		intf := olt.Intfs[i]
		data := &openolt.Indication_IntfOperInd{&openolt.IntfOperIndication{Type: intf.Type, IntfId: intf.IntfID, OperState: intf.OperState}}
		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			logger.Error("Failed to send IntfOper [id: %d] indication : %v\n", i, err)
			return err
		}
		logger.Info("SendOperInd olt:%d intf:%d (%s)\n", olt.ID, intf.IntfID, intf.Type)
	}
	return nil
}

func sendOnuDiscInd(stream openolt.Openolt_EnableIndicationServer, onus []*device.Onu) error {
	for i, onu := range onus {
		data := &openolt.Indication_OnuDiscInd{&openolt.OnuDiscIndication{IntfId: onu.IntfID, SerialNumber: onu.SerialNumber}}
		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			logger.Error("Failed to send ONUDiscInd [id: %d]: %v\n", i, err)
			return err
		}
		logger.Info("sendONUDiscInd Onuid: %d\n", i)
	}
	return nil
}

func sendOnuInd(stream openolt.Openolt_EnableIndicationServer, onus []*device.Onu, delay int) error {
	for i, onu := range onus {
		time.Sleep(time.Duration(delay) * time.Second)
		data := &openolt.Indication_OnuInd{&openolt.OnuIndication{IntfId: onu.IntfID, OnuId: onu.OnuID, OperState: "up", AdminState: "up", SerialNumber: onu.SerialNumber}}
		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			logger.Error("Failed to send ONUInd [id: %d]: %v\n", i, err)
			return err
		}
		logger.Info("sendONUInd Onuid: %d\n", i)
	}
	return nil
}
