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

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"gerrit.opencord.org/voltha-bbsim/common/utils"
	"gerrit.opencord.org/voltha-bbsim/device"
	"gerrit.opencord.org/voltha-bbsim/protos"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
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
		matched.UpdateIntStatus(device.ONU_ACTIVATED)
		logger.Debug("ONU IntfID: %d OnuID: %d activated succesufully.", onu.IntfId, onu.OnuId)
	}
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
	var resp OmciIndication
	logger.Debug("OLT %d receives OmciMsgOut to IF %v (ONU-ID: %v) pkt:%x.", s.Olt.ID, msg.IntfId, msg.OnuId, msg.Pkt)
	resp.IntfId = msg.IntfId
	resp.OnuId = msg.OnuId
	resp.Pkt = make([]byte, len(msg.Pkt))
	s.omciChan <- resp
	return new(openolt.Empty), nil
}

func (s *Server) OnuPacketOut(c context.Context, packet *openolt.OnuPacket) (*openolt.Empty, error) {
	onu, _ := s.GetOnuByID(packet.OnuId)
	utils.LoggerWithOnu(onu).Debug("OLT %d receives OnuPacketOut () to IF-ID:%d ONU-ID %d.", s.Olt.ID, packet.IntfId, packet.OnuId)
	onuid := packet.OnuId
	intfid := packet.IntfId
	rawpkt := gopacket.NewPacket(packet.Pkt, layers.LayerTypeEthernet, gopacket.Default)
	if err := s.onuPacketOut(intfid, onuid, rawpkt); err != nil {
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

	onu, _ := s.GetOnuByID(uint32(flow.OnuId))

	utils.LoggerWithOnu(onu).WithFields(log.Fields{
		"olt":   s.Olt.ID,
		"c_tag": flow.Action.IVid,
	}).Debug("OLT receives FlowAdd().")

	return new(openolt.Empty), nil
}

func (s *Server) FlowRemove(c context.Context, flow *openolt.Flow) (*openolt.Empty, error) {
	onu, _ := s.GetOnuByID(uint32(flow.OnuId))

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
