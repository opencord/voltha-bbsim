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

package main

import (
	"./openolt"
	"log"
	"net"
	"google.golang.org/grpc"
	"fmt"
	"flag"
	"reflect"
	"strings"
	"strconv"
	"sync"
)

type server struct{
	olt olt
	onus map[uint32]map[uint32]*onu
}


type oltState int
type onuState int

const(
	PRE_ENABLE oltState = iota
	OLT_UP
	PONIF_UP
	ONU_DISCOVERED
)

const(
	ONU_PRE_ACTIVATED onuState = iota
	ONU_ACTIVATED
)

type olt struct {
	ID			         uint32
	NumPonIntf           uint32
	NumNniIntf           uint32
	Mac                  string
	SerialNumber         string
	Manufacture          string
	Name                 string
	internalState        oltState
	OperState            string
	Intfs                []intf
	HeartbeatSignature   uint32
}

type intf struct{
	Type                string
	IntfID              uint32
	OperState           string
}

type onu struct{
	internalState       onuState
	IntfID              uint32
	OperState           string
	SerialNumber        *openolt.SerialNumber
}

func createOlt(oltid uint32, npon uint32, nnni uint32) olt{
	olt := olt {}
	olt.ID              = oltid
	olt.NumPonIntf      = npon
	olt.NumNniIntf      = nnni
	olt.Name          = "BBSIM OLT"
	olt.internalState = PRE_ENABLE
	olt.OperState     = "up"
	olt.Intfs = make([]intf, olt.NumPonIntf + olt.NumNniIntf)
	olt.HeartbeatSignature = oltid
	for i := uint32(0); i < olt.NumNniIntf; i ++ {
		olt.Intfs[i].IntfID = i
		olt.Intfs[i].OperState = "up"
		olt.Intfs[i].Type      = "nni"
	}
	for i := uint32(olt.NumNniIntf); i <  olt.NumPonIntf + olt.NumNniIntf; i ++ {
		olt.Intfs[i].IntfID = i
		olt.Intfs[i].OperState = "up"
		olt.Intfs[i].Type      = "pon"
	}
	return olt
}

func createSN(oltid uint32, intfid uint32, onuid uint32) string{
	sn := fmt.Sprintf("%X%X%02X", oltid, intfid, onuid)
	return sn
}

func createOnus(oltid uint32, intfid uint32, nonus uint32, nnni uint32) map[uint32] *onu {
	onus := make(map[uint32] *onu ,nonus)
	for onuid := uint32(1 + (intfid - nnni) * nonus); onuid <= (intfid - nnni + 1) * nonus; onuid ++ {
		onu := onu{}
		onu.internalState = ONU_PRE_ACTIVATED
		onu.IntfID        = intfid
		onu.OperState     = "up"
		onu.SerialNumber  = new(openolt.SerialNumber)
		onu.SerialNumber.VendorId = []byte("BRCM")
		onu.SerialNumber.VendorSpecific = []byte(createSN(oltid, intfid, uint32(onuid))) //FIX
		onus[onuid] = &onu
	}
	return onus
}

func validateONU(targetonu openolt.Onu, regonus map[uint32]map[uint32] *onu) bool{
	for _, onus := range regonus{
		for _, onu := range onus{
			if validateSN(*targetonu.SerialNumber, *onu.SerialNumber){
				return true
			}
		}
	}
	return false
}

func validateSN(sn1 openolt.SerialNumber, sn2 openolt.SerialNumber) bool{
	return reflect.DeepEqual(sn1.VendorId, sn2.VendorId) && reflect.DeepEqual(sn1.VendorSpecific, sn2.VendorSpecific)
}

func isAllONUActive(regonus map[uint32]map[uint32] *onu ) bool{
	for _, onus := range regonus{
		for _, onu := range onus{
			if onu.internalState != ONU_ACTIVATED{
				return false
			}
		}
	}
	return true
}

func updateOnusOpStatus(ponif uint32, onus map[uint32] *onu, opstatus string){
	for i, onu := range onus{
		onu.OperState = "up"
		log.Printf("(PONIF:%d) ONU [%d] discovered.\n", ponif, i)
	}
}

func activateOLT(s *server, stream openolt.Openolt_EnableIndicationServer) error{
	// Activate OLT
	olt  := &s.olt
	onus := s.onus
	if err := sendOltInd(stream, olt); err != nil {
		return err
	}
	olt.OperState = "up"
	olt.internalState = OLT_UP
	log.Printf("OLT %s sent OltInd.\n", olt.Name)


	// OLT sends Interface Indication to Adapter
	if err := sendIntfInd(stream, olt); err != nil {
		return err
	}
	log.Printf("OLT %s sent IntfInd.\n", olt.Name)

	// OLT sends Operation Indication to Adapter after activating each interface
	//time.Sleep(IF_UP_TIME * time.Second)
	olt.internalState = PONIF_UP
	if err := sendOperInd(stream, olt); err != nil {
		return err
	}
	log.Printf("OLT %s sent OperInd.\n", olt.Name)

	// OLT sends ONU Discover Indication to Adapter after ONU discovery
	for intfid := uint32(olt.NumNniIntf); intfid < olt.NumNniIntf + olt.NumPonIntf; intfid ++ {
		updateOnusOpStatus(intfid, onus[intfid], "up")
	}

	for intfid := uint32(olt.NumNniIntf); intfid < olt.NumNniIntf + olt.NumPonIntf; intfid ++ {
		sendOnuDiscInd(stream, onus[intfid])
		log.Printf("OLT id:%d sent ONUDiscInd.\n", olt.ID)
	}

	for{
		//log.Printf("stop %v\n", s.onus)
		if isAllONUActive(s.onus){
			//log.Printf("break! %v\n", s.onus)
			break
		}
	}
	for intfid := uint32(olt.NumNniIntf); intfid < olt.NumNniIntf + olt.NumPonIntf; intfid ++ {
		sendOnuInd(stream, onus[intfid])
		log.Printf("OLT id:%d sent ONUInd.\n", olt.ID)
	}
	return nil
}

func sendOltInd(stream openolt.Openolt_EnableIndicationServer, olt *olt) error{
	data := &openolt.Indication_OltInd{OltInd: &openolt.OltIndication{OperState: "up"}}
	if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
		log.Printf("Failed to send OLT indication: %v\n", err)
		return err
	}
	return nil
}

func sendIntfInd(stream openolt.Openolt_EnableIndicationServer, olt *olt) error{
	for i := uint32(0); i < olt.NumPonIntf + olt.NumNniIntf; i ++ {
		intf := olt.Intfs[i]
		data := &openolt.Indication_IntfInd{&openolt.IntfIndication{IntfId: intf.IntfID, OperState: intf.OperState}}
		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			log.Printf("Failed to send Intf [id: %d] indication : %v\n", i, err)
			return err
		}
		log.Printf("SendIntfInd olt:%d intf:%d (%s)\n", olt.ID, intf.IntfID, intf.Type)
	}
	return nil
}

func sendOperInd(stream openolt.Openolt_EnableIndicationServer, olt *olt) error{
	for i := uint32(0); i < olt.NumPonIntf + olt.NumNniIntf; i ++ {
		intf := olt.Intfs[i]
		data := &openolt.Indication_IntfOperInd{&openolt.IntfOperIndication{Type: intf.Type, IntfId: intf.IntfID, OperState: intf.OperState}}
		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			log.Printf("Failed to send IntfOper [id: %d] indication : %v\n", i, err)
			return err
		}
		log.Printf("SendOperInd olt:%d intf:%d (%s)\n", olt.ID, intf.IntfID, intf.Type)
	}
	return nil
}

func sendOnuDiscInd(stream openolt.Openolt_EnableIndicationServer, onus map[uint32] *onu) error{
	for i, onu := range onus {
		data := &openolt.Indication_OnuDiscInd{&openolt.OnuDiscIndication{IntfId: onu.IntfID, SerialNumber:onu.SerialNumber}}
		log.Printf("sendONUDiscInd Onuid: %d\n", i)
		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			log.Printf("Failed to send ONUDiscInd [id: %d]: %v\n", i, err)
			return err
		}
	}
	return nil
}

func sendOnuInd(stream openolt.Openolt_EnableIndicationServer, onus map[uint32] *onu) error{
	for i, onu := range onus {
		data := &openolt.Indication_OnuInd{&openolt.OnuIndication{IntfId: onu.IntfID, OnuId: uint32(i), OperState: "up", AdminState: "up", SerialNumber:onu.SerialNumber}}
		log.Printf("sendONUInd Onuid: %d\n", i)
		if err := stream.Send(&openolt.Indication{Data: data}); err != nil {
			log.Printf("Failed to send ONUInd [id: %d]: %v\n", i, err)
			return err
		}
	}
	return nil
}

func newServer(oltid uint32, npon uint32, nonus uint32) *server{
	s := new(server)
	s.olt  = createOlt(oltid, npon, 1)
	nnni := s.olt.NumNniIntf
	log.Printf("OLT ID: %d was retrieved.\n", s.olt.ID)

	s.onus = make(map[uint32]map[uint32] *onu)
	for intfid := nnni; intfid < npon + nnni; intfid ++ {
		s.onus[intfid] = createOnus(oltid, intfid, nonus, nnni)
	}
	//log.Printf("s.onus: %v\n", s.onus)
	return s
}

func printBanner(){
	log.Println("     ________    _______   ________                 ")
	log.Println("    / ____   | / ____   | / ______/  __            ")
	log.Println("   / /____/ / / /____/ / / /_____   /_/            ")
	log.Println("  / _____  | / _____  | /______  | __  __________ ")
	log.Println(" / /____/ / / /____/ / _______/ / / / / __  __  / ")
	log.Println("/________/ /________/ /________/ /_/ /_/ /_/ /_/  ")
}

func getOptions()(string, uint32, uint32, uint32, uint32){
	addressport := flag.String("H","127.0.0.1:50060","IP address:port")
	nolts    := flag.Int("N", 1, "Number of OLTs")
	nports   := flag.Int("i", 1, "Number of PON-IF ports")
	nonus    := flag.Int("n", 1, "Number of ONUs per PON-IF port")
	flag.Parse()
	fmt.Printf("%v\n", *addressport)
	//fmt.Println("nports:", *nports, "nonus:", *nonus)
	address  := strings.Split(*addressport, ":")[0]
	port,_ := strconv.Atoi(strings.Split(*addressport, ":")[1])
	return address, uint32(port), uint32(*nolts), uint32(*nports), uint32(*nonus)
}


func main() {
	printBanner()
	ipaddress, baseport, nolts, npon, nonus := getOptions()
	log.Printf("ip:%s, baseport:%d, nolts:%d, npon:%d, nonus:%d\n", ipaddress, baseport, nolts, npon, nonus)
	servers := make([] *server, nolts)
	grpcservers := make([] *grpc.Server, nolts)
	lis         := make([] net.Listener, nolts)
	var wg sync.WaitGroup
	wg.Add(int(nolts))
	for oltid := uint32(0); oltid < nolts; oltid ++ {
		portnum := baseport + oltid
		addressport := ipaddress + ":" + strconv.Itoa(int(portnum))
		log.Printf("Listening %s ...", addressport)
		lis[oltid], _ = net.Listen("tcp", addressport)
		servers[oltid] = newServer(oltid, npon, nonus)
		grpcservers[oltid] = grpc.NewServer()
		openolt.RegisterOpenoltServer(grpcservers[oltid], servers[oltid])
		go grpcservers[oltid].Serve(lis[oltid])
	}
	wg.Wait()
}
