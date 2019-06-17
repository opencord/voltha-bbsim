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

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"gerrit.opencord.org/voltha-bbsim/device"
	openolt "gerrit.opencord.org/voltha-bbsim/protos"
)

func sendOltIndUp(stream openolt.Openolt_EnableIndicationServer, olt *device.Olt) error {
	data := &openolt.Indication_OltInd{OltInd: &openolt.OltIndication{OperState: "up"}}
	if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
		logger.Error("Failed to send OLT UP indication: %v", err)
		return err
	}
	return nil
}

func sendOltIndDown(stream openolt.Openolt_EnableIndicationServer) error {
	data := &openolt.Indication_OltInd{OltInd: &openolt.OltIndication{OperState: "down"}}
	if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
		logger.Error("Failed to send OLT DOWN indication: %v", err)
		return err
	}
	return nil
}

func sendIntfInd(stream openolt.Openolt_EnableIndicationServer, olt *device.Olt) error {
	// There is no need to send IntfInd for NNI
	for i := uint32(0); i < olt.NumPonIntf; i++ {
		intf := olt.PonIntfs[i]
		data := &openolt.Indication_IntfInd{&openolt.IntfIndication{IntfId: intf.IntfID, OperState: intf.OperState}}
		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			logger.Error("Failed to send Intf [id: %d] indication : %v", i, err)
			return err
		}
		logger.Info("SendIntfInd olt:%d intf:%d (%s)", olt.ID, intf.IntfID, intf.Type)
	}
	return nil
}

func sendOperInd(stream openolt.Openolt_EnableIndicationServer, olt *device.Olt) error {
	// Send OperInd for Nni
	for i := uint32(0); i < olt.NumNniIntf; i++ {
		intf := olt.NniIntfs[i]
		data := &openolt.Indication_IntfOperInd{&openolt.IntfOperIndication{Type: intf.Type, IntfId: intf.IntfID, OperState: intf.OperState}}
		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			logger.Error("Failed to send NNI IntfOper [id: %d] indication : %v", i, err)
			return err
		}
		logger.Info("SendOperInd NNI olt:%d intf:%d (%s)", olt.ID, intf.IntfID, intf.Type)
	}

	// Send OperInd for Pon
	for i := uint32(0); i < olt.NumPonIntf; i++ {
		intf := olt.PonIntfs[i]
		data := &openolt.Indication_IntfOperInd{&openolt.IntfOperIndication{Type: intf.Type, IntfId: intf.IntfID, OperState: intf.OperState}}
		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			logger.Error("Failed to send PON IntfOper [id: %d] indication : %v", i, err)
			return err
		}
		logger.Info("SendOperInd PON olt:%d intf:%d (%s)", olt.ID, intf.IntfID, intf.Type)
	}
	return nil
}

func sendOnuDiscInd(stream openolt.Openolt_EnableIndicationServer, onu *device.Onu) error {
	data := &openolt.Indication_OnuDiscInd{OnuDiscInd: &openolt.OnuDiscIndication{IntfId: onu.IntfID, SerialNumber: onu.SerialNumber}}
	if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
		logger.Error("Failed to send ONUDiscInd [id: %d]: %v", onu.OnuID, err)
		return err
	}
	device.LoggerWithOnu(onu).Info("sendONUDiscInd Onuid")
	return nil
}

func sendOnuInd(stream openolt.Openolt_EnableIndicationServer, onu *device.Onu, delay int, operState string, adminState string) error {
	time.Sleep(time.Duration(delay) * time.Millisecond)
	data := &openolt.Indication_OnuInd{OnuInd: &openolt.OnuIndication{
		IntfId:       onu.IntfID,
		OnuId:        onu.OnuID,
		OperState:    operState,
		AdminState:   adminState,
		SerialNumber: onu.SerialNumber,
	}}
	if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
		logger.Error("Failed to send ONUInd [id: %d]: %v", onu.OnuID, err)
		return err
	}
	device.LoggerWithOnu(onu).Info("sendONUInd Onuid")
	return nil
}

func sendDyingGaspInd(stream openolt.Openolt_EnableIndicationServer, intfID uint32, onuID uint32) error {
	// Form DyingGasp Indication with ONU-ID and Intf-ID
	alarmData := &openolt.AlarmIndication_DyingGaspInd{DyingGaspInd: &openolt.DyingGaspIndication{IntfId: intfID, OnuId: onuID, Status: "on"}}
	data := &openolt.Indication_AlarmInd{AlarmInd: &openolt.AlarmIndication{Data: alarmData}}

	// Send Indication to VOLTHA
	if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
		logger.Error("Failed to send DyingGaspInd : %v", err)
		return err
	}
	logger.Info("sendDyingGaspInd Intf-ID:%d ONU-ID:%d", intfID, onuID)
	return nil
}

func startAlarmLoop(stream openolt.Openolt_EnableIndicationServer, alarmchan chan *openolt.Indication) {
	logger.Debug("SendAlarm() Invoked")
	for {
		select {
		case ind := <-alarmchan:
			logger.Debug("Alarm recieved at alarm-channel to send to voltha %+v", ind)
			err := stream.Send(ind)
			if err != nil {
				logger.Debug("Error occured while sending alarm indication %v", err)
			}
		}
	}
}
