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
	"strconv"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"gerrit.opencord.org/voltha-bbsim/common/utils"
	"gerrit.opencord.org/voltha-bbsim/device"
	"gerrit.opencord.org/voltha-bbsim/protos"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	omci "github.com/opencord/omci-sim"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// gRPC Service
func (s *Server) DisableOlt(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Debug("OLT receives DisableOLT()")
	if s.EnableServer != nil {
		if err := sendOltIndDown(*s.EnableServer); err != nil {
			return new(openolt.Empty), err
		}
		logger.Debug("Successfuly sent OLT DOWN indication")
	}
	return new(openolt.Empty), nil
}

func (s *Server) ReenableOlt(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Debug("OLT receives Reenable()")
	return new(openolt.Empty), nil
}

func (s *Server) CollectStatistics(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Debug("OLT receives CollectStatistics()")
	return new(openolt.Empty), nil
}

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
	devinfo.DeviceSerialNumber = "BBSIMOLT00"+strconv.FormatInt(int64(s.Olt.ID), 10)


	return devinfo, nil
}

func (s *Server) ActivateOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error) {
	logger.Debug("OLT receives ActivateONU()")
	result := device.ValidateONU(*onu, s.Onumap)
	if result == true {
		matched, error := getOnuBySN(s.Onumap, onu.SerialNumber)
		if error != nil {
			logger.Fatal("%s", error)
		}
		onuid := onu.OnuId
		matched.OnuID = onuid
		s.updateDevIntState(matched, device.ONU_ACTIVE)
		logger.Debug("ONU IntfID: %d OnuID: %d activated succesufully.", onu.IntfId, onu.OnuId)
	}
	return new(openolt.Empty), nil
}

func (s *Server) CreateTconts(c context.Context, tconts *openolt.Tconts) (*openolt.Empty, error) {
	logger.Debug("OLT receives CreateTconts()")
	return new(openolt.Empty), nil
}

func (s *Server) RemoveTconts(c context.Context, tconts *openolt.Tconts) (*openolt.Empty, error) {
	logger.Debug("OLT receives RemoveTconts()")
	return new(openolt.Empty), nil
}

func (s *Server) DeactivateOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error) {
	logger.Debug("OLT receives DeactivateONU()")
	return new(openolt.Empty), nil
}

func (s *Server) DeleteOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error) {
	logger.Debug("OLT receives DeleteONU()")
	return new(openolt.Empty), nil
}

func (s *Server) OmciMsgOut(c context.Context, msg *openolt.OmciMsg) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives OmciMsgOut to IF %v (ONU-ID: %v) pkt:%x.", s.Olt.ID, msg.IntfId, msg.OnuId, msg.Pkt)
	s.omciOut <- *msg
	return new(openolt.Empty), nil
}

func (s *Server) OnuPacketOut(c context.Context, packet *openolt.OnuPacket) (*openolt.Empty, error) {
	onu, _ := s.GetOnuByID(packet.OnuId, packet.IntfId)
	utils.LoggerWithOnu(onu).Debugf("OLT %d receives OnuPacketOut () to IF-ID:%d ONU-ID %d.", s.Olt.ID, packet.IntfId, packet.OnuId)
	onuid := packet.OnuId
	intfid := packet.IntfId
	rawpkt := gopacket.NewPacket(packet.Pkt, layers.LayerTypeEthernet, gopacket.Default)
	if err := s.onuPacketOut(intfid, onuid, rawpkt); err != nil {
		utils.LoggerWithOnu(onu).WithField("error", err).Errorf("OnuPacketOut Error ")
		return new(openolt.Empty), err
	}
	return new(openolt.Empty), nil
}

func (s *Server) UplinkPacketOut(c context.Context, packet *openolt.UplinkPacket) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives UplinkPacketOut().", s.Olt.ID)
	rawpkt := gopacket.NewPacket(packet.Pkt, layers.LayerTypeEthernet, gopacket.Default)
	if err := s.uplinkPacketOut(rawpkt); err != nil {
		return new(openolt.Empty), err
	}
	return new(openolt.Empty), nil
}

func (s *Server) FlowAdd(c context.Context, flow *openolt.Flow) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives FlowAdd() IntfID:%d OnuID:%d EType:%x GemPortID:%d", s.Olt.ID, flow.AccessIntfId, flow.OnuId, flow.Classifier.EthType, flow.GemportId)
	onu, err := s.GetOnuByID(uint32(flow.OnuId), uint32(flow.AccessIntfId))

	if err == nil {
		intfid := onu.IntfID
		onuid := onu.OnuID
		onu.GemportID = uint16(flow.GemportId)

		utils.LoggerWithOnu(onu).WithFields(log.Fields{
			"olt":   s.Olt.ID,
			"c_tag": flow.Action.IVid,
		}).Debug("OLT receives FlowAdd().")

		if flow.Classifier.EthType == uint32(layers.EthernetTypeEAPOL) {
			omcistate := omci.GetOnuOmciState(onu.IntfID, onu.OnuID)
			if omcistate != omci.DONE {
				logger.Warn("FlowAdd() OMCI state %d is not \"DONE\"", omci.GetOnuOmciState(onu.OnuID, onu.IntfID))
			}
			s.updateOnuIntState(intfid, onuid, device.ONU_OMCIACTIVE)
		}
	}
	return new(openolt.Empty), nil
}

func (s *Server) FlowRemove(c context.Context, flow *openolt.Flow) (*openolt.Empty, error) {
	onu, _ := s.GetOnuByID(uint32(flow.OnuId), uint32(flow.AccessIntfId))

	utils.LoggerWithOnu(onu).WithFields(log.Fields{
		"olt":   s.Olt.ID,
		"c_tag": flow.Action.IVid,
	}).Debug("OLT receives FlowRemove().")

	return new(openolt.Empty), nil
}

func (s *Server) HeartbeatCheck(c context.Context, empty *openolt.Empty) (*openolt.Heartbeat, error) {
	logger.Debug("OLT %d receives HeartbeatCheck().", s.Olt.ID)
	signature := new(openolt.Heartbeat)
	signature.HeartbeatSignature = s.Olt.HeartbeatSignature
	return signature, nil
}

func (s *Server) EnablePonIf(c context.Context, intf *openolt.Interface) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives EnablePonIf().", s.Olt.ID)
	return new(openolt.Empty), nil
}

func (s *Server) DisablePonIf(c context.Context, intf *openolt.Interface) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives DisablePonIf().", s.Olt.ID)
	return new(openolt.Empty), nil
}

func (s *Server) Reboot(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Debug("OLT %d receives Reboot ().", s.Olt.ID)
	// Initialize OLT & Env
	logger.Debug("Initialized by Reboot")
	s.Disable()
	return new(openolt.Empty), nil
}

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

func NewGrpcServer(addrport string) (l net.Listener, g *grpc.Server, e error) {
	logger.Debug("Listening %s ...", addrport)
	g = grpc.NewServer()
	l, e = net.Listen("tcp", addrport)
	return
}
