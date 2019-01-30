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
	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"net"
	"errors"
	"encoding/hex"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"crypto/md5"
	"sync"
)

type eapState int
const (
	START eapState = iota	//TODO: This state definition should support 802.1X
	RESPID
	RESPCHA
	SUCCESS
)



type responder struct {
	peers   map [key] *peerInstance
	eapolIn chan *EAPByte
}

type peerInstance struct{
	key      key
	srcaddr  *net.HardwareAddr
	version  uint8
	curId    uint8
	curState eapState
}

type key struct {
	intfid uint32
	onuid  uint32
}

var resp *responder
var once sync.Once
func getResponder () *responder {
	once.Do(func(){
		resp = &responder{peers: make(map[key] *peerInstance), eapolIn: nil}
	})
	return resp
}

func RunEapolResponder(ctx context.Context, eapolOut chan *EAPPkt, eapolIn chan *EAPByte, errch chan error) {
	responder := getResponder()
	responder.eapolIn = eapolIn
	peers := responder.peers
	
	go func() {
		logger.Debug("EAPOL response process starts")
		defer logger.Debug("EAPOL response process was done")
		for {
			select {
			case msg := <- eapolOut:
				logger.Debug("Received eapol from eapolOut")
				intfid := msg.IntfId
				onuid := msg.OnuId

				if peer, ok := peers[key{intfid: intfid, onuid:onuid}]; ok {
					logger.Debug("Key hit intfid:%d onuid: %d", intfid, onuid)
					curstate := peer.curState
					nextstate, err := peer.transitState(curstate, eapolIn, msg.Pkt)
					if err != nil {
						logger.Error("Failed to transitState: %s", err)
					}
					peer.curState = nextstate
				} else {
					logger.Error("Failed to find eapol peer instance intfid:%d onuid:%d", intfid, onuid)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func startPeer (intfid uint32, onuid uint32) error {
	peer := peerInstance{key: key{intfid: intfid, onuid: onuid},
						srcaddr: &net.HardwareAddr{0x2e, 0x60, 0x70, 0x13, 0x07, byte(onuid)},
						version: 1,
						curId: 0,
						curState: START}

	eap := peer.createEAPStart()
	bytes := peer.createEAPOL(eap)
	resp := getResponder()
	eapolIn := resp.eapolIn
	if err := peer.sendPkt(bytes, eapolIn); err != nil {
		return errors.New("Failed to send EAPStart")
	}
	logger.Debug("Sending EAPStart")
	logger.Debug(hex.Dump(bytes))
	//peers[key{intfid: intfid, onuid: onuid}] = &peer
	resp.peers[key{intfid: intfid, onuid: onuid}] = &peer
	return nil
}

func (p *peerInstance)transitState(cur eapState, omciIn chan *EAPByte, recvpkt gopacket.Packet) (next eapState, err error) {
	logger.Debug("currentState:%d", cur)
	eap, err := extractEAP(recvpkt)
	if err != nil {
		return cur, nil
	}
	if eap.Code == layers.EAPCodeRequest && eap.Type == layers.EAPTypeIdentity {
		logger.Debug("Received EAP-Request/Identity")
		logger.Debug(recvpkt.Dump())
		p.curId = eap.Id
		if cur == START {
			reseap := p.createEAPResID()
			pkt := p.createEAPOL(reseap)
			logger.Debug("Sending EAP-Response/Identity")
			if err != p.sendPkt(pkt, omciIn) {
				return cur, err
			}
			return RESPID, nil
		}
	} else if eap.Code == layers.EAPCodeRequest && eap.Type == layers.EAPTypeOTP {
		logger.Debug("Received EAP-Request/Challenge")
		logger.Debug(recvpkt.Dump())
		if cur == RESPID {
			p.curId = eap.Id
			resdata := getMD5Res (p.curId, eap)
			resdata = append([]byte{0x10}, resdata ...)
			reseap := p.createEAPResCha(resdata)
			pkt := p.createEAPOL(reseap)
			logger.Debug("Sending EAP-Response/Challenge")
			if err != p.sendPkt(pkt, omciIn) {
				return cur, err
			}
			return RESPCHA, nil
		}
	} else if eap.Code == layers.EAPCodeSuccess && eap.Type == layers.EAPTypeNone {
		logger.Debug("Received EAP-Success")
		logger.Debug(recvpkt.Dump())
		if cur == RESPCHA {
			return SUCCESS, nil
		}
	} else {
		logger.Debug("Received unsupported EAP")
		return cur, nil
	}
	logger.Debug("State transition does not support..current state:%d", cur)
	logger.Debug(recvpkt.Dump())
	return cur, nil
}

func (p *peerInstance) createEAPOL (eap *layers.EAP) []byte {
	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{}

	ethernetLayer := &layers.Ethernet{
		SrcMAC: *p.srcaddr,
		DstMAC: net.HardwareAddr{0x01, 0x80, 0xC2, 0x00, 0x00, 0x03},
		EthernetType: layers.EthernetTypeEAPOL,
	}

	if eap == nil {	// EAP Start
		gopacket.SerializeLayers(buffer, options,
			ethernetLayer,
			&layers.EAPOL{Version: p.version, Type:1, Length: 0},
		)
	} else {
		gopacket.SerializeLayers(buffer, options,
			ethernetLayer,
			&layers.EAPOL{Version: p.version, Type:0, Length: eap.Length},
			eap,
		)
	}
	bytes := buffer.Bytes()
	return bytes
}

func (p *peerInstance) createEAPStart () *layers.EAP {
	return nil
}

func (p *peerInstance) createEAPResID () *layers.EAP {
	eap := layers.EAP{Code: layers.EAPCodeResponse,
		Id: p.curId,
		Length: 9,
		Type: layers.EAPTypeIdentity,
		TypeData: []byte{0x75, 0x73, 0x65, 0x72 }}
	return &eap
}

func (p *peerInstance) createEAPResCha (payload []byte) *layers.EAP {
	eap := layers.EAP{Code: layers.EAPCodeResponse,
		Id: p.curId, Length: 22,
		Type: layers.EAPTypeOTP,
		TypeData: payload}
	return &eap
}

func (p *peerInstance) sendPkt (pkt []byte, omciIn chan *EAPByte) error {
	// Send our packet
	msg := EAPByte{IntfId: p.key.intfid,
					OnuId: p.key.onuid,
					Byte: pkt}
	omciIn <- &msg
	logger.Debug("sendPkt intfid:%d onuid:%d", p.key.intfid, p.key.onuid)
	logger.Debug(hex.Dump(msg.Byte))
	return nil
}

func getMD5Res (id uint8, eap *layers.EAP) []byte {
	i := byte(id)
	C := []byte(eap.BaseLayer.Contents)[6:]
	P := []byte{i, 0x70, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64} //"password"
	data := md5.Sum(append(P, C ...))
	ret := make([]byte, 16)
	for j := 0; j < 16; j ++ {
		ret[j] = data[j]
	}
	return ret
}

func extractEAPOL (pkt gopacket.Packet) (*layers.EAPOL, error) {
	layerEAPOL := pkt.Layer(layers.LayerTypeEAPOL)
	eapol, _ := layerEAPOL.(*layers.EAPOL)
	if eapol == nil {
		return nil, errors.New("Cannot extract EAPOL")
	}
	return eapol, nil
}

func extractEAP (pkt gopacket.Packet) (*layers.EAP, error) {
	layerEAP := pkt.Layer(layers.LayerTypeEAP)
	eap, _ := layerEAP.(*layers.EAP)
	if eap == nil {
		return nil, errors.New("Cannot extract EAP")
	}
	return eap, nil
}