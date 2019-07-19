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

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/opencord/voltha-bbsim/common/logger"
	"github.com/opencord/voltha-bbsim/device"
)

// RecvWorker receives the packet and forwards to the channel
func RecvWorker(io *Ioinfo, handler *pcap.Handle, r chan Packet) {
	logger.Debug("recvWorker runs. handler: %v", *handler)
	packetSource := gopacket.NewPacketSource(handler, handler.LinkType())
	for packet := range packetSource.Packets() {
		logger.Debug("recv packet from IF: %v", *handler)
		logger.Debug("Packet received %v", packet)
		// logger.Println(packet.Dump())
		pkt := Packet{}
		pkt.Info = io
		pkt.Pkt = packet
		r <- pkt
	}
}

// SendUni sends packet to UNI interface
func SendUni(handle *pcap.Handle, packet gopacket.Packet, onu *device.Onu) {
	err := handle.WritePacketData(packet.Data())
	if err != nil {
		device.LoggerWithOnu(onu).Errorf("Error in send packet to UNI-IF: %v e:%v", *handle, err)
	}
	device.LoggerWithOnu(onu).Debugf("Successfully send packet to UNI-IF: %v", *handle)
}

// SendNni sends packaet to NNI interface
func SendNni(handle *pcap.Handle, packet gopacket.Packet) {
	err := handle.WritePacketData(packet.Data())
	if err != nil {
		logger.Error("Error in send packet to NNI e:%s", err)
	}
	logger.Debug("send packet to NNI-IF: %v ", *handle)
	// logger.Println(packet.Dump())
}

// PopVLAN pops the vlan ans return packet
func PopVLAN(pkt gopacket.Packet) (gopacket.Packet, uint16, error) {
	if layer := getDot1QLayer(pkt); layer != nil {
		if eth := getEthernetLayer(pkt); eth != nil {
			ethernetLayer := &layers.Ethernet{
				SrcMAC:       eth.SrcMAC,
				DstMAC:       eth.DstMAC,
				EthernetType: layer.Type,
			}
			buffer := gopacket.NewSerializeBuffer()
			err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{},
				ethernetLayer,
				gopacket.Payload(layer.Payload),
			)
			if err != nil {
				logger.Error("%v", err)
			}

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
	return nil, 0, errors.New("failed to pop vlan")
}

// PushVLAN pushes the vlan header to the packet and returns tha packet
func PushVLAN(pkt gopacket.Packet, vid uint16, onu *device.Onu) (gopacket.Packet, error) {
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
		err := gopacket.SerializeLayers(
			buffer,
			gopacket.SerializeOptions{
				FixLengths: false,
			},
			ethernetLayer,
			dot1qLayer,
			gopacket.Payload(eth.Payload),
		)
		if err != nil {
			logger.Error("%v", err)
		}

		ret := gopacket.NewPacket(
			buffer.Bytes(),
			layers.LayerTypeEthernet,
			gopacket.Default,
		)
		device.LoggerWithOnu(onu).Debugf("Push the 802.1Q header (VID: %d)", vid)
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

func makeNniName(oltid uint32) (upif string, dwif string) {
	upif = NniVethNorthPfx + strconv.Itoa(int(oltid))
	dwif = NniVethSouthPfx + strconv.Itoa(int(oltid))
	return
}

func setupVethHandler(inveth string, outveth string, vethnames []string) (*pcap.Handle, []string, error) {
	logger.Debug("SetupVethHandler(%s, %s) called ", inveth, outveth)
	err1 := CreateVethPairs(inveth, outveth)
	vethnames = append(vethnames, inveth)
	if err1 != nil {
		logger.Error("setupVethHandler failed: %v", err1)
		RemoveVeths(vethnames)
		return nil, vethnames, err1
	}
	handler, err2 := getVethHandler(inveth)
	if err2 != nil {
		logger.Error("getVethHandler failed: %v", err2)
		RemoveVeths(vethnames)
		return nil, vethnames, err2
	}
	return handler, vethnames, nil
}

func getVethHandler(vethname string) (*pcap.Handle, error) {
	var (
		deviceName        = vethname
		snapshotLen int32 = 1518
		promiscuous       = false
		err         error
		timeout     = pcap.BlockForever
	)
	handle, err := pcap.OpenLive(deviceName, snapshotLen, promiscuous, timeout)
	if err != nil {
		return nil, err
	}
	logger.Debug("Server handle is created for %s", vethname)
	return handle, nil
}
