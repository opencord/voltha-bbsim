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
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"log"
	"net"
)

// gRPC Service
func (s *Server) DisableOlt(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Info("OLT receives DisableOLT()\n")
	if s.EnableServer != nil {
		if err := sendOltIndDown(*s.EnableServer); err != nil {
			return new(openolt.Empty), err
		}
		logger.Info("Successfuly sent OLT DOWN indication")
	}
	return new(openolt.Empty), nil
}

func (s *Server) ReenableOlt(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Info("OLT receives Reenable()\n")
	return new(openolt.Empty), nil
}

func (s *Server) CollectStatistics(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Info("OLT receives CollectStatistics()\n")
	return new(openolt.Empty), nil
}

func (s *Server) GetDeviceInfo(c context.Context, empty *openolt.Empty) (*openolt.DeviceInfo, error) {
	logger.Info("OLT receives GetDeviceInfo()\n")
	devinfo := new(openolt.DeviceInfo)
	devinfo.Vendor = "CORD"
	devinfo.OnuIdStart = 0
	devinfo.OnuIdEnd = 3
	devinfo.PonPorts = 4
	return devinfo, nil
}

func (s *Server) ActivateOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error) {
	logger.Info("OLT receives ActivateONU()\n")
	result := device.ValidateONU(*onu, s.Onumap)
	if result == true {
		matched, error := getOnuBySN(s.Onumap, onu.SerialNumber)
		if error != nil {
			log.Fatalf("%s\n", error)
		}
		onuid := onu.OnuId
		matched.OnuID = onuid
		matched.UpdateIntStatus(device.ONU_ACTIVATED)
		logger.Info("ONU IntfID: %d OnuID: %d activated succesufully.\n", onu.IntfId, onu.OnuId)
	}
	return new(openolt.Empty), nil
}

func (s *Server) DeactivateOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error) {
	logger.Info("OLT receives DeactivateONU()\n")
	return new(openolt.Empty), nil
}

func (s *Server) DeleteOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error) {
	logger.Info("OLT receives DeleteONU()\n")
	return new(openolt.Empty), nil
}

func (s *Server) OmciMsgOut(c context.Context, msg *openolt.OmciMsg) (*openolt.Empty, error) {
	logger.Info("OLT %d receives OmciMsgOut to IF %v (ONU-ID: %v) pkt:%x.\n", s.Olt.ID, msg.IntfId, msg.OnuId, msg.Pkt)
	//s.olt.Queue = append(s.olt.Queue, *msg)
	return new(openolt.Empty), nil
}

func (s *Server) OnuPacketOut(c context.Context, packet *openolt.OnuPacket) (*openolt.Empty, error) {
	logger.Info("OLT %d receives OnuPacketOut () to IF-ID:%d ONU-ID %d.\n", s.Olt.ID, packet.IntfId, packet.OnuId)
	onuid := packet.OnuId
	intfid := packet.IntfId
	rawpkt := gopacket.NewPacket(packet.Pkt, layers.LayerTypeEthernet, gopacket.Default)
	if err := s.onuPacketOut(intfid, onuid, rawpkt); err != nil {
		return new(openolt.Empty), err
	}
	return new(openolt.Empty), nil
}

func (s *Server) UplinkPacketOut(c context.Context, packet *openolt.UplinkPacket) (*openolt.Empty, error) {
	logger.Info("OLT %d receives UplinkPacketOut().\n", s.Olt.ID)
	rawpkt := gopacket.NewPacket(packet.Pkt, layers.LayerTypeEthernet, gopacket.Default)
	if err := s.uplinkPacketOut(rawpkt); err != nil {
		return new(openolt.Empty), err
	}
	return new(openolt.Empty), nil
}

func (s *Server) FlowAdd(c context.Context, flow *openolt.Flow) (*openolt.Empty, error) {
	logger.Info("OLT %d receives FlowAdd().\n", s.Olt.ID)
	logger.Debug("Flow's ONU-ID: %d, CTAG: %d\n", flow.OnuId, flow.Action.IVid)
	//onuid := uint32(flow.OnuId)
	//ctag := flow.Action.IVid
	//s.CtagMap[onuid] = ctag
	return new(openolt.Empty), nil
}

func (s *Server) FlowRemove(c context.Context, flow *openolt.Flow) (*openolt.Empty, error) {
	logger.Info("OLT %d receives FlowRemove().\n", s.Olt.ID)
	return new(openolt.Empty), nil
}

func (s *Server) HeartbeatCheck(c context.Context, empty *openolt.Empty) (*openolt.Heartbeat, error) {
	logger.Info("OLT %d receives HeartbeatCheck().\n", s.Olt.ID)
	signature := new(openolt.Heartbeat)
	signature.HeartbeatSignature = s.Olt.HeartbeatSignature
	return signature, nil
}

func (s *Server) EnablePonIf(c context.Context, intf *openolt.Interface) (*openolt.Empty, error) {
	logger.Info("OLT %d receives EnablePonIf().\n", s.Olt.ID)
	return new(openolt.Empty), nil
}

func (s *Server) DisablePonIf(c context.Context, intf *openolt.Interface) (*openolt.Empty, error) {
	logger.Info("OLT %d receives DisablePonIf().\n", s.Olt.ID)
	return new(openolt.Empty), nil
}

func (s *Server) Reboot(c context.Context, empty *openolt.Empty) (*openolt.Empty, error) {
	logger.Info("OLT %d receives Reboot ().\n", s.Olt.ID)
	// Initialize OLT & Env
	logger.Debug("Initialized by Reboot")
	s.Disable()
	return new(openolt.Empty), nil
}

func (s *Server) EnableIndication(empty *openolt.Empty, stream openolt.Openolt_EnableIndicationServer) error {
	logger.Info("OLT receives EnableInd.\n")
	defer func() {
		logger.Debug("grpc EnableIndication Done")
	}()
	if err := s.Enable(&stream); err != nil {
		logger.Error("Failed to Enable Core: %v\n", err)
		return err
	}
	return nil
}

func NewGrpcServer(addrport string) (l net.Listener, g *grpc.Server, e error) {
	logger.Info("Listening %s ...", addrport)
	g = grpc.NewServer()
	l, e = net.Listen("tcp", addrport)
	return
}
