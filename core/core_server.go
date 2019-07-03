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
	"encoding/hex"
	"errors"
	"reflect"
	"strconv"
	"sync"

	pb "gerrit.opencord.org/voltha-bbsim/api"
	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"gerrit.opencord.org/voltha-bbsim/device"
	flowHandler "gerrit.opencord.org/voltha-bbsim/flow"
	openolt "gerrit.opencord.org/voltha-bbsim/protos"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	omci "github.com/opencord/omci-sim"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

// Constants
const (
	NniVethNorthPfx    = "nni_north"
	NniVethSouthPfx    = "nni_south"
	MaxPonPorts        = 64
	MaxOnusPerPon      = 64 // This value should be the same with the value in AdapterPlatform class
	VendorIDLength     = 4
	SerialNumberLength = 12
	OpenOltStart       = "start"
	OpenOltStop        = "stop"
)

// Server structure consists of all the params required for BBsim.
type Server struct {
	wg              *sync.WaitGroup
	Olt             *device.Olt
	Onumap          map[uint32][]*device.Onu
	SNmap           sync.Map
	AutoONUActivate bool
	Ioinfos         []*Ioinfo
	gRPCserver      *grpc.Server
	gRPCAddress     string
	gRPCPort        uint32
	mgmtServer      *grpc.Server
	mgmtGrpcPort    uint32
	mgmtRestPort    uint32
	Vethnames       []string
	IndInterval     int
	Processes       []string
	EnableServer    *openolt.Openolt_EnableIndicationServer
	CtagMap         map[string]uint32
	cancel          context.CancelFunc
	stateRepCh      chan stateReport
	omciIn          chan openolt.OmciIndication
	omciOut         chan openolt.OmciMsg
	eapolIn         chan *byteMsg
	eapolOut        chan *byteMsg
	dhcpIn          chan *byteMsg
	dhcpOut         chan *byteMsg
	FlowMap         map[FlowKey]*openolt.Flow
	alarmCh         chan *openolt.Indication
	deviceActionCh  chan *pb.DeviceAction
	serverActionCh  chan string
}

// Packet structure
type Packet struct {
	Info *Ioinfo
	Pkt  gopacket.Packet
}

type byteMsg struct {
	IntfId uint32
	OnuId  uint32
	Byte   []byte
}

type stateReport struct {
	device  device.Device
	current device.DeviceState
	next    device.DeviceState
}

// FlowKey used for FlowMap key
type FlowKey struct {
	FlowID        uint32
	FlowDirection string
}

//Has options (OLT id, number onu ports) from mediator
// NewCore initialize OLT and ONU objects
func NewCore(opt *option) *Server {
	// TODO: make it decent
	oltid := opt.oltid
	npon := opt.npon
	if npon > MaxPonPorts {
		logger.Warn("Provided number of PON ports exceeds limit of %d", MaxPonPorts)
		logger.Info("Setting number of PON ports to %d", MaxPonPorts)
		npon = MaxPonPorts
	}
	nonus := opt.nonus
	if nonus > MaxOnusPerPon {
		logger.Warn("Provided number of ONUs per PON port exceeds limit of %d", MaxOnusPerPon)
		logger.Info("Setting number of ONUs per PON port to %d", MaxOnusPerPon)
		nonus = MaxOnusPerPon
	}
	s := Server{
		Olt:             device.NewOlt(oltid, npon, 1), // TODO nnni is to be taken from options
		Onumap:          make(map[uint32][]*device.Onu),
		Ioinfos:         []*Ioinfo{},
		gRPCAddress:     opt.address,
		gRPCPort:        opt.port,
		Vethnames:       []string{},
		IndInterval:     opt.intvl,
		AutoONUActivate: !opt.interactiveOnuActivation,
		Processes:       []string{},
		mgmtGrpcPort:    opt.mgmtGrpcPort,
		mgmtRestPort:    opt.mgmtRestPort,
		EnableServer:    nil,
		stateRepCh:      make(chan stateReport, 8),
		omciIn:          make(chan openolt.OmciIndication, 1024),
		omciOut:         make(chan openolt.OmciMsg, 1024),
		eapolIn:         make(chan *byteMsg, 1024),
		eapolOut:        make(chan *byteMsg, 1024),
		dhcpIn:          make(chan *byteMsg, 1024),
		dhcpOut:         make(chan *byteMsg, 1024),
		FlowMap:         make(map[FlowKey]*openolt.Flow),
		serverActionCh:  make(chan string),
	}
	logger.Info("OLT %d created: %v", s.Olt.ID, s.Olt)

	nnni := s.Olt.NumNniIntf
	logger.Info("OLT ID: %d was retrieved.", s.Olt.ID)
	logger.Info("OLT Serial-Number: %v", s.Olt.SerialNumber)
	// Creating Onu Map
	for intfid := uint32(0); intfid < npon; intfid++ {
		s.Onumap[intfid] = device.NewOnus(oltid, intfid, nonus, nnni)
	}

	logger.Debug("Onu Map:")
	for _, onus := range s.Onumap {
		for _, onu := range onus {
			logger.Debug("%+v", *onu)
		}
	}

	// TODO: To be fixed because it is hardcoded
	s.CtagMap = make(map[string]uint32)
	for i := 0; i < MaxOnusPerPon; i++ {
		oltid := s.Olt.ID
		intfid := uint32(1)
		sn := device.ConvB2S(device.NewSN(oltid, intfid, uint32(i)))
		s.CtagMap[sn] = uint32(900 + i) // This is hard coded for BBWF
	}

	flowHandler.InitializeFlowManager(s.Olt.ID)
	return &s
}

// Start starts the openolt gRPC server (blocking)
func (s *Server) Start() error {
	logger.Debug("Starting OpenOLT gRPC Server")
	defer func() {
		logger.Debug("OpenOLT gRPC Server Stopped")
	}()

	// Start Openolt gRPC server
	addressport := s.gRPCAddress + ":" + strconv.Itoa(int(s.gRPCPort))
	listener, gserver, err := NewGrpcServer(addressport)
	if err != nil {
		logger.Error("Failed to create gRPC server: %v", err)
		return err
	}
	s.gRPCserver = gserver
	openolt.RegisterOpenoltServer(gserver, s)
	if err := gserver.Serve(listener); err != nil {
		logger.Error("Failed to run gRPC server: %v", err)
		return err
	}
	return nil
}

// Stop stops the openolt gRPC servers (non-blocking).
func (s *Server) Stop() {
	logger.Debug("Stopping OpenOLT gRPC Server & PktLoops")
	defer logger.Debug("OpenOLT gRPC Server & PktLoops Stopped")

	if s.gRPCserver != nil {
		s.gRPCserver.Stop()
		logger.Debug("gRPCserver.Stop()")
	}

	s.StopPktLoops()
	return
}

func (s *Server) startMgmtServer(wg *sync.WaitGroup) {
	defer logger.Debug("Management api server exited")

	grpcAddressPort := s.gRPCAddress + ":" + strconv.Itoa(int(s.mgmtGrpcPort))
	restAddressPort := s.gRPCAddress + ":" + strconv.Itoa(int(s.mgmtRestPort))
	// Start rest gateway for BBSim server
	go StartRestGatewayService(grpcAddressPort, restAddressPort, wg)
	addressPort := s.gRPCAddress + ":" + strconv.Itoa(int(s.mgmtGrpcPort))

	listener, apiserver, err := NewMgmtAPIServer(addressPort)
	if err != nil {
		logger.Error("Unable to create management api server %v", err)
		return
	}

	s.mgmtServer = apiserver
	pb.RegisterBBSimServiceServer(apiserver, s)
	if e := apiserver.Serve(listener); e != nil {
		logger.Error("Failed to run management api server %v", e)
		return
	}

}

func (s *Server) stopMgmtServer() error {
	if s.mgmtServer != nil {
		s.mgmtServer.GracefulStop()
		logger.Debug("Management server stopped")
		return nil
	}
	return errors.New("can not stop management server, server not created")
}

// Enable invokes methods for activation of OLT and ONU (blocking)
func (s *Server) Enable(sv *openolt.Openolt_EnableIndicationServer) error {
	olt := s.Olt
	defer func() {
		olt.Initialize()
		// Below lines commented as we dont want to change the onu state on restart
		// for intfid := range s.Onumap {
		// 	for _, onu := range s.Onumap[intfid] {
		// 		onu.Initialize()
		// 	}
		// }
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

	errorchan := make(chan error, 5)
	go s.StartPktLoops(coreCtx, *sv, errorchan)

	if s.AutoONUActivate == true {
		// Initialize all ONUs
		for intfid := range s.Onumap {
			for _, onu := range s.Onumap[intfid] {
				onu.Initialize()
			}
		}
		// Activate all ONUs
		s.activateONUs(*sv, s.Onumap)
	}

	select {
	case err := <-errorchan:
		if err != nil {
			logger.Debug("Error: %v", err)
			return err
		}
	}

	return nil
}

// Disable stops packet loops (non-blocking)
func (s *Server) Disable() {
	defer func() {
		logger.Debug("Disable() Done")
	}()
	logger.Debug("Disable() Start")
	s.StopPktLoops()
}

func (s *Server) updateDevIntState(dev device.Device, state device.DeviceState) {
	logger.Debug("updateDevIntState called state:%d", state)
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

func (s *Server) updateOnuIntState(intfid uint32, onuid uint32, state device.DeviceState) error {
	onu, err := s.GetOnuByID(onuid, intfid)
	if err != nil {
		return err
	}
	s.updateDevIntState(onu, state)
	return nil
}

func (s *Server) activateOnu(onu *device.Onu) {
	snKey := stringifySerialNumber(onu.SerialNumber)
	s.SNmap.Store(snKey, onu)
	device.UpdateOnusOpStatus(onu.IntfID, onu, "up")

	err := sendOnuDiscInd(*s.EnableServer, onu)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	logger.Info("OLT id:%d sent ONUDiscInd.", s.Olt.ID)
	logger.Debug("activateONUs Entry in SNmap %v", snKey)
}

func (s *Server) activateONUs(stream openolt.Openolt_EnableIndicationServer, Onumap map[uint32][]*device.Onu) {
	// Add all ONUs to SerialNumber Map
	for intfid := range Onumap {
		for _, onu := range Onumap[intfid] {
			s.activateOnu(onu)
		}
	}
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
	if err := sendOperInd(stream, olt); err != nil {
		logger.Error("Fail to sendOperInd: %v", err)
		return err
	}
	logger.Info("OLT %s sent OperInd.", olt.Name)
	return nil
}

// StartPktLoops creates veth pairs and invokes runPktLoops (blocking)
func (s *Server) StartPktLoops(ctx context.Context, stream openolt.Openolt_EnableIndicationServer, errorchan chan error) {
	logger.Debug("StartPktLoops () Start")
	defer func() {
		RemoveVeths(s.Vethnames)
		s.Vethnames = []string{}
		s.Ioinfos = []*Ioinfo{}
		s.wg.Done()
		s.updateDevIntState(s.Olt, device.OLT_PREACTIVE)
		logger.Debug("StartPktLoops () Done")
	}()
	s.alarmCh = make(chan *openolt.Indication, 10)
	go startAlarmLoop(stream, s.alarmCh)
	go s.startDeviceActionLoop()
	s.wg.Add(1)
	ioinfos, veths, err := createIoinfos(s.Olt.ID, s.Vethnames)
	if err != nil {
		logger.Error("createIoinfos failed: %v", err)
		errorchan <- err
	}
	s.Ioinfos = ioinfos
	s.Vethnames = veths
	logger.Debug("Created vethnames: %v", s.Vethnames)

	parent := ctx
	child, cancel := context.WithCancel(parent)
	s.cancel = cancel

	if err = s.runPktLoops(child, stream); err != nil {
		logger.Error("runPktLoops failed: %v", err)
		errorchan <- err
	}
	errorchan <- nil
}

// StopPktLoops (non-blocking)
func (s *Server) StopPktLoops() {
	if s.cancel != nil {
		cancel := s.cancel
		cancel()
	}
}

func createIoinfos(oltid uint32, Vethnames []string) ([]*Ioinfo, []string, error) {
	ioinfos := []*Ioinfo{}
	var err error
	var handler *pcap.Handle
	nniup, nnidw := makeNniName(oltid)
	if handler, Vethnames, err = setupVethHandler(nniup, nnidw, Vethnames); err != nil {
		logger.Error("setupVethHandler failed for nni: %v", err)
		return ioinfos, Vethnames, err
	}

	iinfo := Ioinfo{Name: nnidw, iotype: "nni", ioloc: "inside", intfid: 1, handler: handler}
	ioinfos = append(ioinfos, &iinfo)
	oinfo := Ioinfo{Name: nniup, iotype: "nni", ioloc: "outside", intfid: 1, handler: nil}
	ioinfos = append(ioinfos, &oinfo)
	return ioinfos, Vethnames, nil
}

// Blocking
func (s *Server) runPktLoops(ctx context.Context, stream openolt.Openolt_EnableIndicationServer) error {
	logger.Debug("runPacketPktLoops Start")
	defer logger.Debug("runPacketLoops Done")

	errchOmci := make(chan error)
	s.RunOmciResponder(ctx, s.omciOut, s.omciIn, errchOmci)
	eg, child := errgroup.WithContext(ctx)
	child, cancel := context.WithCancel(child)

	errchEapol := make(chan error)
	RunEapolResponder(ctx, s.eapolOut, s.eapolIn, errchEapol)

	errchDhcp := make(chan error)
	RunDhcpResponder(ctx, s.dhcpOut, s.dhcpIn, errchDhcp)

	eg.Go(func() error {
		logger.Debug("runOMCIResponder Start")
		defer logger.Debug("runOMCIResponder Done")
		select {
		case v, ok := <-errchOmci: // Wait for OmciInitialization
			if ok { // Error
				logger.Error("Error happend in Omci: %s", v)
				return v
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
			if ok { // Error
				logger.Error("Error happend in Eapol:%s", v)
				return v
			}
		case <-child.Done():
			return nil
		}
		return nil
	})

	eg.Go(func() error {
		logger.Debug("runDhcpResponder Start")
		defer logger.Debug("runDhcpResponder Done")
		select {
		case v, ok := <-errchDhcp:
			if ok { // Error
				logger.Error("Error happend in Dhcp:%s", v)
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
	logger.Debug("runMainPktLoop Start")
	defer func() {
		logger.Debug("runMainPktLoop Done")
	}()
	ioinfo, err := s.IdentifyNniIoinfo("inside")
	if err != nil {
		return err
	}
	nhandler, nnichannel := ioinfo.handler, make(chan Packet, 32)
	go RecvWorker(ioinfo, nhandler, nnichannel)
	defer func() {
		close(nnichannel)
	}()
	logger.Debug("BEFORE OLT_ACTIVE")
	s.updateDevIntState(s.Olt, device.OLT_ACTIVE)
	logger.Debug("AFTER  OLT_ACTIVE")
	data := &openolt.Indication_PktInd{}
	for {
		select {
		case msg := <-s.omciIn:
			logger.Debug("OLT %d send omci indication, IF %v (ONU-ID: %v) pkt:%x.", s.Olt.ID, msg.IntfId, msg.OnuId, msg.Pkt)
			omci := &openolt.Indication_OmciInd{OmciInd: &msg}
			if err := stream.Send(&openolt.Indication{Data: omci}); err != nil {
				logger.Error("send omci indication failed: %v", err)
				continue
			}
		case msg := <-s.eapolIn:
			intfid := msg.IntfId
			onuid := msg.OnuId
			gemid, err := s.getGemPortID(intfid, onuid)
			if err != nil {
				logger.Error("Failed to getGemPortID intfid:%d onuid:%d", intfid, onuid)
				continue
			}

			logger.Debug("OLT %d send eapol packet in (upstream), IF %v (ONU-ID: %v) pkt: %x", s.Olt.ID, intfid, onuid, msg.Byte)

			data = &openolt.Indication_PktInd{PktInd: &openolt.PacketIndication{IntfType: "pon", IntfId: intfid, GemportId: gemid, Pkt: msg.Byte}}
			if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
				logger.Error("Fail to send EAPOL PktInd indication. %v", err)
				return err
			}
		case msg := <-s.dhcpIn: // TODO: We should put omciIn, eapolIn, dhcpIn toghether
			intfid := msg.IntfId
			onuid := msg.OnuId
			gemid, err := s.getGemPortID(intfid, onuid)
			bytes := msg.Byte
			pkt := gopacket.NewPacket(bytes, layers.LayerTypeEthernet, gopacket.Default)

			if err != nil {
				logger.Error("Failed to getGemPortID intfid:%d onuid:%d", intfid, onuid)
				continue
			}

			onu, err := s.GetOnuByID(onuid, intfid)
			if err != nil {
				logger.Error("Failed to GetOnuByID:%d", onuid)
				continue
			}
			sn := device.ConvB2S(onu.SerialNumber.VendorSpecific)
			if ctag, ok := s.CtagMap[sn]; ok == true {
				tagpkt, err := PushVLAN(pkt, uint16(ctag), onu)
				if err != nil {
					device.LoggerWithOnu(onu).WithFields(log.Fields{
						"gemId": gemid,
					}).Error("Fail to tag C-tag")
				} else {
					pkt = tagpkt
				}
			} else {
				device.LoggerWithOnu(onu).WithFields(log.Fields{
					"gemId":   gemid,
					"cTagMap": s.CtagMap,
				}).Error("Could not find onuid in CtagMap", onuid, sn, s.CtagMap)
			}

			logger.Debug("OLT %d send dhcp packet in (upstream), IF %v (ONU-ID: %v) pkt: %x", s.Olt.ID, intfid, onuid, pkt.Dump())

			data = &openolt.Indication_PktInd{PktInd: &openolt.PacketIndication{IntfType: "pon", IntfId: intfid, GemportId: gemid, Pkt: msg.Byte}}
			if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
				logger.Error("Fail to send DHCP PktInd indication: %v", err)
				return err
			}

		case nnipkt := <-nnichannel:
			logger.Debug("Received packet from NNI")
			if nnipkt.Info == nil || nnipkt.Info.iotype != "nni" {
				logger.Debug("WARNING: This packet does not come from NNI ")
				continue
			}

			onuid := nnipkt.Info.onuid
			intfid := nnipkt.Info.intfid
			onu, _ := s.GetOnuByID(onuid, intfid)

			device.LoggerWithOnu(onu).Info("Received packet from NNI in grpc Server.")

			pkt := nnipkt.Pkt
			data = &openolt.Indication_PktInd{PktInd: &openolt.PacketIndication{IntfType: "nni", IntfId: intfid, Pkt: pkt.Data()}}
			if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
				logger.Error("Fail to send PktInd indication: %v", err)
				return err
			}

		case <-ctx.Done():
			logger.Debug("Closed nnichannel ")
			return nil
		}
	}
}

func (s *Server) onuPacketOut(intfid uint32, onuid uint32, rawpkt gopacket.Packet) error {
	layerEth := rawpkt.Layer(layers.LayerTypeEthernet)
	onu, err := s.GetOnuByID(onuid, intfid)
	if err != nil {
		logger.Error("Failed processing onuPacketOut: %v", err)
		return err
	}

	if layerEth != nil {
		pkt, _ := layerEth.(*layers.Ethernet)
		ethtype := pkt.EthernetType
		if ethtype == layers.EthernetTypeEAPOL {
			device.LoggerWithOnu(onu).Info("Received downstream packet is EAPOL.")
			eapolPkt := byteMsg{IntfId: intfid, OnuId: onuid, Byte: rawpkt.Data()}
			s.eapolOut <- &eapolPkt
			return nil
		} else if layerDHCP := rawpkt.Layer(layers.LayerTypeDHCPv4); layerDHCP != nil {
			device.LoggerWithOnu(onu).WithFields(log.Fields{
				"payload": layerDHCP.LayerPayload(),
				"type":    layerDHCP.LayerType().String(),
			}).Info("Received downstream packet is DHCP.")
			rawpkt, _, _ = PopVLAN(rawpkt)
			rawpkt, _, _ = PopVLAN(rawpkt)
			logger.Debug("%s", rawpkt.Dump())
			dhcpPkt := byteMsg{IntfId: intfid, OnuId: onuid, Byte: rawpkt.Data()}
			s.dhcpOut <- &dhcpPkt
			return nil
		} else {
			device.LoggerWithOnu(onu).Warn("WARNING: Received packet is not EAPOL or DHCP")
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
	device.LoggerWithOnu(onu).Info("WARNING: Received packet does not have layerEth")
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
	logger.Debug("%s", poppkt.Dump())
	SendNni(handle, poppkt)
	return nil
}

// IsAllOnuActive checks for ONU_ACTIVE state for all the onus in the map
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
		for _, onu := range onus {
			if onu.GetIntState() != device.ONU_OMCIACTIVE {
				return false
			}
		}
	}
	return true
}

func (s *Server) getGemPortID(intfid uint32, onuid uint32) (uint32, error) {
	logger.Debug("getGemPortID(intfid:%d, onuid:%d)", intfid, onuid)
	gemportid, err := omci.GetGemPortId(intfid, onuid)
	if err != nil {
		logger.Warn("Failed to getGemPortID from OMCI lib: %s", err)
		onu, err := s.GetOnuByID(onuid, intfid)
		if err != nil {
			logger.Error("Failed to getGemPortID: %s", err)
			return 0, err
		}
		gemportid = onu.GemportID
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
	err := errors.New("no matching serial number found")
	logger.Error("%s", err)
	return nil, err
}

// GetOnuByID returns ONU object as per onuID and intfID
func (s *Server) GetOnuByID(onuid uint32, intfid uint32) (*device.Onu, error) {
	return getOnuByID(s.Onumap, onuid, intfid)
}

func getOnuByID(onumap map[uint32][]*device.Onu, onuid uint32, intfid uint32) (*device.Onu, error) {
	for _, onu := range onumap[intfid] {
		if onu.OnuID == onuid {
			return onu, nil
		}
	}
	err := errors.New("no matching OnuID found")
	logger.WithFields(log.Fields{
		"onumap": onumap,
		"onuid":  onuid,
		"intfid": intfid,
	}).Error(err)
	return nil, err
}

// getOnuFromSNmap method returns onu object from SNmap if found
func (s *Server) getOnuFromSNmap(serialNumber *openolt.SerialNumber) (*device.Onu, bool) {
	snkey := stringifySerialNumber(serialNumber)

	logger.Debug("getOnuFromSNmap received serial number %s", snkey)

	if onu, exist := s.SNmap.Load(snkey); exist {
		logger.Info("Serial number found in map")
		return onu.(*device.Onu), true
	}
	logger.Info("Serial number not found in map")
	return nil, false
}

func stringifySerialNumber(serialNum *openolt.SerialNumber) string {
	return string(serialNum.VendorId) + device.ConvB2S(serialNum.VendorSpecific)
}

func getOpenoltSerialNumber(SerialNumber string) (*openolt.SerialNumber, error) {
	if len(SerialNumber) != SerialNumberLength {
		logger.Error("Invalid serial number %s", SerialNumber)
		return nil, errors.New("invalid serial number")
	}
	// First four characters are vendorId
	vendorID := SerialNumber[:VendorIDLength]
	vendorSpecific := SerialNumber[VendorIDLength:]

	vsbyte, _ := hex.DecodeString(vendorSpecific)

	// Convert to Openolt serial number
	serialNum := new(openolt.SerialNumber)
	serialNum.VendorId = []byte(vendorID)
	serialNum.VendorSpecific = vsbyte

	return serialNum, nil
}

// TODO move to device_onu.go
func (s *Server) sendOnuIndicationsOnOltReboot() {
	if AutoONUActivate == 1 {
		// For auto activate mode, onu indications is sent in Enable()
		return
	}

	s.SNmap.Range(
		func(key, value interface{}) bool {
			onu := value.(*device.Onu)
			if onu.InternalState == device.ONU_LOS_RAISED {
				return true
			}

			err := sendOnuDiscInd(*s.EnableServer, onu)
			if err != nil {
				logger.Error(err.Error())
			}

			return true
		})
}

// StartServerActionLoop reads on server-action channel, and starts and stops the server as per the value received
func (s *Server) StartServerActionLoop(wg *sync.WaitGroup) {
	for {
		select {
		case Req := <-s.serverActionCh:
			logger.Debug("Request Received On serverActionCh: %+v", Req)
			switch Req {
			case "start":
				logger.Debug("Server Start Request Received On ServerActionChannel")
				go s.Start() // blocking
			case "stop":
				logger.Debug("Server Stop Request Received On ServerActionChannel")
				s.Stop()
			default:
				logger.Error("Invalid value received in deviceActionCh")
			}
		}
	}
}

// startDeviceActionLoop reads on the action-channel, and performs onu and olt reboot related actions
// TODO all onu and olt related actions (like alarms) should be handled using this function
func (s *Server) startDeviceActionLoop() {
	logger.Debug("startDeviceActionLoop invoked")
	s.deviceActionCh = make(chan *pb.DeviceAction, 10)
	for {
		logger.Debug("Action channel loop started")
		select {
		case Req := <-s.deviceActionCh:
			logger.Debug("Reboot Action Type: %+v", Req.DeviceAction)
			switch Req.DeviceType {
			case DeviceTypeOnu:
				value, _ := s.SNmap.Load(Req.DeviceSerialNumber)
				onu := value.(*device.Onu)
				if Req.DeviceAction == SoftReboot {
					s.handleONUSoftReboot(onu.IntfID, onu.OnuID)
				} else if Req.DeviceAction == HardReboot {
					s.handleONUHardReboot(onu)
				}
			case DeviceTypeOlt:
				logger.Debug("Reboot For OLT Received")
				s.handleOLTReboot()
			default:
				logger.Error("Invalid value received in deviceActionCh")
			}
		}
	}
}
