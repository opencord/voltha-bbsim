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
	"context"
	"errors"
	"strconv"
	"sync"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"gerrit.opencord.org/voltha-bbsim/common/utils"
	"gerrit.opencord.org/voltha-bbsim/device"
	"gerrit.opencord.org/voltha-bbsim/protos"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	UNI_VETH_UP_PFX  = "sim_uu"
	UNI_VETH_DW_PFX  = "sim_ud"
	NNI_VETH_UP_PFX  = "sim_nu"
	NNI_VETH_DW_PFX  = "sim_nd"
	MAX_ONUS_PER_PON = 64 // This value should be the same with the value in AdapterPlatrorm class
)

type Server struct {
	wg           *sync.WaitGroup
	Olt          *device.Olt
	Onumap       map[uint32][]*device.Onu
	Ioinfos      []*Ioinfo
	gRPCserver   *grpc.Server
	gRPCAddress  string
	gRPCPort     uint32
	Vethnames    []string
	IndInterval  int
	Processes    []string
	EnableServer *openolt.Openolt_EnableIndicationServer
	CtagMap      map[string]uint32
	cancel       context.CancelFunc
	state        coreState
	stateChan    chan coreState
	omciIn       chan openolt.OmciIndication
	omciOut      chan openolt.OmciMsg
}

type Packet struct {
	Info *Ioinfo
	Pkt  gopacket.Packet
}

type coreState int

const (
	INACTIVE   = iota // OLT/ONUs are not instantiated
	PRE_ACTIVE        // Before PacketInDaemon Running
	ACTIVE            // After PacketInDaemon Running
)

/* coreState
INACTIVE -> PRE_ACTIVE -> ACTIVE
    (ActivateOLT)   (Enable)
       <-              <-
*/

func NewCore(opt *option, omciOut chan openolt.OmciMsg, omciIn chan openolt.OmciIndication) *Server {
	// TODO: make it decent
	oltid := opt.oltid
	npon := opt.npon
	nonus := opt.nonus
	s := Server{
		Olt:          device.NewOlt(oltid, npon, 1),
		Onumap:       make(map[uint32][]*device.Onu),
		Ioinfos:      []*Ioinfo{},
		gRPCAddress:  opt.address,
		gRPCPort:     opt.port,
		Vethnames:    []string{},
		IndInterval:  opt.intvl,
		Processes:    []string{},
		EnableServer: nil,
		state:        INACTIVE,
		stateChan:    make(chan coreState, 8),
		omciIn:       omciIn,
		omciOut:      omciOut,
	}

	nnni := s.Olt.NumNniIntf
	logger.Info("OLT ID: %d was retrieved.", s.Olt.ID)
	for intfid := nnni; intfid < npon+nnni; intfid++ {
		s.Onumap[intfid] = device.NewOnus(oltid, intfid, nonus, nnni)
	}

	//TODO: To be fixed because it is hardcoded
	s.CtagMap = make(map[string]uint32)
	for i := 0; i < MAX_ONUS_PER_PON; i++ {
		oltid := s.Olt.ID
		intfid := uint32(1)
		sn := convB2S(device.NewSN(oltid, intfid, uint32(i)))
		s.CtagMap[sn] = uint32(900 + i) // This is hard coded for BBWF
	}
	return &s
}

//Blocking
func (s *Server) Start() error {
	s.wg = &sync.WaitGroup{}
	logger.Debug("Start() Start")
	defer func() {
		close(s.stateChan)
		logger.Debug("Start() Done")
	}()
	addressport := s.gRPCAddress + ":" + strconv.Itoa(int(s.gRPCPort))
	listener, gserver, err := NewGrpcServer(addressport)
	if err != nil {
		logger.Error("Failed to create gRPC server", err)
		return err
	}
	s.gRPCserver = gserver
	openolt.RegisterOpenoltServer(gserver, s)
	if err := gserver.Serve(listener); err != nil {
		logger.Error("Failed to run gRPC server", err)
		return err
	}
	s.wg.Wait()
	return nil
}

//Non-Blocking
func (s *Server) Stop() {
	logger.Debug("Stop() Start")
	defer logger.Debug("Stop() Done")
	if s.gRPCserver != nil {
		s.gRPCserver.Stop()
		logger.Debug("gRPCserver.Stop()")
	}
	s.StopPktInDaemon()
	return
}

// Blocking
func (s *Server) Enable(sv *openolt.Openolt_EnableIndicationServer) error {
	defer func() {
		olt := s.Olt
		olt.InitializeStatus()
		for intfid, _ := range s.Onumap {
			for _, onu := range s.Onumap[intfid] {
				onu.InitializeStatus()
			}
		}
		s.updateState(INACTIVE)
		logger.Debug("Enable() Done")
	}()
	logger.Debug("Enable() Start")
	s.EnableServer = sv
	if err := s.activateOLT(*sv); err != nil {
		return err
	}
	s.updateState(PRE_ACTIVE)

	coreCtx := context.Background()
	coreCtx, corecancel := context.WithCancel(coreCtx)
	s.cancel = corecancel
	if err := s.StartPktInDaemon(coreCtx, *sv); err != nil {
		return err
	}
	return nil
}

//Non-Blocking
func (s *Server) Disable() {
	defer func() {
		logger.Debug("Disable() Done")
	}()
	logger.Debug("Disable() Start")
	s.StopPktInDaemon()
}

func (s *Server) updateState(state coreState) {
	s.state = state
	s.stateChan <- state
	logger.Debug("State updated to:%d", state)
}

func (s *Server) activateOLT(stream openolt.Openolt_EnableIndicationServer) error {
	defer logger.Debug("activateOLT() Done")
	logger.Debug("activateOLT() Start")
	// Activate OLT
	olt := s.Olt
	if err := sendOltIndUp(stream, olt); err != nil {
		return err
	}
	olt.OperState = "up"
	*olt.InternalState = device.OLT_UP
	logger.Info("OLT %s sent OltInd.", olt.Name)

	// OLT sends Interface Indication to Adapter
	if err := sendIntfInd(stream, olt); err != nil {
		logger.Error("Fail to sendIntfInd: %v", err)
		return err
	}
	logger.Info("OLT %s sent IntfInd.", olt.Name)

	// OLT sends Operation Indication to Adapter after activating each interface
	//time.Sleep(IF_UP_TIME * time.Second)
	*olt.InternalState = device.PONIF_UP
	if err := sendOperInd(stream, olt); err != nil {
		logger.Error("Fail to sendOperInd: %v", err)
		return err
	}
	logger.Info("OLT %s sent OperInd.", olt.Name)

	// OLT sends ONU Discover Indication to Adapter after ONU discovery
	for intfid, _ := range s.Onumap {
		device.UpdateOnusOpStatus(intfid, s.Onumap[intfid], "up")
	}

	for intfid, _ := range s.Onumap {
		sendOnuDiscInd(stream, s.Onumap[intfid])
		logger.Info("OLT id:%d sent ONUDiscInd.", olt.ID)
	}

	// OLT Sends OnuInd after waiting all of those ONUs up
	for {
		if IsAllOnuActive(s.Onumap) {
			logger.Debug("All the Onus are Activated.")
			break
		}
	}

	for intfid, _ := range s.Onumap {
		sendOnuInd(stream, s.Onumap[intfid], s.IndInterval)
		logger.Info("OLT id:%d sent ONUInd.", olt.ID)
	}
	return nil
}

// Blocking
func (s *Server) StartPktInDaemon(ctx context.Context, stream openolt.Openolt_EnableIndicationServer) error {
	logger.Debug("StartPktInDaemon() Start")
	defer func() {
		RemoveVeths(s.Vethnames)
		s.Vethnames = []string{}
		s.Ioinfos = []*Ioinfo{}
		s.wg.Done()
		s.updateState(PRE_ACTIVE)
		logger.Debug("StartPktInDaemon() Done")
	}()
	s.wg.Add(1)
	ioinfos, veths, err := createIoinfos(s.Olt.ID, s.Vethnames, s.Onumap)
	if err != nil {
		logger.Error("createIoinfos failed.", err)
		return err
	}
	s.Ioinfos = ioinfos
	s.Vethnames = veths
	logger.Debug("Created vethnames:%v", s.Vethnames)

	parent := ctx
	child, cancel := context.WithCancel(parent)
	s.cancel = cancel

	if err = s.runPacketInDaemon(child, stream); err != nil {
		logger.Error("runPacketInDaemon failed.", err)
		return err
	}
	return nil
}

//Non-Blocking
func (s *Server) StopPktInDaemon() {
	if s.cancel != nil {
		cancel := s.cancel
		cancel()
	}
}

func createIoinfos(oltid uint32, Vethnames []string, onumap map[uint32][]*device.Onu) ([]*Ioinfo, []string, error) {
	ioinfos := []*Ioinfo{}
	var err error
	for intfid, _ := range onumap {
		for i := 0; i < len(onumap[intfid]); i++ {
			var handler *pcap.Handle
			onuid := onumap[intfid][i].OnuID
			uniup, unidw := makeUniName(oltid, intfid, onuid)
			if handler, Vethnames, err = setupVethHandler(uniup, unidw, Vethnames); err != nil {
				logger.Error("setupVethHandler failed (onuid: %d)", onuid, err)
				return ioinfos, Vethnames, err
			}
			iinfo := Ioinfo{Name: uniup, iotype: "uni", ioloc: "inside", intfid: intfid, onuid: onuid, handler: handler}
			ioinfos = append(ioinfos, &iinfo)
			oinfo := Ioinfo{Name: unidw, iotype: "uni", ioloc: "outside", intfid: intfid, onuid: onuid, handler: nil}
			ioinfos = append(ioinfos, &oinfo)
		}
	}

	var handler *pcap.Handle
	nniup, nnidw := makeNniName(oltid)
	if handler, Vethnames, err = setupVethHandler(nniup, nnidw, Vethnames); err != nil {
		logger.Error("setupVethHandler failed for nni", err)
		return ioinfos, Vethnames, err
	}

	iinfo := Ioinfo{Name: nnidw, iotype: "nni", ioloc: "inside", intfid: 1, handler: handler}
	ioinfos = append(ioinfos, &iinfo)
	oinfo := Ioinfo{Name: nniup, iotype: "nni", ioloc: "outside", intfid: 1, handler: nil}
	ioinfos = append(ioinfos, &oinfo)
	return ioinfos, Vethnames, nil
}

//Blocking
func (s *Server) runPacketInDaemon(ctx context.Context, stream openolt.Openolt_EnableIndicationServer) error {
	logger.Debug("runPacketInDaemon Start")
	defer logger.Debug("runPacketInDaemon Done")
	unichannel := make(chan Packet, 2048)

	for intfid, _ := range s.Onumap {
		for _, onu := range s.Onumap[intfid] {
			onuid := onu.OnuID
			ioinfo, err := s.identifyUniIoinfo("inside", intfid, onuid)
			if err != nil {
				utils.LoggerWithOnu(onu).Error("Fail to identifyUniIoinfo (onuid: %d): %v", onuid, err)
				return err
			}
			uhandler := ioinfo.handler
			go RecvWorker(ioinfo, uhandler, unichannel)
		}
	}

	ioinfo, err := s.IdentifyNniIoinfo("inside")
	if err != nil {
		return err
	}
	nhandler := ioinfo.handler
	nnichannel := make(chan Packet, 32)
	go RecvWorker(ioinfo, nhandler, nnichannel)

	data := &openolt.Indication_PktInd{}
	s.updateState(ACTIVE)
	for {
		select {
		case msg := <-s.omciIn:
			logger.Debug("OLT %d send omci indication, IF %v (ONU-ID: %v) pkt:%x.", s.Olt.ID, msg.IntfId, msg.OnuId, msg.Pkt)
			omci := &openolt.Indication_OmciInd{OmciInd: &msg}
			if err := stream.Send(&openolt.Indication{Data: omci}); err != nil {
				logger.Error("send omci indication failed.", err)
				continue
			}
		case unipkt := <-unichannel:
			logger.Debug("Received packet from UNI in grpc Server")
			if unipkt.Info == nil || unipkt.Info.iotype != "uni" {
				logger.Debug("WARNING: This packet does not come from UNI ")
				continue
			}

			intfid := unipkt.Info.intfid
			onuid := unipkt.Info.onuid
			gemid, _ := getGemPortID(intfid, onuid)
			pkt := unipkt.Pkt
			layerEth := pkt.Layer(layers.LayerTypeEthernet)
			le, _ := layerEth.(*layers.Ethernet)
			ethtype := le.EthernetType
			onu, _ := s.GetOnuByID(onuid)

			if ethtype == 0x888e {
				utils.LoggerWithOnu(onu).WithFields(log.Fields{
					"gemId": gemid,
				}).Info("Received upstream packet is EAPOL.")
			} else if layerDHCP := pkt.Layer(layers.LayerTypeDHCPv4); layerDHCP != nil {
				utils.LoggerWithOnu(onu).WithFields(log.Fields{
					"gemId": gemid,
				}).Info("Received upstream packet is DHCP.")

				//C-TAG
				sn := convB2S(onu.SerialNumber.VendorSpecific)
				if ctag, ok := s.CtagMap[sn]; ok == true {
					tagpkt, err := PushVLAN(pkt, uint16(ctag))
					if err != nil {
						utils.LoggerWithOnu(onu).WithFields(log.Fields{
							"gemId": gemid,
						}).Error("Fail to tag C-tag")
					} else {
						pkt = tagpkt
					}
				} else {
					utils.LoggerWithOnu(onu).WithFields(log.Fields{
						"gemId":   gemid,
						"cTagMap": s.CtagMap,
					}).Error("Could not find onuid in CtagMap", onuid, sn, s.CtagMap)
				}
			} else {
				utils.LoggerWithOnu(onu).WithFields(log.Fields{
					"gemId": gemid,
				}).Info("Received upstream packet is of unknow type, skipping.")
				continue
			}

			utils.LoggerWithOnu(onu).Info("sendPktInd - UNI Packet")
			data = &openolt.Indication_PktInd{PktInd: &openolt.PacketIndication{IntfType: "pon", IntfId: intfid, GemportId: gemid, Pkt: pkt.Data()}}
			if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
				logger.Error("Fail to send PktInd indication.", err)
				return err
			}

		case nnipkt := <-nnichannel:
			if nnipkt.Info == nil || nnipkt.Info.iotype != "nni" {
				logger.Debug("WARNING: This packet does not come from NNI ")
				continue
			}
			onuid := nnipkt.Info.onuid
			onu, _ := s.GetOnuByID(onuid)

			utils.LoggerWithOnu(onu).Info("Received packet from NNI in grpc Server.")
			intfid := nnipkt.Info.intfid
			pkt := nnipkt.Pkt
			utils.LoggerWithOnu(onu).Info("sendPktInd - NNI Packet")
			data = &openolt.Indication_PktInd{PktInd: &openolt.PacketIndication{IntfType: "nni", IntfId: intfid, Pkt: pkt.Data()}}
			if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
				logger.Error("Fail to send PktInd indication.", err)
				return err
			}

		case <-ctx.Done():
			logger.Debug("PacketInDaemon thread receives close ")
			close(unichannel)
			logger.Debug("Closed unichannel ")
			close(nnichannel)
			logger.Debug("Closed nnichannel ")
			return nil
		}
	}
	return nil
}

func (s *Server) onuPacketOut(intfid uint32, onuid uint32, rawpkt gopacket.Packet) error {
	layerEth := rawpkt.Layer(layers.LayerTypeEthernet)
	onu, _ := s.GetOnuByID(onuid)

	if layerEth != nil {
		pkt, _ := layerEth.(*layers.Ethernet)
		ethtype := pkt.EthernetType
		if ethtype == 0x888e {
			utils.LoggerWithOnu(onu).Info("Received downstream packet is EAPOL.")
		} else if layerDHCP := rawpkt.Layer(layers.LayerTypeDHCPv4); layerDHCP != nil {
			utils.LoggerWithOnu(onu).Info("Received downstream packet is DHCP.")
			rawpkt, _, _ = PopVLAN(rawpkt)
			rawpkt, _, _ = PopVLAN(rawpkt)
		} else {
			return nil
		}
		ioinfo, err := s.identifyUniIoinfo("inside", intfid, onuid)
		if err != nil {
			return err
		}
		handle := ioinfo.handler
		SendUni(handle, rawpkt)
		return nil
	}
	logger.Info("WARNING: Received packet is not supported")
	return nil
}

func (s *Server) uplinkPacketOut(rawpkt gopacket.Packet) error {
	poppkt, _, err := PopVLAN(rawpkt)
	poppkt, _, err = PopVLAN(poppkt)
	if err != nil {
		logger.Error("%s", err)
		return err
	}
	ioinfo, err := s.IdentifyNniIoinfo("inside")
	if err != nil {
		return err
	}
	handle := ioinfo.handler
	SendNni(handle, poppkt)
	return nil
}

func IsAllOnuActive(onumap map[uint32][]*device.Onu) bool {
	for _, onus := range onumap {
		for _, onu := range onus {
			if onu.GetIntStatus() != device.ONU_ACTIVATED {
				return false
			}
		}
	}
	return true
}

func getGemPortID(intfid uint32, onuid uint32) (uint32, error) {
	key := OnuKey{intfid, onuid}
	if onuState, ok := Onus[key]; !ok {
		idx := uint32(0)
		// Backwards compatible with bbsim_olt adapter
		return 1024 + (((MAX_ONUS_PER_PON*intfid + onuid - 1) * 7) + idx), nil
	} else {
		// FIXME - Gem Port ID is 2 bytes - fix openolt.proto
		return uint32(onuState.gemPortId), nil
	}
}

func getOnuBySN(onumap map[uint32][]*device.Onu, sn *openolt.SerialNumber) (*device.Onu, error) {
	for _, onus := range onumap {
		for _, onu := range onus {
			if device.ValidateSN(*sn, *onu.SerialNumber) {
				return onu, nil
			}
		}
	}
	err := errors.New("No mathced SN is found ")
	logger.Error("%s", err)
	return nil, err
}

func (s *Server) GetOnuByID(onuid uint32) (*device.Onu, error) {
	return getOnuByID(s.Onumap, onuid)
}

func getOnuByID(onumap map[uint32][]*device.Onu, onuid uint32) (*device.Onu, error) {
	for _, onus := range onumap {
		for _, onu := range onus {
			if onu.OnuID == onuid {
				return onu, nil
			}
		}
	}
	err := errors.New("No matched OnuID is found ")
	logger.WithFields(log.Fields{
		"onumap": onumap,
		"onuid":  onuid,
	}).Error(err)
	return nil, err
}

func convB2S(b []byte) string {
	s := ""
	for _, i := range b {
		s = s + strconv.FormatInt(int64(i/16), 16) + strconv.FormatInt(int64(i%16), 16)
	}
	return s
}
