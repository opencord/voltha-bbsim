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

	omci "github.com/opencord/omci-sim"

	"reflect"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"gerrit.opencord.org/voltha-bbsim/common/utils"
	"gerrit.opencord.org/voltha-bbsim/device"
	"gerrit.opencord.org/voltha-bbsim/protos"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
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
	stateRepCh   chan stateReport
	omciIn       chan openolt.OmciIndication
	omciOut      chan openolt.OmciMsg
	eapolIn      chan *byteMsg
	eapolOut     chan *byteMsg
}

type Packet struct {
	Info *Ioinfo
	Pkt  gopacket.Packet
}

type byteMsg struct {
	IntfId uint32
	OnuId  uint32
	Byte []byte
}

type stateReport struct {
	device  device.Device
	current device.DeviceState
	next    device.DeviceState
}

func NewCore(opt *option) *Server {
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
		stateRepCh:   make(chan stateReport, 8),
		omciIn:       make(chan openolt.OmciIndication, 1024),
		omciOut:      make(chan openolt.OmciMsg, 1024),
		eapolIn:      make(chan *byteMsg, 1024),
		eapolOut:     make(chan *byteMsg, 1024),
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
		close(s.stateRepCh)
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
	s.StopPktLoops()
	return
}

// Blocking
func (s *Server) Enable(sv *openolt.Openolt_EnableIndicationServer) error {
	olt := s.Olt
	defer func() {
		olt.Initialize()
		for intfid, _ := range s.Onumap {
			for _, onu := range s.Onumap[intfid] {
				onu.Initialize()
			}
		}
		s.updateDevIntState(olt, device.OLT_INACTIVE)
		logger.Debug("Enable() Done")
	}()
	logger.Debug("Enable() Start")
	s.EnableServer = sv
	if err := s.activateOLT(*sv); err != nil {
		return err
	}
	s.updateDevIntState(olt, device.OLT_PREACTIVE)

	coreCtx := context.Background()
	coreCtx, corecancel := context.WithCancel(coreCtx)
	s.cancel = corecancel
	if err := s.StartPktLoops(coreCtx, *sv); err != nil {
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
	s.StopPktLoops()
}

func (s *Server) updateDevIntState(dev device.Device, state device.DeviceState) {
	current := dev.GetIntState()
	dev.UpdateIntState(state)
	s.stateRepCh <- stateReport{device: dev, current: current, next: state}
	if reflect.TypeOf(dev) == reflect.TypeOf(&device.Olt{}) {
		logger.Debug("OLT State updated to:%d", state)
	} else if reflect.TypeOf(dev) == reflect.TypeOf(&device.Onu{}) {
		logger.Debug("ONU State updated to:%d", state)
	} else {
		logger.Error("UpdateDevIntState () doesn't support this device: %s", reflect.TypeOf(dev))
	}
}

func (s *Server) updateOnuIntState (intfid uint32, onuid uint32, state device.DeviceState) error {
	onu, err := s.GetOnuByID(onuid)	//TODO: IntfID should be included ?
	if err != nil {
		return err
	}
	s.updateDevIntState(onu, state)
	return nil
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
	logger.Info("OLT %s sent OltInd.", olt.Name)

	// OLT sends Interface Indication to Adapter
	if err := sendIntfInd(stream, olt); err != nil {
		logger.Error("Fail to sendIntfInd: %v", err)
		return err
	}
	logger.Info("OLT %s sent IntfInd.", olt.Name)

	// OLT sends Operation Indication to Adapter after activating each interface
	//time.Sleep(IF_UP_TIME * time.Second)
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
func (s *Server) StartPktLoops(ctx context.Context, stream openolt.Openolt_EnableIndicationServer) error {
	logger.Debug("StartPktLoops () Start")
	defer func() {
		RemoveVeths(s.Vethnames)
		s.Vethnames = []string{}
		s.Ioinfos = []*Ioinfo{}
		s.wg.Done()
		s.updateDevIntState(s.Olt, device.OLT_PREACTIVE)
		logger.Debug("StartPktLoops () Done")
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

	if err = s.runPktLoops(child, stream); err != nil {
		logger.Error("runPktLoops failed.", err)
		return err
	}
	return nil
}

//Non-Blocking
func (s *Server) StopPktLoops() {
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
func (s *Server) runPktLoops(ctx context.Context, stream openolt.Openolt_EnableIndicationServer) error {
	logger.Debug("runPacketPktLoops Start")
	defer logger.Debug("runPacketLoops Done")

	errchOmci := make(chan error)
	RunOmciResponder(ctx, s.omciOut, s.omciIn, errchOmci)
	eg, child := errgroup.WithContext(ctx)
	child, cancel := context.WithCancel(child)

	errchEapol := make(chan error)
	RunEapolResponder(ctx, s.eapolOut, s.eapolIn, errchEapol)

	eg.Go(func() error {
		logger.Debug("runOMCIResponder Start")
		defer logger.Debug("runOMCIResponder Done")
		select {
		case v, ok := <-errchOmci: // Wait for OmciInitialization
			if ok { //Error
				logger.Error("Error happend in Omci:%s", v)
				return v
			} else { //Close
				s.updateDevIntState(s.Olt, device.OLT_ACTIVE)
			}
		case <-child.Done():
			return nil
		}
		return nil
	})

	eg.Go(func() error {
		logger.Debug("runEapolResponder Start")
		defer logger.Debug("runEapolResponder Done")
		select {
		case v, ok := <-errchEapol:
			if ok { //Error
				logger.Error("Error happend in Eapol:%s", v)
				return v
			}
		case <-child.Done():
			return nil
		}
		return nil
	})

	eg.Go(func() error {
		err := s.runMainPktLoop(child, stream)
		return err
	})

	if err := eg.Wait(); err != nil {
		logger.Error("Error happend in runPacketLoops:%s", err)
		cancel()
	}
	return nil
}

func (s *Server) runMainPktLoop(ctx context.Context, stream openolt.Openolt_EnableIndicationServer) error {
	unichannel := make(chan Packet, 2048)
	defer func() {
		close(unichannel)
		logger.Debug("Closed unichannel ")
		logger.Debug("runMainPktLoop Done")
	}()
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
	nhandler, nnichannel := ioinfo.handler, make(chan Packet, 32)
	go RecvWorker(ioinfo, nhandler, nnichannel)
	defer func() {
		close(nnichannel)
	}()

	data := &openolt.Indication_PktInd{}
	for {
		select {
		case msg := <-s.omciIn:
			logger.Debug("OLT %d send omci indication, IF %v (ONU-ID: %v) pkt:%x.", s.Olt.ID, msg.IntfId, msg.OnuId, msg.Pkt)
			omci := &openolt.Indication_OmciInd{OmciInd: &msg}
			if err := stream.Send(&openolt.Indication{Data: omci}); err != nil {
				logger.Error("send omci indication failed.", err)
				continue
			}
		case msg := <- s.eapolIn:
			intfid := msg.IntfId
			onuid := msg.OnuId
			gemid, err := getGemPortID(intfid, onuid)
			if err != nil {
				logger.Error("Failed to getGemPortID intfid:%d onuid:%d", intfid, onuid)
				continue
			}

			logger.Debug("OLT %d send eapol packet in (upstream), IF %v (ONU-ID: %v) pkt:%x.", s.Olt.ID, intfid, onuid)

			data = &openolt.Indication_PktInd{PktInd: &openolt.PacketIndication{IntfType: "pon", IntfId: intfid, GemportId: gemid, Pkt: msg.Byte}}
			if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
				logger.Error("Fail to send EAPOL PktInd indication.", err)
				return err
			}
		case unipkt := <-unichannel:
			onuid := unipkt.Info.onuid
			onu, _ := s.GetOnuByID(onuid)
			utils.LoggerWithOnu(onu).Debug("Received packet from UNI in grpc Server")
			if unipkt.Info == nil || unipkt.Info.iotype != "uni" {
				logger.Debug("WARNING: This packet does not come from UNI ")
				continue
			}

			intfid := unipkt.Info.intfid
			gemid, err := getGemPortID(intfid, onuid)
			if err != nil {
				continue
			}
			pkt := unipkt.Pkt
			layerEth := pkt.Layer(layers.LayerTypeEthernet)
			le, _ := layerEth.(*layers.Ethernet)
			ethtype := le.EthernetType

			if ethtype == layers.EthernetTypeEAPOL {
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
					tagpkt, err := PushVLAN(pkt, uint16(ctag), onu)
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
			logger.Debug("Received packet from NNI")
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
		if ethtype == layers.EthernetTypeEAPOL {
			utils.LoggerWithOnu(onu).Info("Received downstream packet is EAPOL.")
			eapolPkt := byteMsg{IntfId:intfid, OnuId:onuid, Byte: rawpkt.Data()}
			s.eapolOut <- &eapolPkt
			return nil
		} else if layerDHCP := rawpkt.Layer(layers.LayerTypeDHCPv4); layerDHCP != nil {
			utils.LoggerWithOnu(onu).WithFields(log.Fields{
				"payload": layerDHCP.LayerPayload(),
				"type":    layerDHCP.LayerType().String(),
			}).Info("Received downstream packet is DHCP.")
			rawpkt, _, _ = PopVLAN(rawpkt)
			rawpkt, _, _ = PopVLAN(rawpkt)
		} else {
			utils.LoggerWithOnu(onu).Info("WARNING: Received packet is not EAPOL or DHCP")
			return nil
		}
		ioinfo, err := s.identifyUniIoinfo("inside", intfid, onuid)
		if err != nil {
			return err
		}
		handle := ioinfo.handler
		SendUni(handle, rawpkt, onu)
		return nil
	}
	utils.LoggerWithOnu(onu).Info("WARNING: Received packet does not have layerEth")
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
			if onu.GetIntState() != device.ONU_ACTIVE {
				return false
			}
		}
	}
	return true
}

func (s *Server) isAllOnuOmciActive() bool {
	for _, onus := range s.Onumap {
		for _, onu := range onus{
			if onu.GetIntState() != device.ONU_OMCIACTIVE {
				return false
			}
		}
	}
	return true
}

func getGemPortID(intfid uint32, onuid uint32) (uint32, error) {
	logger.Debug("getGemPortID(intfid:%d, onuid:%d)", intfid, onuid)
	gemportid, err := omci.GetGemPortId(intfid, onuid)
	if err != nil {
		logger.Error("%s", err)
		return 0, err
	}
	return uint32(gemportid), nil
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
