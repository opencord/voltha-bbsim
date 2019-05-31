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
	"fmt"
	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"math/rand"
	"net"
	"reflect"
	"sync"
)

const (
	DHCP_INIT clientState = iota + 1
	DHCP_SELECTING
	DHCP_REQUESTING
	DHCP_BOUND
)

type dhcpResponder struct {
	clients map[clientKey]*dhcpClientInstance
	dhcpIn  chan *byteMsg
}

type dhcpClientInstance struct {
	key      clientKey
	srcaddr  *net.HardwareAddr
	srcIP    *net.IPAddr
	serverIP *net.IPAddr
	hostname string
	curId    uint32
	curState clientState
}

var dhcpresp *dhcpResponder
var dhcponce sync.Once

func getDHCPResponder() *dhcpResponder {
	dhcponce.Do(func() {
		dhcpresp = &dhcpResponder{clients: make(map[clientKey]*dhcpClientInstance), dhcpIn: nil}
	})
	return dhcpresp
}

var defaultParamsRequestList = []layers.DHCPOpt{
	layers.DHCPOptSubnetMask,
	layers.DHCPOptBroadcastAddr,
	layers.DHCPOptTimeOffset,
	layers.DHCPOptRouter,
	layers.DHCPOptDomainName,
	layers.DHCPOptDNS,
	layers.DHCPOptDomainSearch,
	layers.DHCPOptHostname,
	layers.DHCPOptNetBIOSTCPNS,
	layers.DHCPOptNetBIOSTCPScope,
	layers.DHCPOptInterfaceMTU,
	layers.DHCPOptClasslessStaticRoute,
	layers.DHCPOptNTPServers,
}

func RunDhcpResponder(ctx context.Context, dhcpOut chan *byteMsg, dhcpIn chan *byteMsg, errch chan error) {
	responder := getDHCPResponder()
	responder.dhcpIn = dhcpIn
	clients := responder.clients

	go func() {
		logger.Debug("DHCP response process starts")
		defer logger.Debug("DHCP response process was done")
		for {
			select {
			case msg := <-dhcpOut:
				logger.Debug("Received dhcp message from dhcpOut")

				if c, ok := clients[clientKey{intfid: msg.IntfId, onuid: msg.OnuId}]; ok {
					nextstate := respondMessage("DHCP", *c, msg, dhcpIn)
					c.updateState(nextstate)
				} else {
					logger.Error("Failed to find dhcp client instance intfid:%d onuid:%d", msg.IntfId, msg.OnuId)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func startDHCPClient(intfid uint32, onuid uint32) error {
	logger.Debug("startDHCPClient intfid:%d onuid:%d", intfid, onuid)
	client := dhcpClientInstance{key: clientKey{intfid: intfid, onuid: onuid},
		srcaddr:  &net.HardwareAddr{0x2e, 0x60, 0x70, 0x13, 0x07, byte(onuid)},
		hostname: "voltha",
		curId:    rand.Uint32(),
		curState: DHCP_INIT}

	dhcp := client.createDHCPDisc()
	bytes, err := client.createDHCP(dhcp)
	if err != nil {
		logger.Error("%s", err)
		return err
	}
	resp := getDHCPResponder()
	dhcpIn := resp.dhcpIn
	if err := client.sendBytes(bytes, dhcpIn); err != nil {
		logger.Error("Failed to send DHCP Discovery")
		return errors.New("Failed to send DHCP Discovery")
	}
	client.curState = DHCP_SELECTING
	logger.Debug("Sending DHCP Discovery intfid:%d onuid:%d", intfid, onuid)
	resp.clients[clientKey{intfid: intfid, onuid: onuid}] = &client
	return nil
}

func (c dhcpClientInstance) transitState(cur clientState, recvbytes []byte) (next clientState, sendbytes []byte, err error) {
	recvpkt := gopacket.NewPacket(recvbytes, layers.LayerTypeEthernet, gopacket.Default)
	dhcp, err := extractDHCP(recvpkt)
	if err != nil {
		return cur, nil, nil
	}
	msgType, err := getMsgType(dhcp)
	if err != nil {
		logger.Error("%s", err)
		return cur, nil, nil
	}
	if dhcp.Operation == layers.DHCPOpReply && msgType == layers.DHCPMsgTypeOffer {
		logger.Debug("Received DHCP Offer")
		logger.Debug(recvpkt.Dump())
		if cur == DHCP_SELECTING {
			senddhcp := c.createDHCPReq()
			sendbytes, err := c.createDHCP(senddhcp)
			if err != nil {
				logger.Debug("Failed to createDHCP")
				return cur, nil, err
			}
			return DHCP_REQUESTING, sendbytes, nil
		}
	} else if dhcp.Operation == layers.DHCPOpReply && msgType == layers.DHCPMsgTypeAck {
		logger.Debug("Received DHCP Ack")
		logger.Debug(recvpkt.Dump())
		if cur == DHCP_REQUESTING {
			return DHCP_BOUND, nil, nil
		}
	} else if dhcp.Operation == layers.DHCPOpReply && msgType == layers.DHCPMsgTypeRelease {
		if cur == DHCP_BOUND {
			senddhcp := c.createDHCPDisc()
			sendbytes, err := c.createDHCP(senddhcp)
			if err != nil {
				fmt.Println("Failed to createDHCP")
				return DHCP_INIT, nil, err
			}
			return DHCP_SELECTING, sendbytes, nil
		}
	} else {
		logger.Debug("Received unsupported DHCP message Operation:%d MsgType:%d", dhcp.Operation, msgType)
		return cur, nil, nil
	}
	logger.Error("State transition does not support..current state:%d", cur)
	logger.Debug(recvpkt.Dump())

	return cur, nil, nil
}

func (c dhcpClientInstance) getState() clientState {
	return c.curState
}

func (c *dhcpClientInstance) updateState(state clientState) {
	msg := fmt.Sprintf("DHCP update state intfid:%d onuid:%d state:%d", c.key.intfid, c.key.onuid, state)
	logger.Debug(msg)
	c.curState = state
}

func (c dhcpClientInstance) getKey() clientKey {
	return c.key
}

func (c *dhcpClientInstance) createDHCP(dhcp *layers.DHCPv4) ([]byte, error) {
	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	ethernetLayer := &layers.Ethernet{
		SrcMAC:       *c.srcaddr,
		DstMAC:       net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		EthernetType: layers.EthernetTypeIPv4,
	}

	ipLayer := &layers.IPv4{
		Version:  4,
		TOS:      0x10,
		TTL:      128,
		SrcIP:    []byte{0, 0, 0, 0},
		DstIP:    []byte{255, 255, 255, 255},
		Protocol: layers.IPProtocolUDP,
	}

	udpLayer := &layers.UDP{
		SrcPort: 68,
		DstPort: 67,
	}

	udpLayer.SetNetworkLayerForChecksum(ipLayer)
	if err := gopacket.SerializeLayers(buffer, options, ethernetLayer, ipLayer, udpLayer, dhcp); err != nil {
		return nil, err
	}

	bytes := buffer.Bytes()
	return bytes, nil
}

func (c *dhcpClientInstance) createDefaultDHCPReq() layers.DHCPv4 {
	return layers.DHCPv4{
		Operation:    layers.DHCPOpRequest,
		HardwareType: layers.LinkTypeEthernet,
		HardwareLen:  6,
		HardwareOpts: 0,
		Xid:          c.curId,
		ClientHWAddr: *c.srcaddr,
	}
}

func (c *dhcpClientInstance) createDefaultOpts() []layers.DHCPOption {
	hostname := []byte(c.hostname)
	opts := []layers.DHCPOption{}
	opts = append(opts, layers.DHCPOption{
		Type:   layers.DHCPOptHostname,
		Data:   hostname,
		Length: uint8(len(hostname)),
	})

	bytes := []byte{}
	for _, option := range defaultParamsRequestList {
		bytes = append(bytes, byte(option))
	}

	opts = append(opts, layers.DHCPOption{
		Type:   layers.DHCPOptParamsRequest,
		Data:   bytes,
		Length: uint8(len(bytes)),
	})
	return opts
}

func (c *dhcpClientInstance) createDHCPDisc() *layers.DHCPv4 {
	dhcpLayer := c.createDefaultDHCPReq()
	defaultOpts := c.createDefaultOpts()
	dhcpLayer.Options = append([]layers.DHCPOption{layers.DHCPOption{
		Type:   layers.DHCPOptMessageType,
		Data:   []byte{byte(layers.DHCPMsgTypeDiscover)},
		Length: 1,
	}}, defaultOpts...)

	return &dhcpLayer
}

func (c *dhcpClientInstance) createDHCPReq() *layers.DHCPv4 {
	dhcpLayer := c.createDefaultDHCPReq()
	defaultOpts := c.createDefaultOpts()
	dhcpLayer.Options = append(defaultOpts, layers.DHCPOption{
		Type:   layers.DHCPOptMessageType,
		Data:   []byte{byte(layers.DHCPMsgTypeRequest)},
		Length: 1,
	})

	data := []byte{182, 21, 0, 128}
	dhcpLayer.Options = append(dhcpLayer.Options, layers.DHCPOption{
		Type:   layers.DHCPOptServerID,
		Data:   data,
		Length: uint8(len(data)),
	})

	data = []byte{0xcd, 0x28, 0xcb, 0xcc, 0x00, 0x01, 0x00, 0x01,
		0x23, 0xed, 0x11, 0xec, 0x4e, 0xfc, 0xcd, 0x28, 0xcb, 0xcc}
	dhcpLayer.Options = append(dhcpLayer.Options, layers.DHCPOption{
		Type:   layers.DHCPOptClientID,
		Data:   data,
		Length: uint8(len(data)),
	})

	data = []byte{182, 21, 0, byte(c.key.onuid)}
	dhcpLayer.Options = append(dhcpLayer.Options, layers.DHCPOption{
		Type:   layers.DHCPOptRequestIP,
		Data:   data,
		Length: uint8(len(data)),
	})
	return &dhcpLayer
}

func (c *dhcpClientInstance) sendBytes(bytes []byte, dhcpIn chan *byteMsg) error {
	// Send our packet
	msg := byteMsg{IntfId: c.key.intfid,
		OnuId: c.key.onuid,
		Byte: bytes}
	dhcpIn <- &msg
	logger.Debug("sendBytes intfid:%d onuid:%d", c.key.intfid, c.key.onuid)
	logger.Debug(hex.Dump(msg.Byte))
	return nil
}

func extractDHCP(pkt gopacket.Packet) (*layers.DHCPv4, error) {
	layerDHCP := pkt.Layer(layers.LayerTypeDHCPv4)
	dhcp, _ := layerDHCP.(*layers.DHCPv4)
	if dhcp == nil {
		return nil, errors.New("Failed to extract DHCP")
	}
	return dhcp, nil
}

func getMsgType(dhcp *layers.DHCPv4) (layers.DHCPMsgType, error) {
	for _, option := range dhcp.Options {
		if option.Type == layers.DHCPOptMessageType {
			if reflect.DeepEqual(option.Data, []byte{byte(layers.DHCPMsgTypeOffer)}) {
				return layers.DHCPMsgTypeOffer, nil
			} else if reflect.DeepEqual(option.Data, []byte{byte(layers.DHCPMsgTypeAck)}) {
				return layers.DHCPMsgTypeAck, nil
			} else if reflect.DeepEqual(option.Data, []byte{byte(layers.DHCPMsgTypeRelease)}) {
				return layers.DHCPMsgTypeRelease, nil
			} else {
				msg := fmt.Sprintf("This type %x is not supported", option.Data)
				return 0, errors.New(msg)
			}
		}
	}
	return 0, errors.New("Failed to extract MsgType from dhcp")
}
