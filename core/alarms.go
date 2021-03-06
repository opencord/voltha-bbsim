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
	"strconv"

	api "github.com/opencord/voltha-bbsim/api"
	"github.com/opencord/voltha-bbsim/common/logger"
	"github.com/opencord/voltha-bbsim/device"
	flowHandler "github.com/opencord/voltha-bbsim/flow"
	openolt "github.com/opencord/voltha-protos/go/openolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// OnuLossOfPloam is state on onu los
	OnuLossOfPloam = "lossofploam"
	// OnuLossOfOmciChannel is the state on omci channel loss alarm
	OnuLossOfOmciChannel = "lossofomcichannel"
	// OnuSignalDegrade is the state on signal degrade alarm
	OnuSignalDegrade = "signaldegrade"
	// AlarmOn is for raising the alarm
	AlarmOn = "on"
	// AlarmOff is for clearing the alarm
	AlarmOff = "off"
)

func (s *Server) handleOnuAlarm(in *api.ONUAlarmRequest) (*api.BBSimResponse, error) {
	logger.Trace("handleOnuAlarm() invoked")
	value, ok := s.SNmap.Load(in.OnuSerial)
	if !ok {
		return &api.BBSimResponse{}, status.Errorf(codes.NotFound, "no active or discovered onu found with serial number "+in.OnuSerial)
	}

	onu := value.(*device.Onu)
	if (onu.InternalState == device.OnuLosRaised || onu.InternalState == device.OnuLosOnOltPonLos) &&
		(in.AlarmType != OnuLossOfPloam) {
		return &api.BBSimResponse{}, status.Errorf(codes.Aborted, in.OnuSerial+" is not reachable, can not send onu alarm")
	}

	if s.Olt.PonIntfs[onu.IntfID].AlarmState == device.PonLosRaised && (in.AlarmType != OnuLossOfPloam) {
		// Don't send onu alarm as OLT-PON is down
		return &api.BBSimResponse{}, status.Errorf(codes.Aborted, "pon-port down, can not send onu alarm")
	}
	switch in.AlarmType {
	case OnuLossOfOmciChannel:
		Ind := formulateLossOfOmciChannelAlarm(in.Status, onu)
		if in.Status == AlarmOn {
			onu.UpdateIntState(device.OnuOmciChannelLosRaised)
		} else {
			onu.UpdateIntState(device.OnuActive)
		}
		s.alarmCh <- Ind
		return &api.BBSimResponse{StatusMsg: RequestAccepted}, nil

	case OnuSignalDegrade:
		Ind := formulateSignalDegradeAlarm(in.Status, onu)
		s.alarmCh <- Ind
		return &api.BBSimResponse{StatusMsg: RequestAccepted}, nil

	case OnuLossOfPloam:
		if in.Status == AlarmOn {
			onu.UpdateIntState(device.OnuLosRaised)
			device.UpdateOnusOpStatus(onu.IntfID, onu, "down")
		} else {
			onu.UpdateIntState(device.OnuActive)
			device.UpdateOnusOpStatus(onu.IntfID, onu, "up")
			err := sendOnuDiscInd(*s.EnableServer, onu)
			if err != nil {
				logger.Error("Error: %s", err.Error())
			}
		}
		Ind := formulateLossOfPLOAM(in.Status, onu)
		s.alarmCh <- Ind
		er := sendOnuInd(*s.EnableServer, onu, onu.OperState, "up")
		if er != nil {
			logger.Debug(er.Error())
		}

		resp, err := s.checkAndSendOltPonLos(in.OnuSerial, in.Status, device.IntfPon) // Send olt los if all the onus attached to a pon-port shows los
		if err != nil {
			return resp, err
		}
		return resp, nil

	default:
		logger.Debug("Unhandled alarm type")
		return &api.BBSimResponse{}, status.Errorf(codes.Unimplemented, "Unhandled alarm type")
	}

}

func (s *Server) handleOltAlarm(in *api.OLTAlarmRequest) (*api.BBSimResponse, error) {
	logger.Trace("handleOltAlarm() invoked")
	switch in.PortType {
	case device.IntfNni:

		if !s.isNniIntfPresentInOlt(in.PortId) {
			return &api.BBSimResponse{}, status.Errorf(codes.NotFound, strconv.Itoa(int(in.PortId))+" NNI not present in olt")
		}

		Ind := formulateOLTLOSAlarm(in.Status, in.PortId, device.IntfNni)
		s.alarmCh <- Ind
		s.setNNIPortState(in.PortId, in.Status)

	case device.IntfPon:
		if !s.isPonIntfPresentInOlt(in.PortId) {
			return &api.BBSimResponse{}, status.Errorf(codes.NotFound, strconv.Itoa(int(in.PortId))+" PON not present in olt")
		}
		Ind := formulateOLTLOSAlarm(in.Status, in.PortId, in.PortType)
		s.alarmCh <- Ind
		onusOperstat := s.setPONPortState(in.PortId, in.Status)
		for _, onu := range s.Onumap[in.PortId] {
			if onu.InternalState == device.OnuLosRaised || onu.InternalState == device.OnuFree {
				continue // Skip for onus which have independently raised onu los
			}

			er := sendOnuInd(*s.EnableServer, onu, onusOperstat, "up")
			if er != nil {
				logger.Debug(er.Error())
			}
			s.sendOnuLosOnOltPonLos(onu, in.Status)
		}
	default:
		return &api.BBSimResponse{}, status.Errorf(codes.InvalidArgument, "invalid interface type provided")
	}

	return &api.BBSimResponse{StatusMsg: RequestAccepted}, nil
}

func (s *Server) setNNIPortState(portID uint32, alarmstatus string) {
	logger.Trace("setNNIPortState() invoked")
	switch alarmstatus {
	case AlarmOn:
		s.Olt.UpdateNniPortState(portID, device.NniLosRaised, "down")
		err := flowHandler.PortDown(0)
		if err != nil {
			logger.Error("Failed in port down %v", err)
		}

	case AlarmOff:
		s.Olt.UpdateNniPortState(portID, device.NniLosCleared, "up")
		err := flowHandler.PortUp(0)
		if err != nil {
			logger.Error("Failed in port up %v", err)
		}
	}
}

func (s *Server) setPONPortState(portID uint32, alarmstatus string) string {
	logger.Trace("setPONPortState() invoked")
	switch alarmstatus {
	case AlarmOn:
		s.Olt.UpdatePonPortState(portID, device.PonLosRaised, "down")
		return "down"

	case AlarmOff:
		s.Olt.UpdatePonPortState(portID, device.PonLosCleared, "up")
		return "up"
	}
	return ""
}

func (s *Server) sendOnuLosOnOltPonLos(onu *device.Onu, status string) {
	logger.Trace("sendOnuLosOnOltPonLos() invoked")
	var internalState device.State

	if status == AlarmOn {
		internalState = device.OnuLosOnOltPonLos
	} else if status == AlarmOff {
		internalState = device.OnuActive
	}

	Ind := formulateLossOfPLOAM(status, onu)
	onu.UpdateIntState(internalState)

	// update onus slice on alarm off
	if status == "off" {
		err := sendOnuDiscInd(*s.EnableServer, onu)
		if err != nil {
			logger.Error(err.Error())
		}
	}

	s.alarmCh <- Ind
}

func formulateLossOfOmciChannelAlarm(status string, onu *device.Onu) *openolt.Indication {
	logger.Trace("formulateLossofOmciChannelAlarm() invoked")

	alarmIndication := &openolt.AlarmIndication_OnuLossOmciInd{
		OnuLossOmciInd: &openolt.OnuLossOfOmciChannelIndication{
			IntfId: onu.IntfID,
			OnuId:  onu.OnuID,
			Status: status,
		},
	}

	alarmind := &openolt.AlarmIndication{
		Data: alarmIndication,
	}

	msg := &openolt.Indication_AlarmInd{AlarmInd: alarmind}
	Ind := &openolt.Indication{Data: msg}
	return Ind
}

func formulateSignalDegradeAlarm(status string, onu *device.Onu) *openolt.Indication {
	logger.Trace("formulateSignalDegrade() invoked")
	alarmIndication := &openolt.AlarmIndication_OnuSignalDegradeInd{
		OnuSignalDegradeInd: &openolt.OnuSignalDegradeIndication{
			IntfId:              onu.IntfID,
			OnuId:               onu.OnuID,
			Status:              status,
			InverseBitErrorRate: 0,
		},
	}
	alarmind := &openolt.AlarmIndication{
		Data: alarmIndication,
	}
	msg := &openolt.Indication_AlarmInd{AlarmInd: alarmind}
	Ind := &openolt.Indication{Data: msg}
	return Ind
}

func formulateLossOfPLOAM(status string, onu *device.Onu) *openolt.Indication {
	logger.Trace("formulateLossOfPLOAM() invoked")

	alarmIndication := &openolt.AlarmIndication_OnuAlarmInd{OnuAlarmInd: &openolt.OnuAlarmIndication{
		IntfId:             onu.IntfID,
		OnuId:              onu.OnuID,
		LosStatus:          status,
		LobStatus:          status,
		LopcMissStatus:     status,
		LopcMicErrorStatus: status,
	}}

	alarmind := &openolt.AlarmIndication{Data: alarmIndication}
	msg := &openolt.Indication_AlarmInd{AlarmInd: alarmind}
	Ind := &openolt.Indication{Data: msg}
	return Ind
}

func formulateOLTLOSAlarm(status string, PortID uint32, intfType string) *openolt.Indication {
	intfID := interfaceIDToPortNo(PortID, intfType)

	alarmIndication := &openolt.AlarmIndication_LosInd{LosInd: &openolt.LosIndication{
		IntfId: intfID,
		Status: status,
	}}

	alarmind := &openolt.AlarmIndication{Data: alarmIndication}
	msg := &openolt.Indication_AlarmInd{AlarmInd: alarmind}
	Ind := &openolt.Indication{Data: msg}
	return Ind
}

func (s *Server) checkAndSendOltPonLos(serial string, status string, intfType string) (*api.BBSimResponse, error) {
	value, ok := s.SNmap.Load(serial)
	if !ok {
		logger.Debug(serial + " not found in OLT-" + strconv.Itoa(int(s.Olt.ID)))
		return &api.BBSimResponse{}, nil
	}

	onu := value.(*device.Onu)
	if s.getNoOfActiveOnuByPortID(onu.IntfID) == 0 {
		logger.Warn("Warning: Sending OLT-LOS, as all onus on pon-port %v raised los", onu.IntfID)
		request := &api.OLTAlarmRequest{PortId: onu.IntfID, Status: AlarmOn, PortType: device.IntfPon}
		resp, err := s.handleOltAlarm(request)
		return resp, err
	}
	if s.Olt.PonIntfs[onu.IntfID].AlarmState == device.PonLosRaised && status == AlarmOff {
		s.setPONPortState(onu.IntfID, status)
		Ind := formulateOLTLOSAlarm(status, onu.IntfID, intfType)
		s.alarmCh <- Ind
	}

	return &api.BBSimResponse{StatusMsg: RequestAccepted}, nil
}

func interfaceIDToPortNo(intfid uint32, intfType string) uint32 {
	logger.Trace("interfaceIDToPortNo() invoked")
	// Converts interface-id to port-numbers that can be understood by the VOLTHA
	if intfType == device.IntfNni {
		// nni at voltha starts with 65536
		// nni = 65536 + interface_id
		return 0x1<<16 + intfid
	} else if intfType == device.IntfPon {
		// pon = 536,870,912 + interface_id
		return (0x2 << 28) + intfid // In openolt code, stats_collection.cc line number 196, pon starts from 0
		// In bbsim, pon starts from 1
	}
	return 0
}
