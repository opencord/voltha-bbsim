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
	"net"
	"strconv"
	"time"

	pb "github.com/opencord/voltha-bbsim/api"
	"github.com/opencord/voltha-bbsim/common/logger"
	"github.com/opencord/voltha-bbsim/device"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// handleONUActivate process ONU status request
func (s *Server) handleONUStatusRequest(in *pb.ONUInfo) (*pb.ONUs, error) {
	onuInfo := &pb.ONUs{}
	if in.OnuSerial != "" { // Get status of single ONU by SerialNumber
		// Get OpenOlt serial number from string
		sn, err := getOpenoltSerialNumber(in.OnuSerial)
		if err != nil {
			logger.Error("Invalid serial number %s", in.OnuSerial)
			return onuInfo, status.Errorf(codes.InvalidArgument, "serial: "+in.OnuSerial+" is invalid")
		}
		// Get ONU by serial number
		onu, err := getOnuBySN(s.Onumap, sn)
		if err != nil {
			logger.Error("ONU with serial number %v not found", sn)
			return onuInfo, status.Errorf(codes.NotFound, "serial: "+in.OnuSerial+" not found")
		}
		onuInfo.Onus = append(onuInfo.Onus, copyONUInfo(onu))
	} else {
		// Return error if specified PON port does not exist
		if _, exist := s.Onumap[in.PonPortId]; !exist {
			logger.Error("PON port %d not found", in.PonPortId)
			return onuInfo, status.Errorf(codes.NotFound, "PON Port: "+strconv.Itoa(int(in.PonPortId))+" not found")
		}

		if in.OnuId != 0 { // Get status of single ONU by ONU-ID
			for intfid := range s.Onumap {
				for _, onu := range s.Onumap[intfid] {
					if in.OnuId == onu.OnuID {
						onuInfo.Onus = append(onuInfo.Onus, copyONUInfo(onu))
					}
				}
			}
		} else {
			// Append ONU data
			for _, onu := range s.Onumap[in.PonPortId] {
				onuInfo.Onus = append(onuInfo.Onus, copyONUInfo(onu))
			}
		}
	}

	return onuInfo, nil
}

// handleONUActivate method handles ONU activate requests from user.
func (s *Server) handleONUActivate(in []*pb.ONUInfo) (*pb.BBSimResponse, error) {
	logger.Info("handleONUActivate request received")
	logger.Debug("Received values: %+v\n", in)

	// Check if indication is enabled
	if s.EnableServer == nil {
		logger.Error(OLTNotEnabled)
		return &pb.BBSimResponse{}, status.Errorf(codes.FailedPrecondition, OLTNotEnabled)
	}

	onuaddmap := make(map[uint32][]*device.Onu)
	var newSerialNums []string

	//Iterate request for each PON port specified
	for _, onu := range in {
		intfid := onu.PonPortId

		// Get the free ONU object for the intfid
		Onu, err := s.GetNextFreeOnu(intfid)
		if err != nil {
			markONUsFree(onuaddmap)
			logger.Error("Failed to get free ONU object for intfID %d :%v", intfid, err)
			return &pb.BBSimResponse{}, status.Errorf(codes.ResourceExhausted, err.Error())
		}

		// Check if Serial number is provided by user
		if onu.OnuSerial != "" {
			// Get OpenOlt serial number from string
			sn, err := getOpenoltSerialNumber(onu.OnuSerial)
			if err != nil {
				logger.Error("Failed to get OpenOlt serial number %v", err)
				Onu.InternalState = device.ONU_FREE
				markONUsFree(onuaddmap)
				return &pb.BBSimResponse{}, status.Errorf(codes.InvalidArgument, "serial number: "+onu.OnuSerial+" is invalid")
			}

			// Check if serial number is not duplicate in requested ONUs
			for _, sn := range newSerialNums {
				if onu.OnuSerial == sn {
					logger.Error("Duplicate serial number found %s", sn)
					// Mark ONUs free
					markONUsFree(onuaddmap)
					Onu.InternalState = device.ONU_FREE
					return &pb.BBSimResponse{}, status.Errorf(codes.InvalidArgument, "duplicate serial number: "+onu.OnuSerial+" provided")
				}
			}
			newSerialNums = append(newSerialNums, onu.OnuSerial)

			// Check if serial number already exist
			_, exist := s.getOnuFromSNmap(sn)
			if exist {
				logger.Error("Provided serial number %v already exist", sn)
				// Mark ONUs free
				markONUsFree(onuaddmap)
				Onu.InternalState = device.ONU_FREE
				return &pb.BBSimResponse{}, status.Errorf(codes.AlreadyExists, "serial number: "+onu.OnuSerial+" already exist")
			}

			// Store user provided serial number in ONU object
			Onu.SerialNumber = sn
		}
		// Store onu object in map for particular intfid
		onuaddmap[intfid] = append(onuaddmap[intfid], Onu)
	}

	if len(onuaddmap) >= 1 {
		//Pass onumap to activateONU to handle indication to VOLTHA
		s.activateONUs(*s.EnableServer, onuaddmap)
	}

	return &pb.BBSimResponse{StatusMsg: RequestAccepted}, nil
}

// handleONUDeactivate deactivates ONU described by a single ONUInfo object
func (s *Server) handleONUDeactivate(in *pb.ONUInfo) error {

	if s.EnableServer == nil {
		logger.Error(OLTNotEnabled)
		return status.Errorf(codes.FailedPrecondition, OLTNotEnabled)
	}

	if in.OnuSerial != "" {
		// Get OpenOlt serial number from string
		serialNumber, err := getOpenoltSerialNumber(in.OnuSerial)
		if err != nil {
			logger.Error("Invalid serial number %s", in.OnuSerial)
			return status.Errorf(codes.InvalidArgument, "serial: "+in.OnuSerial+" is invalid")
		}
		// Get ONU by serial number
		onu, exist := s.getOnuFromSNmap(serialNumber)
		if !exist {
			logger.Error("ONU with serial number %s not found", in.OnuSerial)
			return status.Errorf(codes.NotFound, "serial: "+in.OnuSerial+" not found")
		}

		if err := s.HandleOnuDeactivate(onu); err != nil {
			return err
		}
	} else {
		if in.OnuId != 0 { // if provided, delete ONU by ONU ID
			onu, err := getOnuByID(s.Onumap, in.OnuId, in.PonPortId)
			if err != nil {
				return err
			}
			if err := s.HandleOnuDeactivate(onu); err != nil {
				return err
			}
		} else { // delete all ONUs on provided port
			if err := s.DeactivateAllOnuByIntfID(in.PonPortId); err != nil {
				logger.Error("Failed in handleONUDeactivate: %v", err)
				return err
			}
		}
	}
	return nil
}

func (s *Server) handleOLTReboot() {
	logger.Debug("HandleOLTReboot() invoked")
	logger.Debug("Sending stop to serverActionCh")
	s.serverActionCh <- OpenOltStop
	time.Sleep(40 * time.Second)

	logger.Debug("Sending start to serverActionCh")
	s.serverActionCh <- OpenOltStart
	for {
		if s.Olt.GetIntState() == device.OLT_ACTIVE {
			logger.Info("Info: OLT reactivated")
			break
		}
		time.Sleep(2 * time.Second)
	}
	s.sendOnuIndicationsOnOltReboot()
}

func (s *Server) handleONUHardReboot(onu *device.Onu) {
	logger.Debug("handleONUHardReboot() invoked")
	_ = sendDyingGaspInd(*s.EnableServer, onu.IntfID, onu.OnuID)
	device.UpdateOnusOpStatus(onu.IntfID, onu, "down")
	// send operstat down to voltha
	_ = sendOnuInd(*s.EnableServer, onu, "down", "up")
	// Give OEH some time to perform cleanup
	time.Sleep(30 * time.Second)
	s.activateOnu(onu)
}

func (s *Server) handleONUSoftReboot(IntfID uint32, OnuID uint32) {
	logger.Debug("handleONUSoftReboot() invoked")
	onu, err := s.GetOnuByID(OnuID, IntfID)
	if err != nil {
		logger.Error("No onu found with given OnuID on interface %v", IntfID)
	}
	OnuAlarmRequest := &pb.ONUAlarmRequest{
		OnuSerial: stringifySerialNumber(onu.SerialNumber),
		AlarmType: OnuLossOfPloam,
		Status:    "on",
	}
	// Raise alarm
	_, err = s.handleOnuAlarm(OnuAlarmRequest)
	if err != nil {
		logger.Error(err.Error())
	}
	// Clear alarm
	time.Sleep(10 * time.Second)
	OnuAlarmRequest.Status = "off"
	_, err = s.handleOnuAlarm(OnuAlarmRequest)
	if err != nil {
		logger.Error(err.Error())
	}
}

// GetNextFreeOnu returns free onu object for specified interface ID
func (s *Server) GetNextFreeOnu(intfid uint32) (*device.Onu, error) {
	onus, ok := s.Onumap[intfid]
	if !ok {
		return nil, errors.New("interface " + strconv.Itoa(int(intfid)) + " not present in ONU map")
	}
	for _, onu := range onus {
		if onu.InternalState == device.ONU_FREE {
			// If auto generated serial number is already used by some other ONU,
			// continue to find for other free object
			snkey := stringifySerialNumber(onu.SerialNumber)
			if _, exist := s.SNmap.Load(snkey); exist {
				continue
			}
			// Update Onu Internal State
			onu.InternalState = device.ONU_INACTIVE
			return onu, nil
		}
	}
	return nil, errors.New("no free ONU found for pon port: " + strconv.Itoa(int(intfid)))
}

// DeactivateAllOnuByIntfID deletes all ONUs for given PON port ID
func (s *Server) DeactivateAllOnuByIntfID(intfid uint32) error {
	for _, onu := range s.Onumap[intfid] {
		if onu.InternalState == device.ONU_FREE || onu.InternalState == device.ONU_INACTIVE {
			continue
		}
		if err := s.HandleOnuDeactivate(onu); err != nil {
			return err
		}
	}
	return nil
}

// HandleOnuDeactivate method handles ONU state changes and sending Indication to voltha
func (s *Server) HandleOnuDeactivate(onu *device.Onu) error {
	logger.Debug("Deactivating ONU %d for Intf: %d", onu.OnuID, onu.IntfID)

	// Update ONU internal state to ONU_INACTIVE
	s.updateDevIntState(onu, device.ONU_INACTIVE)

	// Update ONU operstate to down
	onu.OperState = "down"

	// Send DyingGasp Alarm to VOLTHA
	_ = sendDyingGaspInd(*s.EnableServer, onu.IntfID, onu.OnuID)
	_ = sendOnuInd(*s.EnableServer, onu, onu.OperState, "down")
	return nil
}

func markONUsFree(onumap map[uint32][]*device.Onu) {
	for intfid := range onumap {
		for _, onu := range onumap[intfid] {
			onu.UpdateIntState(device.ONU_FREE)
		}
	}
}

func copyONUInfo(onu *device.Onu) *pb.ONUInfo {
	onuData := &pb.ONUInfo{
		OnuId:     onu.OnuID,
		PonPortId: onu.IntfID,
		OnuSerial: stringifySerialNumber(onu.SerialNumber),
		OnuState:  device.ONUState[onu.InternalState],
		OperState: onu.OperState,
	}
	return onuData
}

func (s *Server) fetchPortDetail(intfID uint32, portType string) (*pb.PortInfo, error) {
	logger.Debug("fetchPortDetail() invoked")
	portInfo := &pb.PortInfo{}
	switch portType {
	case device.IntfNni:
		if !s.isNniIntfPresentInOlt(intfID) {
			return &pb.PortInfo{}, errors.New("NNI " + strconv.Itoa(int(intfID)) + " not present in " +
				strconv.Itoa(int(s.Olt.ID)))
		}
		portInfo = &pb.PortInfo{
			PortType:          portType,
			PortId:            intfID,
			PonPortMaxOnus:    0,
			PonPortActiveOnus: 0,
			PortState:         s.Olt.NniIntfs[intfID].OperState,
			AlarmState:        device.OLTAlarmStateToString[s.Olt.NniIntfs[intfID].AlarmState],
		}
		return portInfo, nil

	case device.IntfPon:
		if !s.isPonIntfPresentInOlt(intfID) {
			return &pb.PortInfo{}, errors.New("PON " + strconv.Itoa(int(intfID)) + " not present in OLT-" +
				strconv.Itoa(int(s.Olt.ID)))
		}
		portInfo = &pb.PortInfo{
			PortType:          portType,
			PortId:            intfID,
			PonPortMaxOnus:    int32(len(s.Onumap[uint32(intfID)])),
			PonPortActiveOnus: s.getNoOfActiveOnuByPortID(intfID),
			PortState:         s.Olt.PonIntfs[intfID].OperState,
			AlarmState:        device.OLTAlarmStateToString[s.Olt.PonIntfs[intfID].AlarmState],
		}
		return portInfo, nil
	default:
		return &pb.PortInfo{}, errors.New(portType + " is not a valid port type")
	}
}

func (s *Server) validateDeviceActionRequest(request *pb.DeviceAction) (*pb.DeviceAction, error) {
	switch request.DeviceType {
	case DeviceTypeOnu:
		if request.DeviceSerialNumber == "" {
			return request, errors.New("onu serial number can not be blank")
		}

		if len(request.DeviceSerialNumber) != SerialNumberLength {
			return request, errors.New("invalid serial number provided")
		}

		if request.DeviceAction != SoftReboot && request.DeviceAction != HardReboot {
			return request, errors.New("invalid device action provided")
		}
		return request, nil
	case DeviceTypeOlt:
		request.DeviceType = DeviceTypeOlt
		request.DeviceAction = HardReboot
		return request, nil
	default:
		return request, errors.New("invalid device type")
	}
}

func (s *Server) getNoOfActiveOnuByPortID(portID uint32) uint32 {
	var noOfActiveOnus uint32
	for _, onu := range s.Onumap[portID] {
		if onu.InternalState == device.ONU_ACTIVE || onu.InternalState == device.ONU_OMCIACTIVE {
			noOfActiveOnus++
		}
	}
	return noOfActiveOnus
}

func (s *Server) isPonIntfPresentInOlt(intfID uint32) bool {
	for _, intf := range s.Olt.PonIntfs {
		if intf.IntfID == intfID {
			return true
		}
	}
	return false
}

func (s *Server) isNniIntfPresentInOlt(intfID uint32) bool {
	for _, intf := range s.Olt.NniIntfs {
		if intf.IntfID == intfID {
			return true
		}
	}
	return false
}

func getOltIP() net.IP {
	// TODO make this better
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		logger.Error(err.Error())
		return net.IP{}
	}
	defer func() {
		err := conn.Close()
		if err != nil {
			logger.Error(err.Error())
		}
	}()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}
