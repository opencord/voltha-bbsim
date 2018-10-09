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
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"log"
	"net"
)

func RecvWorker(io *Ioinfo, handler *pcap.Handle, r chan Packet) {
	log.Printf("recvWorker runs. handler: %v", *handler)
	packetSource := gopacket.NewPacketSource(handler, handler.LinkType())
	for packet := range packetSource.Packets() {
		log.Printf("recv packet from IF: %v \n", *handler)
		//log.Println(packet.Dump())
		pkt := Packet{}
		pkt.Info = io
		pkt.Pkt = packet
		r <- pkt
	}
}

func SendUni(handle *pcap.Handle, packet gopacket.Packet) {
	handle.WritePacketData(packet.Data())
	log.Printf("send packet to UNI-IF: %v \n", *handle)
	//log.Println(packet.Dump())
}

func SendNni(handle *pcap.Handle, packet gopacket.Packet) {
	handle.WritePacketData(packet.Data())
	log.Printf("send packet to NNI-IF: %v \n", *handle)
	//log.Println(packet.Dump())
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
			log.Printf("Pop the 802.1Q header (VID: %d)", vid)
			return retpkt, vid, nil
		}
	}
	//return pkt, 1, nil
	return nil, 0, errors.New("failed to pop vlan")
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
		log.Printf("Push the 802.1Q header (VID: %d)", vid)
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
