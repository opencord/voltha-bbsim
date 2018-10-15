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
	"gerrit.opencord.org/voltha-bbsim/device"
	"gerrit.opencord.org/voltha-bbsim/protos"
	"gerrit.opencord.org/voltha-bbsim/setup"
	"errors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"google.golang.org/grpc"
	"log"
	"strconv"
	"sync"
	"time"
)

type Mode int

const MAX_ONUS_PER_PON = 64 // This value should be the same with the value in AdapterPlatrorm class

const (
	DEFAULT Mode = iota
	AAA
	BOTH
)

type Server struct {
	Olt          *device.Olt
	Onumap       map[uint32][]*device.Onu
	Ioinfos      []*Ioinfo
	Endchan      chan int
	Mode         Mode
	AAAWait      int
	DhcpWait     int
	DhcpServerIP string
	gRPCserver   *grpc.Server
}

type Packet struct {
	Info *Ioinfo
	Pkt  gopacket.Packet
}

func CreateServer(oltid uint32, npon uint32, nonus uint32, aaawait int, dhcpwait int, ip string, g *grpc.Server, mode Mode, e chan int) *Server {
	s := new(Server)
	s.Olt = device.CreateOlt(oltid, npon, 1)
	nnni := s.Olt.NumNniIntf
	log.Printf("OLT ID: %d was retrieved.\n", s.Olt.ID)
	s.Onumap = make(map[uint32][]*device.Onu)
	s.AAAWait = aaawait
	s.DhcpWait = dhcpwait
	s.DhcpServerIP = ip
	s.gRPCserver = g
	s.Mode = mode
	s.Endchan = e
	for intfid := nnni; intfid < npon+nnni; intfid++ {
		s.Onumap[intfid] = device.CreateOnus(oltid, intfid, nonus, nnni)
	}
	return s
}

func (s *Server) activateOLT(stream openolt.Openolt_EnableIndicationServer) error {
	// Activate OLT
	olt := s.Olt
	oltid := olt.ID
	vethenv := []string{}
	wg := &sync.WaitGroup{}

	if err := sendOltInd(stream, olt); err != nil {
		return err
	}
	olt.OperState = "up"
	olt.InternalState = device.OLT_UP
	log.Printf("OLT %s sent OltInd.\n", olt.Name)

	// OLT sends Interface Indication to Adapter
	if err := sendIntfInd(stream, olt); err != nil {
		log.Printf("[ERROR] Fail to sendIntfInd: %v\n", err)
		return err
	}
	log.Printf("OLT %s sent IntfInd.\n", olt.Name)

	// OLT sends Operation Indication to Adapter after activating each interface
	//time.Sleep(IF_UP_TIME * time.Second)
	olt.InternalState = device.PONIF_UP
	if err := sendOperInd(stream, olt); err != nil {
		log.Printf("[ERROR] Fail to sendOperInd: %v\n", err)
		return err
	}
	log.Printf("OLT %s sent OperInd.\n", olt.Name)

	// OLT sends ONU Discover Indication to Adapter after ONU discovery
	for intfid, _ := range s.Onumap {
		device.UpdateOnusOpStatus(intfid, s.Onumap[intfid], "up")
	}

	for intfid, _ := range s.Onumap {
		sendOnuDiscInd(stream, s.Onumap[intfid])
		log.Printf("OLT id:%d sent ONUDiscInd.\n", olt.ID)
	}

	// OLT Sends OnuInd after waiting all of those ONUs up
	for {
		if s.IsAllONUActive() {
			break
		}
	}
	for intfid, _ := range s.Onumap {
		sendOnuInd(stream, s.Onumap[intfid])
		log.Printf("OLT id:%d sent ONUInd.\n", olt.ID)
	}

	if s.Mode == DEFAULT {
		//EnableIndication's stream should be kept even after activateOLT() is finished.
		//Otherwise, OpenOLT adapter sends EnableIndication again.
		<-s.Endchan
		log.Println("core server thread receives close !")
	} else if s.Mode == AAA || s.Mode == BOTH {
		var err error
		s.Ioinfos = []*Ioinfo{}
		for intfid, _ := range s.Onumap {
			for i := 0; i < len(s.Onumap[intfid]); i++ {
				var handler *pcap.Handle
				onuid := s.Onumap[intfid][i].OnuID
				uniup, unidw := makeUniName(oltid, intfid, onuid)
				if handler, vethenv, err = setupVethHandler(uniup, unidw, vethenv); err != nil {
					return err
				}
				iinfo := Ioinfo{name: uniup, iotype: "uni", ioloc: "inside", intfid: intfid, onuid: onuid, handler: handler}
				s.Ioinfos = append(s.Ioinfos, &iinfo)
				oinfo := Ioinfo{name: unidw, iotype: "uni", ioloc: "outside", intfid: intfid, onuid: onuid, handler: nil}
				s.Ioinfos = append(s.Ioinfos, &oinfo)
			}
		}
		var handler *pcap.Handle
		nniup, nnidw := makeNniName(oltid)
		if handler, vethenv, err = setupVethHandler(nniup, nnidw, vethenv); err != nil {
			return err
		}
		iinfo := Ioinfo{name: nnidw, iotype: "nni", ioloc: "inside", intfid: 1, handler: handler}
		s.Ioinfos = append(s.Ioinfos, &iinfo)
		oinfo := Ioinfo{name: nnidw, iotype: "nni", ioloc: "outside", intfid: 1, handler: nil}
		s.Ioinfos = append(s.Ioinfos, &oinfo)

		errchan := make(chan error)
		go func() {
			<-errchan
			close(s.Endchan)
		}()

		wg.Add(1)
		go func() {
			defer func() {
				log.Println("runPacketInDaemon Done")
				wg.Done()
			}()
			err := s.runPacketInDaemon(stream)
			if err != nil {
				errchan <- err
				return
			}
		}()

		wg.Add(1)
		go func() {
			defer func() {
				log.Println("exeAAATest Done")
				wg.Done()
			}()
			infos, err := s.getUniIoinfos("outside")
			if err != nil {
				errchan <- err
				return
			}
			univeths := []string{}
			for _, info := range infos {
				univeths = append(univeths, info.name)
			}
			err = s.exeAAATest(univeths)
			if err != nil {
				errchan <- err
				return
			}

			if s.Mode == BOTH {
				go func() {
					defer func() {
						log.Println("exeDHCPTest Done")
					}()
					info, err := s.identifyNniIoinfo("outside")
					setup.ActivateDHCPServer(info.name, s.DhcpServerIP)

					infos, err := s.getUniIoinfos("outside")
					if err != nil {
						errchan <- err
						return
					}
					univeths := []string{}
					for _, info := range infos {
						univeths = append(univeths, info.name)
					}
					err = s.exeDHCPTest(univeths)
					if err != nil {
						errchan <- err
						return
					}
				}()
			}
		}()
		wg.Wait()
		tearDown(vethenv) // Grace teardown
		log.Println("Grace shutdown down")
	}
	return nil
}

func (s *Server) runPacketInDaemon(stream openolt.Openolt_EnableIndicationServer) error {
	log.Println("runPacketInDaemon Start")
	unichannel := make(chan Packet, 2048)
	flag := false

	for intfid, _ := range s.Onumap {
		for _, onu := range s.Onumap[intfid] { //TODO: should be updated for multiple-Interface
			onuid := onu.OnuID
			ioinfo, err := s.identifyUniIoinfo("inside", intfid, onuid)
			if err != nil {
				log.Printf("[ERROR] Fail to identifyUniIoinfo (onuid: %d): %v\n", onuid, err)
				return err
			}
			uhandler := ioinfo.handler
			defer uhandler.Close()
			go RecvWorker(ioinfo, uhandler, unichannel)
		}
	}

	ioinfo, err := s.identifyNniIoinfo("inside")
	if err != nil {
		return err
	}
	nhandler := ioinfo.handler
	defer nhandler.Close()
	nnichannel := make(chan Packet, 32)
	go RecvWorker(ioinfo, nhandler, nnichannel)

	data := &openolt.Indication_PktInd{}
	for {
		select {
		case unipkt := <-unichannel:
			log.Println("Received packet in grpc Server from UNI.")
			if unipkt.Info == nil || unipkt.Info.iotype != "uni" {
				log.Println("[WARNING] This packet does not come from UNI !")
				continue
			}
			intfid := unipkt.Info.intfid
			onuid := unipkt.Info.onuid
			gemid, _ := getGemPortID(intfid, onuid)
			pkt := unipkt.Pkt
			layerEth := pkt.Layer(layers.LayerTypeEthernet)
			le, _ := layerEth.(*layers.Ethernet)
			ethtype := le.EthernetType
			if ethtype == 0x888e {
				log.Printf("Received upstream packet is EAPOL.")
				log.Println(unipkt.Pkt.Dump())
				log.Println(pkt.Dump())
			} else if layerDHCP := pkt.Layer(layers.LayerTypeDHCPv4); layerDHCP != nil {
				log.Printf("Received upstream packet is DHCP.")
				log.Println(unipkt.Pkt.Dump())
				log.Println(pkt.Dump())
			} else {
				continue
			}
			log.Printf("sendPktInd intfid:%d (onuid: %d) gemid:%d\n", intfid, onuid, gemid)
			data = &openolt.Indication_PktInd{PktInd: &openolt.PacketIndication{IntfType: "pon", IntfId: intfid, GemportId: gemid, Pkt: pkt.Data()}}
			if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
				log.Printf("[ERROR] Failed to send PktInd indication. %v\n", err)
				return err
			}
		case nnipkt := <-nnichannel:
			if nnipkt.Info == nil || nnipkt.Info.iotype != "nni" {
				log.Println("[WARNING] This packet does not come from NNI !")
				continue
			}
			log.Println("Received packet in grpc Server from NNI.")
			intfid := nnipkt.Info.intfid
			pkt := nnipkt.Pkt
			log.Printf("sendPktInd intfid:%d\n", intfid)
			data = &openolt.Indication_PktInd{PktInd: &openolt.PacketIndication{IntfType: "nni", IntfId: intfid, Pkt: pkt.Data()}}
			if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
				log.Printf("[ERROR] Failed to send PktInd indication. %v\n", err)
				return err
			}
		case <-s.Endchan:
			if flag == false {
				log.Println("PacketInDaemon thread receives close !")
				close(unichannel)
				log.Println("Closed unichannel !")
				close(nnichannel)
				log.Println("Closed nnichannel !")
				flag = true
				return nil
			}
		}
	}
	return nil
}

func (s *Server) exeAAATest(vethenv []string) error {
	log.Println("exeAAATest Start")
	for i := 0; i < s.AAAWait; i++ {
		select {
		case <-s.Endchan:
			log.Println("exeAAATest thread receives close !")
			return nil
		default:
			log.Println("exeAAATest is now sleeping....")
			time.Sleep(time.Second)
		}
	}
	err := setup.ActivateWPASups(vethenv)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) exeDHCPTest(vethenv []string) error {
	log.Println("exeDHCPTest Start")
	for i := 0; i < s.DhcpWait; i++ {
		select {
		case <-s.Endchan:
			log.Println("exeDHCPTest thread receives close !")
			return nil
		default:
			log.Println("exeDHCPTest is now sleeping....")
			time.Sleep(time.Second)
		}
	}
	err := setup.ActivateDHCPClients(vethenv)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) onuPacketOut(intfid uint32, onuid uint32, rawpkt gopacket.Packet) error {
	layerEth := rawpkt.Layer(layers.LayerTypeEthernet)
	if layerEth != nil {
		pkt, _ := layerEth.(*layers.Ethernet)
		ethtype := pkt.EthernetType
		if ethtype == 0x888e {
			log.Printf("Received downstream packet is EAPOL.")
			log.Println(rawpkt.Dump())
		} else if layerDHCP := rawpkt.Layer(layers.LayerTypeDHCPv4); layerDHCP != nil {
			log.Printf("Received downstream packet is DHCP.")
			log.Println(rawpkt.Dump())
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
	log.Printf("[WARNING] Received packet is not supported")
	return nil
}

func (s *Server) uplinkPacketOut(rawpkt gopacket.Packet) error {
	log.Println("")
	poppkt, _, err := PopVLAN(rawpkt)
	poppkt, _, err = PopVLAN(poppkt)
	if err != nil {
		log.Println(err)
		return err
	}
	ioinfo, err := s.identifyNniIoinfo("inside")
	if err != nil {
		return err
	}
	handle := ioinfo.handler
	SendNni(handle, poppkt)
	return nil
}

func (s *Server) IsAllONUActive() bool {
	for _, onus := range s.Onumap {
		for _, onu := range onus {
			if onu.InternalState != device.ONU_ACTIVATED {
				return false
			}
		}
	}
	return true
}

func getVID(onuid uint32) (uint16, error) {
	return uint16(onuid), nil
}

func getGemPortID(intfid uint32, onuid uint32) (uint32, error) {
	idx := uint32(0)
	return 1024 + (((MAX_ONUS_PER_PON*intfid + onuid - 1) * 7) + idx), nil
	//return uint32(1032 + 8 * (vid - 1)), nil
}

func (s *Server) getOnuBySN(sn *openolt.SerialNumber) (*device.Onu, error) {
	for _, onus := range s.Onumap {
		for _, onu := range onus {
			if device.ValidateSN(*sn, *onu.SerialNumber) {
				return onu, nil
			}
		}
	}
	err := errors.New("No mathced SN is found !")
	log.Println(err)
	return nil, err
}

func (s *Server) getOnuByID(onuid uint32) (*device.Onu, error) {
	for _, onus := range s.Onumap {
		for _, onu := range onus {
			if onu.OnuID == onuid {
				return onu, nil
			}
		}
	}
	err := errors.New("No matched OnuID is found !")
	log.Println(err)
	return nil, err
}

func makeUniName(oltid uint32, intfid uint32, onuid uint32) (upif string, dwif string) {
	upif = setup.UNI_VETH_UP_PFX + strconv.Itoa(int(oltid)) + "_" + strconv.Itoa(int(intfid)) + "_" + strconv.Itoa(int(onuid))
	dwif = setup.UNI_VETH_DW_PFX + strconv.Itoa(int(oltid)) + "_" + strconv.Itoa(int(intfid)) + "_" + strconv.Itoa(int(onuid))
	return
}

func makeNniName(oltid uint32) (upif string, dwif string) {
	upif = setup.NNI_VETH_UP_PFX + strconv.Itoa(int(oltid))
	dwif = setup.NNI_VETH_DW_PFX + strconv.Itoa(int(oltid))
	return
}

func tearDown(vethenv []string) error {
	log.Println("tearDown()")
	setup.KillAllWPASups()
	setup.KillAllDHCPClients()
	setup.TearVethDown(vethenv)
	return nil
}

func setupVethHandler(inveth string, outveth string, vethenv []string) (*pcap.Handle, []string, error) {
	log.Printf("setupVethHandler: %s and %s\n", inveth, outveth)
	err1 := setup.CreateVethPairs(inveth, outveth)
	vethenv = append(vethenv, inveth)
	if err1 != nil {
		setup.RemoveVeths(vethenv)
		return nil, vethenv, err1
	}
	handler, err2 := getVethHandler(inveth)
	if err2 != nil {
		setup.RemoveVeths(vethenv)
		return nil, vethenv, err2
	}
	return handler, vethenv, nil
}

func getVethHandler(vethname string) (*pcap.Handle, error) {
	var (
		device       string = vethname
		snapshot_len int32  = 1518
		promiscuous  bool   = false
		err          error
		timeout      time.Duration = pcap.BlockForever
	)
	handle, err := pcap.OpenLive(device, snapshot_len, promiscuous, timeout)
	if err != nil {
		return nil, err
	}
	log.Printf("Server handle created for %s\n", vethname)
	return handle, nil
}
