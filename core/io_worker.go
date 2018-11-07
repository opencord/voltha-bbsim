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
	"errors"
	"net"
	"strconv"
	"time"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

func RecvWorker(io *Ioinfo, handler *pcap.Handle, r chan Packet) {
	logger.Debug("recvWorker runs. handler: %v", *handler)
	packetSource := gopacket.NewPacketSource(handler, handler.LinkType())
	for packet := range packetSource.Packets() {
		logger.Debug("recv packet from IF: %v \n", *handler)
		//logger.Println(packet.Dump())
		pkt := Packet{}
		pkt.Info = io
		pkt.Pkt = packet
		r <- pkt
	}
}

func SendUni(handle *pcap.Handle, packet gopacket.Packet) {
	err := handle.WritePacketData(packet.Data())
	if err != nil {
		logger.Error("Error in send packet to UNI-IF: %v e:%s\n", *handle, err)
	}
	logger.Debug("Successfully send packet to UNI-IF: %v \n", *handle)
	//logger.Println(packet.Dump())
}

func SendNni(handle *pcap.Handle, packet gopacket.Packet) {
	err := handle.WritePacketData(packet.Data())
	if err != nil {
		logger.Error("Error in send packet to NNI e:%s\n", err)
	}
	logger.Debug("send packet to NNI-IF: %v \n", *handle)
	//logger.Println(packet.Dump())
}

func PopVLAN(pkt gopacket.Packet) (gopacket.Packet, uint16, error) {
	if layer := getDot1QLayer(pkt); layer != nil {
		if eth := getEthernetLayer(pkt); eth != nil {
			ethernetLayer := &layers.Ethernet{
				SrcMAC:       eth.SrcMAC,
				DstMAC:       eth.DstMAC,
				EthernetType: layer.Type,
			}
			buffer := gopacket.NewSerializeBuffer()
			gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{},
				ethernetLayer,
				gopacket.Payload(layer.Payload),
			)
			retpkt := gopacket.NewPacket(
				buffer.Bytes(),
				layers.LayerTypeEthernet,
				gopacket.Default,
			)
			vid := uint16(4095 & layer.VLANIdentifier)
			logger.Debug("Pop the 802.1Q header (VID: %d)", vid)
			return retpkt, vid, nil
		}
	}
	return pkt, 0, nil
	//return nil, 0, errors.New("failed to pop vlan")
}

func PushVLAN(pkt gopacket.Packet, vid uint16) (gopacket.Packet, error) {
	if eth := getEthernetLayer(pkt); eth != nil {
		ethernetLayer := &layers.Ethernet{
			SrcMAC:       eth.SrcMAC,
			DstMAC:       eth.DstMAC,
			EthernetType: 0x8100,
		}
		dot1qLayer := &layers.Dot1Q{
			Type:           eth.EthernetType,
			VLANIdentifier: uint16(vid),
		}

		buffer := gopacket.NewSerializeBuffer()
		gopacket.SerializeLayers(
			buffer,
			gopacket.SerializeOptions{
				FixLengths: false,
			},
			ethernetLayer,
			dot1qLayer,
			gopacket.Payload(eth.Payload),
		)
		ret := gopacket.NewPacket(
			buffer.Bytes(),
			layers.LayerTypeEthernet,
			gopacket.Default,
		)
		logger.Debug("Push the 802.1Q header (VID: %d)", vid)
		return ret, nil
	}
	return nil, errors.New("failed to push vlan")
}

func getEthernetLayer(pkt gopacket.Packet) *layers.Ethernet {
	eth := &layers.Ethernet{}
	if ethLayer := pkt.Layer(layers.LayerTypeEthernet); ethLayer != nil {
		eth, _ = ethLayer.(*layers.Ethernet)
	}
	return eth
}
func getDot1QLayer(pkt gopacket.Packet) (dot1q *layers.Dot1Q) {
	if dot1qLayer := pkt.Layer(layers.LayerTypeDot1Q); dot1qLayer != nil {
		dot1q = dot1qLayer.(*layers.Dot1Q)
	}
	return dot1q
}

func getMacAddress(ifName string) net.HardwareAddr {
	var err error
	var netIf *net.Interface
	var hwAddr net.HardwareAddr
	if netIf, err = net.InterfaceByName(ifName); err == nil {
		hwAddr = netIf.HardwareAddr
	}
	return hwAddr
}

func makeUniName(oltid uint32, intfid uint32, onuid uint32) (upif string, dwif string) {
	upif = UNI_VETH_UP_PFX + strconv.Itoa(int(oltid)) + "_" + strconv.Itoa(int(intfid)) + "_" + strconv.Itoa(int(onuid))
	dwif = UNI_VETH_DW_PFX + strconv.Itoa(int(oltid)) + "_" + strconv.Itoa(int(intfid)) + "_" + strconv.Itoa(int(onuid))
	return
}

func makeNniName(oltid uint32) (upif string, dwif string) {
	upif = NNI_VETH_UP_PFX + strconv.Itoa(int(oltid))
	dwif = NNI_VETH_DW_PFX + strconv.Itoa(int(oltid))
	return
}

func setupVethHandler(inveth string, outveth string, vethnames []string) (*pcap.Handle, []string, error) {
	logger.Debug("SetupVethHandler(%s, %s) called ", inveth, outveth)
	err1 := CreateVethPairs(inveth, outveth)
	vethnames = append(vethnames, inveth)
	if err1 != nil {
		RemoveVeths(vethnames)
		return nil, vethnames, err1
	}
	handler, err2 := getVethHandler(inveth)
	if err2 != nil {
		RemoveVeths(vethnames)
		return nil, vethnames, err2
	}
	return handler, vethnames, nil
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
	logger.Debug("Server handle is created for %s\n", vethname)
	return handle, nil
}
