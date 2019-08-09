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
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/opencord/voltha-bbsim/common/logger"
)

type clientState int

// Constants for eapol states
const (
	EAP_START clientState = iota + 1 // TODO: This state definition should support 802.1X
	EAP_RESPID
	EAP_RESPCHA
	EAP_SUCCESS
)

func (eap clientState) String() string {
	return [...]string{"EAP_START", "EAP_RESPID", "EAP_RESPCHA", "EAP_SUCCESS"}[eap]
}

type eapResponder struct {
	clients map[clientKey]*eapClientInstance
	eapolIn chan *byteMsg
}

type clientInstance interface {
	transitState(cur clientState, recvbytes []byte) (next clientState, sendbytes []byte, err error)
	getState() clientState
	getKey() clientKey
}

type eapClientInstance struct {
	key      clientKey
	srcaddr  *net.HardwareAddr
	version  uint8
	curId    uint8
	curState clientState
}

type clientKey struct {
	intfid uint32
	onuid  uint32
}

var resp *eapResponder
var once sync.Once

func getEAPResponder() *eapResponder {
	once.Do(func() {
		resp = &eapResponder{clients: make(map[clientKey]*eapClientInstance), eapolIn: nil}
	})
	return resp
}

// RunEapolResponder starts go routine which processes and responds for received eapol messages
func RunEapolResponder(ctx context.Context, eapolOut chan *byteMsg, eapolIn chan *byteMsg, errch chan error) {
	responder := getEAPResponder()
	responder.eapolIn = eapolIn

	go func() {
		logger.Debug("EAPOL response process starts")
		defer logger.Debug("EAPOL response process was done")
		for {
			select {
			case msg := <-eapolOut:
				logger.Debug("Received eapol from eapolOut intfid:%d onuid:%d", msg.IntfId, msg.OnuId)
				responder := getEAPResponder()
				clients := responder.clients
				if c, ok := clients[clientKey{intfid: msg.IntfId, onuid: msg.OnuId}]; ok {
					logger.Debug("Got client intfid:%d onuid: %d (ClientID: %v)", c.key.intfid, c.key.onuid, c.curId)
					nextstate := respondMessage("EAPOL", *c, msg, eapolIn)
					c.updateState(nextstate)
				} else {
					logger.WithFields(log.Fields{
						"clients": clients,
					}).Errorf("Failed to find eapol client instance intfid:%d onuid:%d", msg.IntfId, msg.OnuId)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func respondMessage(msgtype string, client clientInstance, recvmsg *byteMsg, msgInCh chan *byteMsg) clientState {
	curstate := client.getState()
	nextstate, sendbytes, err := client.transitState(curstate, recvmsg.Byte)

	if err != nil {
		msg := fmt.Sprintf("Failed to transitState in %s: %s", msgtype, err)
		logger.Error(msg, err)
	}

	if sendbytes != nil {
		key := client.getKey()
		if err := sendBytes(key, sendbytes, msgInCh); err != nil {
			msg := fmt.Sprintf("Failed to sendBytes in %s: %s", msgtype, err)
			logger.Error(msg)
		}
	} else {
		logger.Debug("sendbytes is nil")
	}
	return nextstate
}

func sendEAPStart(intfid uint32, onuid uint32, client eapClientInstance, bytes []byte, eapolIn chan *byteMsg) error {
	for {
		responder := getEAPResponder()
		clients := responder.clients
		if c, ok := clients[clientKey{intfid: intfid, onuid: onuid}]; ok {
			if c.curState == EAP_SUCCESS {
				logger.WithFields(log.Fields{
					"int_id": intfid,
					"onu_id": onuid,
				}).Debug("EAP_SUCCESS received, stop retrying")
				break
			}
			// Reset state to EAP start
			c.updateState(EAP_START)
		} else {
			logger.WithFields(log.Fields{
				"clients": clients,
			}).Errorf("Failed to find eapol client instance intfid:%d onuid:%d (sendEAPStart)", intfid, onuid)
		}
		if err := sendBytes(clientKey{intfid, onuid}, bytes, eapolIn); err != nil {
			return errors.New("Failed to send EAPStart")
		}
		logger.WithFields(log.Fields{
			"int_id":  intfid,
			"onu_id":  onuid,
			"eapolIn": eapolIn,
			"bytes":   bytes,
		}).Debug("EAPStart Sent")
		time.Sleep(30 * time.Second)
	}
	return nil
}

func startEAPClient(intfid uint32, onuid uint32) error {
	client := eapClientInstance{key: clientKey{intfid: intfid, onuid: onuid},
		srcaddr:  &net.HardwareAddr{0x2e, 0x60, 0x70, 0x13, 0x07, byte(onuid)},
		version:  1,
		curId:    0,
		curState: EAP_START}

	eap := client.createEAPStart()
	bytes := client.createEAPOL(eap)
	resp := getEAPResponder()
	eapolIn := resp.eapolIn
	// start a loop that keeps sending EAPOL packets until it succeeds
	go sendEAPStart(intfid, onuid, client, bytes, eapolIn)
	// clients[key{intfid: intfid, onuid: onuid}] = &client
	resp.clients[clientKey{intfid: intfid, onuid: onuid}] = &client
	return nil
}

func (c eapClientInstance) transitState(cur clientState, recvbytes []byte) (next clientState, respbytes []byte, err error) {
	recvpkt := gopacket.NewPacket(recvbytes, layers.LayerTypeEthernet, gopacket.Default)
	eap, err := extractEAP(recvpkt)
	if err != nil {
		return cur, nil, nil
	}
	if eap.Code == layers.EAPCodeRequest && eap.Type == layers.EAPTypeIdentity {
		logger.Debug("Received EAP-Request/Identity")
		logger.Debug(recvpkt.Dump())
		c.curId = eap.Id
		if cur == EAP_START {
			reseap := c.createEAPResID()
			pkt := c.createEAPOL(reseap)
			logger.Debug("Moving from EAP_START to EAP_RESPID")
			return EAP_RESPID, pkt, nil
		}
	} else if eap.Code == layers.EAPCodeRequest && eap.Type == layers.EAPTypeOTP {
		logger.Debug("Received EAP-Request/Challenge")
		logger.Debug(recvpkt.Dump())
		if cur == EAP_RESPID {
			c.curId = eap.Id
			senddata := getMD5Data(c.curId, eap)
			senddata = append([]byte{0x10}, senddata...)
			sendeap := c.createEAPResCha(senddata)
			pkt := c.createEAPOL(sendeap)
			logger.Debug("Moving from EAP_RESPID to EAP_RESPCHA")
			return EAP_RESPCHA, pkt, nil
		}
	} else if eap.Code == layers.EAPCodeSuccess && eap.Type == layers.EAPTypeNone {
		logger.Debug("Received EAP-Success")
		logger.Debug(recvpkt.Dump())
		if cur == EAP_RESPCHA {
			logger.Debug("Moving from EAP_RESPCHA to EAP_SUCCESS")
			return EAP_SUCCESS, nil, nil
		}
	} else {
		logger.Debug("Received unsupported EAP")
		return cur, nil, nil
	}
	logger.Debug("State transition does not support..current state:%d", cur)
	logger.Debug(recvpkt.Dump())
	return cur, nil, nil
}

func (c eapClientInstance) getState() clientState {
	return c.curState
}

func (c *eapClientInstance) updateState(state clientState) {
	msg := fmt.Sprintf("EAP update state intfid:%d onuid:%d state:%d", c.key.intfid, c.key.onuid, state)
	logger.Debug(msg)
	c.curState = state
}

func (c eapClientInstance) getKey() clientKey {
	return c.key
}

func sendBytes(key clientKey, pkt []byte, chIn chan *byteMsg) error {
	// Send our packet
	msg := byteMsg{IntfId: key.intfid,
		OnuId: key.onuid,
		Byte:  pkt}
	chIn <- &msg
	logger.Debug("sendBytes intfid:%d onuid:%d", key.intfid, key.onuid)
	logger.Debug(hex.Dump(msg.Byte))
	return nil
}

func (c *eapClientInstance) createEAPOL(eap *layers.EAP) []byte {
	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{}

	ethernetLayer := &layers.Ethernet{
		SrcMAC:       *c.srcaddr,
		DstMAC:       net.HardwareAddr{0x01, 0x80, 0xC2, 0x00, 0x00, 0x03},
		EthernetType: layers.EthernetTypeEAPOL,
	}

	if eap == nil { // EAP Start
		gopacket.SerializeLayers(buffer, options,
			ethernetLayer,
			&layers.EAPOL{Version: c.version, Type: 1, Length: 0},
		)
	} else {
		gopacket.SerializeLayers(buffer, options,
			ethernetLayer,
			&layers.EAPOL{Version: c.version, Type: 0, Length: eap.Length},
			eap,
		)
	}
	bytes := buffer.Bytes()
	return bytes
}

func (c *eapClientInstance) createEAPStart() *layers.EAP {
	return nil
}

func (c *eapClientInstance) createEAPResID() *layers.EAP {
	eap := layers.EAP{Code: layers.EAPCodeResponse,
		Id:       c.curId,
		Length:   9,
		Type:     layers.EAPTypeIdentity,
		TypeData: []byte{0x75, 0x73, 0x65, 0x72}}
	return &eap
}

func (c *eapClientInstance) createEAPResCha(payload []byte) *layers.EAP {
	eap := layers.EAP{Code: layers.EAPCodeResponse,
		Id: c.curId, Length: 22,
		Type:     layers.EAPTypeOTP,
		TypeData: payload}
	return &eap
}

func getMD5Data(id uint8, eap *layers.EAP) []byte {
	i := byte(id)
	C := []byte(eap.BaseLayer.Contents)[6:]
	P := []byte{i, 0x70, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64} // "password"
	data := md5.Sum(append(P, C...))
	ret := make([]byte, 16)
	for j := 0; j < 16; j++ {
		ret[j] = data[j]
	}
	return ret
}

func extractEAPOL(pkt gopacket.Packet) (*layers.EAPOL, error) {
	layerEAPOL := pkt.Layer(layers.LayerTypeEAPOL)
	eapol, _ := layerEAPOL.(*layers.EAPOL)
	if eapol == nil {
		return nil, errors.New("Cannot extract EAPOL")
	}
	return eapol, nil
}

func extractEAP(pkt gopacket.Packet) (*layers.EAP, error) {
	layerEAP := pkt.Layer(layers.LayerTypeEAP)
	eap, _ := layerEAP.(*layers.EAP)
	if eap == nil {
		return nil, errors.New("Cannot extract EAP")
	}
	return eap, nil
}
