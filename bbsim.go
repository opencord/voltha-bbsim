package main

import (
	"./openolt"
	"log"
	"net"
	"google.golang.org/grpc"
	"golang.org/x/net/context"
	"fmt"
	"flag"
	"reflect"
	"time"
	"strings"
	"strconv"
	"sync"
)

type server struct{
	olt olt
	onus map[uint32][]onu
}


type oltState int

const(
	PRE_ENABLE oltState = iota
	OLT_UP
	PONIF_UP
	ONU_DISCOVERED
)


//a
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
	for i := uint32(0); i < olt.NumPonIntf; i ++ {
		olt.Intfs[i].IntfID = i
		olt.Intfs[i].OperState = "up"
		olt.Intfs[i].Type      = "pon"
	}
	for i := uint32(olt.NumPonIntf); i <  olt.NumPonIntf + olt.NumNniIntf; i ++ {
		olt.Intfs[i].IntfID = i
		olt.Intfs[i].OperState = "up"
		olt.Intfs[i].Type      = "nni"
	}
	return olt
}

func createSN(oltid uint32, intfid uint32, onuid uint32) string{
	sn := fmt.Sprintf("%X%X%02X", oltid, intfid, onuid)
	return sn
}

func createOnus(oltid uint32, intfid uint32, nonus uint32) [] onu {
	onus := make([]onu ,nonus)
	for onuid := uint32(0); onuid < nonus; onuid ++ {
		onus[onuid].IntfID       = intfid
		onus[onuid].OperState    = "down"
		onus[onuid].SerialNumber = new(openolt.SerialNumber)
		onus[onuid].SerialNumber.VendorId = []byte("BRCM")
		onus[onuid].SerialNumber.VendorSpecific = []byte(createSN(oltid, intfid, uint32(onuid))) //FIX
	}
	return onus
}

func validateONU(targetonu openolt.Onu, regonus map[uint32][]onu) bool{
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


func updateOnusOpStatus(ponif uint32, onus [] onu, opstatus string){
	for i, onu := range onus{
		onu.OperState = "up"
		log.Printf("(PONIF:%d) ONU [%d] %v discovered.\n", ponif, i, onu.SerialNumber)
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
	for intfid := uint32(0); intfid < olt.NumPonIntf; intfid ++ {
		updateOnusOpStatus(intfid, onus[intfid], "up")
	}

	for intfid := uint32(0); intfid < olt.NumPonIntf; intfid ++ {
		sendOnuDiscInd(stream, onus[intfid])
		log.Printf("OLT id:%d sent ONUDiscInd.\n", olt.ID)
	}
	olt.internalState = ONU_DISCOVERED
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
	}
	return nil
}

func sendOnuDiscInd(stream openolt.Openolt_EnableIndicationServer, onus [] onu) error{
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

func newServer(oltid uint32, npon uint32, nonus uint32) *server{
	s := new(server)
	s.olt  = createOlt(oltid, npon, 1)
	log.Printf("OLT ID: %d was retrieved.\n", s.olt.ID)

	s.onus = make(map[uint32][]onu)
	for intfid := uint32(0); intfid < npon; intfid ++ {
		s.onus[intfid] = createOnus(oltid, intfid, nonus)
	}
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
	var(
		addressport = flag.String("H","172.17.0.1:50060","IP address:port")
		address  = strings.Split(*addressport, ":")[0]
		port,_ = strconv.Atoi(strings.Split(*addressport, ":")[1])
		nolts    = flag.Int("N", 1, "Number of OLTs")
		nports   = flag.Int("i", 1, "Number of PON-IF ports")
		nonus    = flag.Int("n", 1, "Number of ONUs per PON-IF port")
	)


	flag.Parse()
	//fmt.Println("nports:", *nports, "nonus:", *nonus)
	return address, uint32(port), uint32(*nolts), uint32(*nports), uint32(*nonus)
}


// gRPC Service
func (s *server) ActivateOnu(c context.Context, onu *openolt.Onu) (*openolt.Empty, error){
	log.Printf("OLT receives ActivateONU()")
	result := validateONU(*onu, s.onus)
	if result == true {
		log.Printf("ONU %d activated succesufully.\n", onu.OnuId)
	}
	return new(openolt.Empty), nil
}

func (s *server)OmciMsgOut(c context.Context, msg *openolt.OmciMsg)(*openolt.Empty, error){
	return new(openolt.Empty), nil
}

func (s *server) OnuPacketOut(c context.Context, packet *openolt.OnuPacket)(*openolt.Empty, error){
	return new(openolt.Empty), nil
}

func (s *server) FlowAdd(c context.Context, flow *openolt.Flow)(*openolt.Empty, error){
	return new(openolt.Empty), nil
}

func (s *server) EnableIndication(empty *openolt.Empty, stream openolt.Openolt_EnableIndicationServer) error {
	log.Printf("OLT receives EnableInd.\n")
	if err := activateOLT(s, stream); err != nil {
		log.Printf("Failed to activate OLT: %v\n", err)
		return err
	}
	for ;;{
		//if err := sendIntfInd(stream, &s.olt); err != nil{
		//	return err
		//}
		time.Sleep(1 * time.Second)
	}
	return nil
}

func (s *server) HeartbeatCheck(c context.Context, empty *openolt.Empty) (*openolt.Heartbeat, error){
	log.Printf("OLT receives HeartbeatCheck.\n")
	signature := new(openolt.Heartbeat)
	signature.HeartbeatSignature = s.olt.HeartbeatSignature
	return signature, nil
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
