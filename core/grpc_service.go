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
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	omci "github.com/opencord/omci-sim"
	"github.com/opencord/voltha-bbsim/common/logger"
	"github.com/opencord/voltha-bbsim/device"
	flowHandler "github.com/opencord/voltha-bbsim/flow"
	openolt "github.com/opencord/voltha-protos/go/openolt"
	"github.com/opencord/voltha-protos/go/tech_profile"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DisableOlt method sends OLT down indication
func (s *Server) DisableOlt(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Debug("OLT receives DisableOLT()")

	err := flowHandler.PortDown(0)
	if err != nil {
		logger.Error("Failed in port down %v", err)
	}

	if s.EnableServer != nil {
		if err := sendOltIndDown(*s.EnableServer); err != nil {
			return new(openolt.Empty), err
		}
		logger.Debug("Successfuly sent OLT DOWN indication")
	}
	return new(openolt.Empty), nil
}

// ReenableOlt method sends OLT up indication for re-enabling OLT
func (s *Server) ReenableOlt(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Debug("OLT receives Reenable()")

	err := flowHandler.PortUp(0)
	if err != nil {
		logger.Error("Failed in port up %v", err)
	}

	if s.EnableServer != nil {
		s.Olt.OperState = "up"
		if err := sendOltIndUp(*s.EnableServer, s.Olt); err != nil {
			logger.Error("Failed to send OLT UP indication for reenable OLT: %v", err)
			return new(openolt.Empty), err
		}
		logger.Debug("Successfuly sent OLT UP indication")
	}
	return new(openolt.Empty), nil
}

// CollectStatistics method invoked by VOLTHA to get OLT statistics
func (s *Server) CollectStatistics(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Debug("OLT receives CollectStatistics()")
	return new(openolt.Empty), nil
}

// GetDeviceInfo returns OLT info
func (s *Server) GetDeviceInfo(c context.Context, empty *openolt.Empty) (*openolt.DeviceInfo, error) {
	logger.Debug("OLT receives GetDeviceInfo()")
	devinfo := new(openolt.DeviceInfo)
	devinfo.Vendor = "EdgeCore"
	devinfo.Model = "asfvolt16"
	devinfo.HardwareVersion = ""
	devinfo.FirmwareVersion = ""
	devinfo.Technology = "xgspon"
	devinfo.PonPorts = 16
	devinfo.OnuIdStart = 1
	devinfo.OnuIdEnd = 255
	devinfo.AllocIdStart = 1024
	devinfo.AllocIdEnd = 16383
	devinfo.GemportIdStart = 1024
	devinfo.GemportIdEnd = 65535
	devinfo.FlowIdStart = 1
	devinfo.FlowIdEnd = 16383
	devinfo.DeviceSerialNumber = s.Olt.SerialNumber

	return devinfo, nil
}

// ActivateOnu method handles ONU activation request from VOLTHA
func (s *Server) ActivateOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error) {
	logger.Trace("OLT receives ActivateONU()")

	matched, exist := s.getOnuFromSNmap(onu.SerialNumber)
	if !exist {
		logger.Error("ONU not found with serial nnumber %v", onu.SerialNumber)
		return new(openolt.Empty), status.Errorf(codes.NotFound, "ONU not found with serial number %v", onu.SerialNumber)
	}
	onuid := onu.OnuId
	matched.OnuID = onuid
	s.updateDevIntState(matched, device.OnuActive)
	logger.Debug("ONU IntfID: %d OnuID: %d activated succesufully.", onu.IntfId, onu.OnuId)
	if err := sendOnuInd(*s.EnableServer, matched, "up", "up"); err != nil {
		logger.Error("Failed to send ONU Indication intfID %d, onuID %d", matched.IntfID, matched.OnuID)
		return new(openolt.Empty), err
	}

	return new(openolt.Empty), nil
}

// CreateTrafficSchedulers method should handle TrafficScheduler creation
func (s *Server) CreateTrafficSchedulers(c context.Context, traffScheduler *tech_profile.TrafficSchedulers) (*openolt.Empty, error) {
	logger.Debug("OLT receives CreateTrafficSchedulers %v", traffScheduler)
	onu, err := s.GetOnuByID(traffScheduler.OnuId, traffScheduler.IntfId)
	if err != nil {
		return new(openolt.Empty), err
	}
	onu.Tconts = traffScheduler
	return new(openolt.Empty), nil
}

// RemoveTrafficSchedulers method should handle TrafficScheduler removal
func (s *Server) RemoveTrafficSchedulers(c context.Context, traffScheduler *tech_profile.TrafficSchedulers) (*openolt.Empty, error) {
	logger.Debug("OLT receives RemoveTrafficSchedulers %v", traffScheduler)
	onu, err := s.GetOnuByID(traffScheduler.OnuId, traffScheduler.IntfId)
	if err != nil {
		return new(openolt.Empty), err
	}
	for _, tcont := range traffScheduler.TrafficScheds {
		if _, exist := onu.GemPortMap[tcont.AllocId]; exist {
			delete(onu.GemPortMap, tcont.AllocId)
		}
	}
	onu.Tconts = nil
	return new(openolt.Empty), nil
}

// CreateTrafficQueues method should handle TrafficQueues creation
func (s *Server) CreateTrafficQueues(context.Context, *tech_profile.TrafficQueues) (*openolt.Empty, error) {
	logger.Debug("OLT receives CreateTrafficQueues()")
	return new(openolt.Empty), nil
}

// RemoveTrafficQueues method should handle TrafficQueues removal
func (s *Server) RemoveTrafficQueues(context.Context, *tech_profile.TrafficQueues) (*openolt.Empty, error) {
	logger.Debug("OLT receives RemoveTrafficQueues()")
	return new(openolt.Empty), nil
}

// DeactivateOnu method should handle ONU deactivation
func (s *Server) DeactivateOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error) {
	logger.Debug("OLT receives DeactivateONU()")
	return new(openolt.Empty), nil
}

// DeleteOnu handles ONU deletion request from VOLTHA
func (s *Server) DeleteOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error) {
	logger.Debug("OLT receives DeleteONU() intfID: %d, onuID: %d", onu.IntfId, onu.OnuId)
	Onu, err := s.GetOnuByID(onu.OnuId, onu.IntfId)
	if err != nil {
		return new(openolt.Empty), err
	}

	// Mark ONU internal state as ONU_FREE and reset onuID
	Onu.InternalState = device.OnuFree
	Onu.OnuID = 0

	// Get snMap key for the ONU serial number
	snkey := stringifySerialNumber(Onu.SerialNumber)

	// Delete Serial number entry from SNmap
	logger.Info("Deleting serial number %s from SNmap", snkey)
	s.SNmap.Delete(snkey)
	return new(openolt.Empty), nil
}

// GetOnuInfo returns ONU info to VOLTHA
func (s *Server) GetOnuInfo(c context.Context, onu *openolt.Onu) (*openolt.OnuIndication, error) {
	logger.Debug("Olt receives GetOnuInfo() intfID: %d, onuID: %d", onu.IntfId, onu.OnuId)
	Onu, err := s.GetOnuByID(onu.OnuId, onu.IntfId)
	if err != nil {
		logger.Error("ONU not found intfID %d, onuID %d", onu.IntfId, onu.OnuId)
		return new(openolt.OnuIndication), err
	}
	onuIndication := new(openolt.OnuIndication)
	onuIndication.IntfId = Onu.IntfID
	onuIndication.OnuId = Onu.OnuID
	onuIndication.OperState = Onu.OperState

	return onuIndication, nil
}

// OmciMsgOut receives OMCI messages from voltha
func (s *Server) OmciMsgOut(c context.Context, msg *openolt.OmciMsg) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives OmciMsgOut to IF %v (ONU-ID: %v) pkt:%x.", s.Olt.ID, msg.IntfId, msg.OnuId, msg.Pkt)
	// Get ONU state
	onu, err := s.GetOnuByID(msg.OnuId, msg.IntfId)
	if err != nil {
		logger.Error("ONU not found intfID %d, onuID %d", msg.IntfId, msg.OnuId)
		return new(openolt.Empty), err
	}
	state := onu.GetIntState()
	logger.Debug("ONU-ID: %v, ONU state: %d", msg.OnuId, state)

	// If ONU is not ACTIVE drop OMCI message
	if state < device.OnuActive {
		logger.Info("ONU (IF %v ONU-ID: %v) is not ACTIVE, so not processing OmciMsg", msg.IntfId, msg.OnuId)
		return new(openolt.Empty), nil
	}
	s.omciOut <- *msg
	return new(openolt.Empty), nil
}

// OnuPacketOut is used by voltha to send OnuPackets to BBSIM
func (s *Server) OnuPacketOut(c context.Context, packet *openolt.OnuPacket) (*openolt.Empty, error) {
	onu, err := s.GetOnuByID(packet.OnuId, packet.IntfId)
	if err != nil {
		logger.Error("Failed in OnuPacketOut, %v", err)
		return new(openolt.Empty), err
	}
	device.LoggerWithOnu(onu).Debugf("OLT %d receives OnuPacketOut () to IF-ID:%d ONU-ID %d.", s.Olt.ID, packet.IntfId, packet.OnuId)
	onuid := packet.OnuId
	intfid := packet.IntfId
	rawpkt := gopacket.NewPacket(packet.Pkt, layers.LayerTypeEthernet, gopacket.Default)
	if err := s.onuPacketOut(intfid, onuid, rawpkt); err != nil {
		device.LoggerWithOnu(onu).WithField("error", err).Errorf("OnuPacketOut Error ")
		return new(openolt.Empty), err
	}
	return new(openolt.Empty), nil
}

// UplinkPacketOut sends uplink packets to BBSIM
func (s *Server) UplinkPacketOut(c context.Context, packet *openolt.UplinkPacket) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives UplinkPacketOut().", s.Olt.ID)
	rawpkt := gopacket.NewPacket(packet.Pkt, layers.LayerTypeEthernet, gopacket.Default)
	if err := s.uplinkPacketOut(rawpkt); err != nil {
		return new(openolt.Empty), err
	}
	return new(openolt.Empty), nil
}

// FlowAdd method should handle flows addition to datapath for OLT and ONU
func (s *Server) FlowAdd(c context.Context, flow *openolt.Flow) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives FlowAdd() %v", s.Olt.ID, flow)
	// Check if flow already present
	flowKey := device.FlowKey{
		FlowID:        flow.FlowId,
		FlowDirection: flow.FlowType,
	}
	if _, exist := s.FlowMap[flowKey]; exist {
		logger.Error("Flow already exists %v", flow)
		return new(openolt.Empty), status.Errorf(codes.AlreadyExists, "Flow already exists")
	}

	// Send flow to flowHandler
	err := flowHandler.AddFlow(flow)
	if err != nil {
		logger.Error("Error in pushing flow to datapath")
	}

	// Update flowMap
	s.FlowMap[flowKey] = flow

	onu, err := s.GetOnuByID(uint32(flow.OnuId), uint32(flow.AccessIntfId))
	if err == nil {
		exist := false
		// check if gemport already present in ONU
		for _, gemport := range onu.GemPortMap[uint32(flow.AllocId)] {
			if gemport == uint32(flow.GemportId) {
				exist = true
				break
			}
		}
		// if not present already, then append in gemport list
		if !exist {
			onu.GemPortMap[uint32(flow.AllocId)] = append(onu.GemPortMap[uint32(flow.AllocId)], uint32(flow.GemportId))
		}

		// EAPOL flow
		if flow.Classifier.EthType == uint32(layers.EthernetTypeEAPOL) {
			logger.WithFields(log.Fields{
				"Classifier.OVid":    flow.Classifier.OVid,
				"Classifier.IVid":    flow.Classifier.IVid,
				"Classifier.EthType": flow.Classifier.EthType,
				"Classifier.SrcPort": flow.Classifier.SrcPort,
				"Classifier.DstPort": flow.Classifier.DstPort,
				"Action.OVid":        flow.Action.OVid,
				"Action.IVid":        flow.Action.IVid,
				"IntfID":             flow.AccessIntfId,
				"OltID":              s.Olt.ID,
				"OnuID":              flow.OnuId,
				"FlowId":             flow.FlowId,
				"UniID":              flow.UniId,
				"PortNo":             flow.PortNo,
				"FlowType":           flow.FlowType,
			}).Debug("OLT receives EAPOL flow")

			if flow.Classifier.OVid == 4091 {
				omcistate := omci.GetOnuOmciState(onu.IntfID, onu.OnuID)
				if omcistate != omci.DONE {
					logger.Warn("FlowAdd() OMCI state %d is not \"DONE\"", omci.GetOnuOmciState(onu.OnuID, onu.IntfID))
				}
				_ = s.updateOnuIntState(onu.IntfID, onu.OnuID, device.OnuOmciActive)
			}
		}

		// DHCP flow
		if flow.Classifier.EthType == uint32(layers.EthernetTypeIPv4) {
			logger.Debug("Received flow's srcPort:%d dstPort:%d", flow.Classifier.SrcPort, flow.Classifier.DstPort)
			if flow.Classifier.SrcPort == uint32(68) && flow.Classifier.DstPort == uint32(67) {
				logger.Debug("OLT %d receives DHCP flow IntfID:%d OnuID:%d EType:%x GemPortID:%d VLanIDs: %d/%d",
					s.Olt.ID, flow.AccessIntfId, flow.OnuId, flow.Classifier.EthType, flow.GemportId, flow.Classifier.OVid, flow.Classifier.IVid)
				omcistate := omci.GetOnuOmciState(onu.IntfID, onu.OnuID)
				if omcistate != omci.DONE {
					logger.Warn("FlowAdd() OMCI state %d is not \"DONE\"", omci.GetOnuOmciState(onu.OnuID, onu.IntfID))
				}
				_ = s.updateOnuIntState(onu.IntfID, onu.OnuID, device.OnuAuthenticated)
			}
		}
		// Update flows in ONU object
		onu.Flows = append(onu.Flows, flowKey)
	}
	return new(openolt.Empty), nil
}

// FlowRemove handles flow deletion from datapath
func (s *Server) FlowRemove(c context.Context, flow *openolt.Flow) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives FlowRemove(): %v", s.Olt.ID, flow)

	// Check if flow exists
	flowKey := device.FlowKey{
		FlowID:        flow.FlowId,
		FlowDirection: flow.FlowType,
	}
	if _, exist := s.FlowMap[flowKey]; !exist {
		logger.Error("Flow %v not found", flow)
		return new(openolt.Empty), status.Errorf(codes.NotFound, "Flow not found")
	}

	flow = s.FlowMap[flowKey]
	// Send delete flow to flowHandler
	err := flowHandler.DeleteFlow(flow)
	if err != nil {
		logger.Error("failed to delete flow")
	}

	onu, err := s.GetOnuByID(uint32(flow.OnuId), uint32(flow.AccessIntfId))
	if err != nil {
		logger.Warn("Failed flow remove %v", err)
	} else {
		// Delete flows from onu
		onu.DeleteFlow(flowKey)
		logger.Debug("Flows %v in onu %d", onu.Flows, onu.OnuID)
	}

	// Delete flow from flowMap
	delete(s.FlowMap, flowKey)

	return new(openolt.Empty), nil
}

// HeartbeatCheck is currently not used by voltha
func (s *Server) HeartbeatCheck(c context.Context, empty *openolt.Empty) (*openolt.Heartbeat, error) {
	logger.Debug("OLT %d receives HeartbeatCheck().", s.Olt.ID)
	signature := new(openolt.Heartbeat)
	signature.HeartbeatSignature = s.Olt.HeartbeatSignature
	return signature, nil
}

// EnablePonIf enables pon interfaces at BBSIM
func (s *Server) EnablePonIf(c context.Context, intf *openolt.Interface) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives EnablePonIf().", s.Olt.ID)
	return new(openolt.Empty), nil
}

// GetPonIf returns interface info to VOLTHA
func (s *Server) GetPonIf(c context.Context, intf *openolt.Interface) (*openolt.IntfIndication, error) {
	logger.Debug("OLT %d receives GetPonIf().", s.Olt.ID)
	stat := new(openolt.IntfIndication)

	if intf.IntfId > (s.Olt.NumPonIntf - 1) {
		logger.Error("PON ID %d out of bounds. %d ports total", intf.IntfId, s.Olt.NumPonIntf)
		return stat, status.Errorf(codes.OutOfRange, "PON ID %d out of bounds. %d ports total (indexing starts at 0)", intf.IntfId, s.Olt.NumPonIntf)
	}
	stat.IntfId = intf.IntfId
	stat.OperState = s.Olt.PonIntfs[intf.IntfId].OperState
	return stat, nil

}

// DisablePonIf disables pon interface at BBSIM
func (s *Server) DisablePonIf(c context.Context, intf *openolt.Interface) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives DisablePonIf().", s.Olt.ID)
	return new(openolt.Empty), nil
}

// Reboot method handles reboot of OLT
func (s *Server) Reboot(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives Reboot ().", s.Olt.ID)
	// Initialize OLT & Env
	logger.Debug("Initialized by Reboot")
	s.handleOLTReboot()
	return new(openolt.Empty), nil
}

// EnableIndication starts sending indications for OLT and ONU
func (s *Server) EnableIndication(empty *openolt.Empty, stream openolt.Openolt_EnableIndicationServer) error {
	logger.Debug("OLT receives EnableInd.")
	defer func() {
		logger.Debug("grpc EnableIndication Done")
	}()
	if err := s.Enable(&stream); err != nil {
		logger.Error("Failed to Enable Core: %v", err)
		return err
	}
	return nil
}

// NewGrpcServer starts openolt gRPC server
func NewGrpcServer(addrport string) (l net.Listener, g *grpc.Server, e error) {
	logger.Info("OpenOLT gRPC server listening %s ...", addrport)
	g = grpc.NewServer()
	l, e = net.Listen("tcp", addrport)
	return
}
