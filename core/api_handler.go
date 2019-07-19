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

	api "github.com/opencord/voltha-bbsim/api"
	"github.com/opencord/voltha-bbsim/common/logger"
	"github.com/opencord/voltha-bbsim/device"
	"github.com/opencord/voltha-bbsim/flow"
	openolt "github.com/opencord/voltha-protos/go/openolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Constants for reboot delays
const (
	OltRebootDelay     = 40
	OnuSoftRebootDelay = 10
	OnuHardRebootDelay = 30
)

// handleONUStatusRequest process ONU status request
func (s *Server) handleONUStatusRequest(in *api.ONUInfo) (*api.ONUs, error) {
	logger.Trace("handleONUStatusRequest() invoked")
	onuInfo := &api.ONUs{}
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
func (s *Server) handleONUActivate(in []*api.ONUInfo) (*api.BBSimResponse, error) {
	logger.Trace("handleONUActivate request received")
	logger.Debug("Received values: %+v\n", in)

	// Check if indication is enabled
	if s.EnableServer == nil {
		logger.Error(OLTNotEnabled)
		return &api.BBSimResponse{}, status.Errorf(codes.FailedPrecondition, OLTNotEnabled)
	}

	onuaddmap := make(map[uint32][]*device.Onu)
	var newSerialNums []string

	//Iterate request for each PON port specified
	for _, onu := range in {
		intfid := onu.PonPortId

		if !s.isPonIntfPresentInOlt(intfid) {
			return &api.BBSimResponse{}, status.Errorf(codes.OutOfRange, "PON-"+strconv.Itoa(int(intfid))+
				" not present in OLT-"+strconv.Itoa(int(s.Olt.ID)))
		}

		if s.Olt.PonIntfs[intfid].AlarmState == device.PonLosRaised {
			return &api.BBSimResponse{}, status.Errorf(codes.FailedPrecondition, "pon-"+strconv.Itoa(int(intfid))+" is not active")
		}

		// Get the free ONU object for the intfid
		Onu, err := s.GetNextFreeOnu(intfid)
		if err != nil {
			markONUsFree(onuaddmap)
			logger.Error("Failed to get free ONU object for intfID %d :%v", intfid, err)
			return &api.BBSimResponse{}, status.Errorf(codes.ResourceExhausted, err.Error())
		}

		// Check if Serial number is provided by user
		if onu.OnuSerial != "" {
			// Get OpenOlt serial number from string
			sn, err := getOpenoltSerialNumber(onu.OnuSerial)
			if err != nil {
				logger.Error("Failed to get OpenOlt serial number %v", err)
				Onu.InternalState = device.OnuFree
				markONUsFree(onuaddmap)
				return &api.BBSimResponse{}, status.Errorf(codes.InvalidArgument, "serial number: "+onu.OnuSerial+" is invalid")
			}

			// Check if serial number is not duplicate in requested ONUs
			for _, sn := range newSerialNums {
				if onu.OnuSerial == sn {
					logger.Error("Duplicate serial number found %s", sn)
					// Mark ONUs free
					markONUsFree(onuaddmap)
					Onu.InternalState = device.OnuFree
					return &api.BBSimResponse{}, status.Errorf(codes.InvalidArgument, "duplicate serial number: "+onu.OnuSerial+" provided")
				}
			}
			newSerialNums = append(newSerialNums, onu.OnuSerial)

			// Check if serial number already exist
			_, exist := s.getOnuFromSNmap(sn)
			if exist {
				logger.Error("Provided serial number %v already exist", sn)
				// Mark ONUs free
				markONUsFree(onuaddmap)
				Onu.InternalState = device.OnuFree
				return &api.BBSimResponse{}, status.Errorf(codes.AlreadyExists, "serial number: "+onu.OnuSerial+" already exist")
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

	return &api.BBSimResponse{StatusMsg: RequestAccepted}, nil
}

// handleONUDeactivate deactivates ONU described by a single ONUInfo object
func (s *Server) handleONUDeactivate(in *api.ONUInfo) error {
	logger.Trace("handleONUDeactivate() invoked")

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
	logger.Trace("HandleOLTReboot() invoked")
	logger.Debug("Sending stop to serverActionCh")
	s.serverActionCh <- OpenOltStop

	// Delete all flows
	err := flow.DeleteAllFlows()
	if err != nil {
		logger.Warn("%v", err)
	}

	// clear flowMap
	s.FlowMap = make(map[device.FlowKey]*openolt.Flow)

	// clear flow IDs from ONU objects
	for intfID := range s.Onumap {
		for _, onu := range s.Onumap[intfID] {
			onu.Flows = nil
		}
	}

	time.Sleep(OltRebootDelay * time.Second)

	logger.Debug("Sending start to serverActionCh")
	s.serverActionCh <- OpenOltStart
	for {
		if s.Olt.GetIntState() == device.OltActive {
			logger.Info("Info: OLT reactivated")
			break
		}
		time.Sleep(2 * time.Second)
	}
	s.sendOnuIndicationsOnOltReboot()
}

func (s *Server) handleONUHardReboot(onu *device.Onu) {
	logger.Trace("handleONUHardReboot() invoked")
	_ = sendDyingGaspInd(*s.EnableServer, onu.IntfID, onu.OnuID)
	device.UpdateOnusOpStatus(onu.IntfID, onu, "down")
	// send operstat down to voltha
	_ = sendOnuInd(*s.EnableServer, onu, "down", "up")
	// Give OEH some time to perform cleanup
	time.Sleep(OnuHardRebootDelay * time.Second)
	s.activateOnu(onu)
}

func (s *Server) handleONUSoftReboot(IntfID uint32, OnuID uint32) {
	logger.Trace("handleONUSoftReboot() invoked")
	onu, err := s.GetOnuByID(OnuID, IntfID)
	if err != nil {
		logger.Error("No onu found with given OnuID on interface %v", IntfID)
		return
	}
	OnuAlarmRequest := &api.ONUAlarmRequest{
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
	time.Sleep(OnuSoftRebootDelay * time.Second)
	OnuAlarmRequest.Status = "off"
	_, err = s.handleOnuAlarm(OnuAlarmRequest)
	if err != nil {
		logger.Error(err.Error())
	}
}

// GetNextFreeOnu returns free onu object for specified interface ID
func (s *Server) GetNextFreeOnu(intfid uint32) (*device.Onu, error) {
	logger.Trace("GetNextFreeOnu() invoked")
	onus, ok := s.Onumap[intfid]
	if !ok {
		return nil, errors.New("interface " + strconv.Itoa(int(intfid)) + " not present in ONU map")
	}
	for _, onu := range onus {
		if onu.InternalState == device.OnuFree {
			// If auto generated serial number is already used by some other ONU,
			// continue to find for other free object
			snkey := stringifySerialNumber(onu.SerialNumber)
			if _, exist := s.SNmap.Load(snkey); exist {
				continue
			}
			// Update Onu Internal State
			onu.InternalState = device.OnuInactive
			return onu, nil
		}
	}
	return nil, errors.New("no free ONU found for pon port: " + strconv.Itoa(int(intfid)))
}

// DeactivateAllOnuByIntfID deletes all ONUs for given PON port ID
func (s *Server) DeactivateAllOnuByIntfID(intfid uint32) error {
	logger.Trace("DeactivateAllOnuByIntfID() invoked")
	for _, onu := range s.Onumap[intfid] {
		if onu.InternalState == device.OnuFree || onu.InternalState == device.OnuInactive {
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
	logger.Trace("HandleOnuDeactivate() invoked")
	logger.Info("Deactivating ONU %d for Intf: %d", onu.OnuID, onu.IntfID)

	// Update ONU internal state to ONU_INACTIVE
	s.updateDevIntState(onu, device.OnuInactive)

	// Update ONU operstate to down
	onu.OperState = "down"

	// Send DyingGasp Alarm to VOLTHA
	_ = sendDyingGaspInd(*s.EnableServer, onu.IntfID, onu.OnuID)
	_ = sendOnuInd(*s.EnableServer, onu, onu.OperState, "down")
	return nil
}

func markONUsFree(onumap map[uint32][]*device.Onu) {
	logger.Trace("markONUsFree() invoked")
	for intfid := range onumap {
		for _, onu := range onumap[intfid] {
			onu.UpdateIntState(device.OnuFree)
		}
	}
}

func copyONUInfo(onu *device.Onu) *api.ONUInfo {
	onuData := &api.ONUInfo{
		OnuId:     onu.OnuID,
		PonPortId: onu.IntfID,
		OnuSerial: stringifySerialNumber(onu.SerialNumber),
		OnuState:  device.ONUState[onu.InternalState],
		OperState: onu.OperState,
	}

	// update gemports
	for _, gemPorts := range onu.GemPortMap {
		onuData.Gemports = append(onuData.Gemports, gemPorts...)
	}

	// fill T-CONT data for ONU
	if onu.Tconts != nil {
		onuData.Tconts = &api.Tconts{
			UniId:  onu.Tconts.UniId,
			PortNo: onu.Tconts.PortNo,
			Tconts: onu.Tconts.TrafficScheds,
		}
	}

	return onuData
}

func (s *Server) fetchPortDetail(intfID uint32, portType string) (*api.PortInfo, error) {
	logger.Trace("fetchPortDetail() invoked %s-%d", portType, intfID)

	portInfo := &api.PortInfo{}
	var maxOnu, activeOnu uint32
	var alarmState device.AlarmState
	var state string

	// Get info for specified port
	if portType == device.IntfNni && s.isNniIntfPresentInOlt(intfID) {
		state = s.Olt.NniIntfs[intfID].OperState
		alarmState = s.Olt.NniIntfs[intfID].AlarmState

	} else if portType == device.IntfPon && s.isPonIntfPresentInOlt(intfID) {
		maxOnu = uint32(len(s.Onumap[intfID]))
		activeOnu = s.getNoOfActiveOnuByPortID(intfID)
		state = s.Olt.PonIntfs[intfID].OperState
		alarmState = s.Olt.PonIntfs[intfID].AlarmState

	} else {
		return &api.PortInfo{}, errors.New(portType + "-" + strconv.Itoa(int(intfID)) + " not present in OLT-" +
			strconv.Itoa(int(s.Olt.ID)))
	}

	// fill proto structure
	portInfo = &api.PortInfo{
		PortType:          portType,
		PortId:            intfID,
		PonPortMaxOnus:    maxOnu,
		PonPortActiveOnus: activeOnu,
		PortState:         state,
	}

	// update alarm state only when alarm is raised
	if alarmState == device.NniLosRaised || alarmState == device.PonLosRaised {
		portInfo.AlarmState = device.OLTAlarmStateToString[alarmState]
	}

	return portInfo, nil
}

func (s *Server) validateDeviceActionRequest(request *api.DeviceAction) (*api.DeviceAction, error) {
	logger.Trace("validateDeviceActionRequest() invoked")
	switch request.DeviceType {
	case DeviceTypeOnu:
		if request.SerialNumber == "" {
			return request, errors.New("onu serial number can not be blank")
		}

		if len(request.SerialNumber) != SerialNumberLength {
			return request, errors.New("invalid serial number provided")
		}

		_, exist := s.SNmap.Load(request.SerialNumber)
		if !exist {
			return &api.DeviceAction{}, errors.New(request.SerialNumber + " not present in OLT-" +
				strconv.Itoa(int(s.Olt.ID)))
		}

		if request.Action != SoftReboot && request.Action != HardReboot {
			return request, errors.New("invalid device action provided")
		}
		return request, nil
	case DeviceTypeOlt:
		request.DeviceType = DeviceTypeOlt
		request.Action = HardReboot
		return request, nil
	default:
		return request, errors.New("invalid device type")
	}
}

func (s *Server) getNoOfActiveOnuByPortID(portID uint32) uint32 {
	var noOfActiveOnus uint32
	for _, onu := range s.Onumap[portID] {
		if onu.InternalState >= device.OnuActive {
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
