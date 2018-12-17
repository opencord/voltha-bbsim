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
	"bytes"
	"encoding/binary"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"gerrit.opencord.org/voltha-bbsim/protos"
	"gerrit.opencord.org/voltha-bbsim/device"
	"time"
	"context"
	"errors"
)

//
// OMCI definitions
//

// OmciMsgType represents a OMCI message-type
type OmciMsgType byte

const (
	// Message Types
	_                                 = iota
	Create                OmciMsgType = 4
	Delete                OmciMsgType = 6
	Set                   OmciMsgType = 8
	Get                   OmciMsgType = 9
	GetAllAlarms          OmciMsgType = 11
	GetAllAlarmsNext      OmciMsgType = 12
	MibUpload             OmciMsgType = 13
	MibUploadNext         OmciMsgType = 14
	MibReset              OmciMsgType = 15
	AlarmNotification     OmciMsgType = 16
	AttributeValueChange  OmciMsgType = 17
	Test                  OmciMsgType = 18
	StartSoftwareDownload OmciMsgType = 19
	DownloadSection       OmciMsgType = 20
	EndSoftwareDownload   OmciMsgType = 21
	ActivateSoftware      OmciMsgType = 22
	CommitSoftware        OmciMsgType = 23
	SynchronizeTime       OmciMsgType = 24
	Reboot                OmciMsgType = 25
	GetNext               OmciMsgType = 26
	TestResult            OmciMsgType = 27
	GetCurrentData        OmciMsgType = 28
	SetTable              OmciMsgType = 29 // Defined in Extended Message Set Only
)

const (
	// Managed Entity Class values
	GEMPortNetworkCTP OmciClass = 268
)

// OMCI Managed Entity Class
type OmciClass uint16

// OMCI Message Identifier
type OmciMessageIdentifier struct {
	Class    OmciClass
	Instance uint16
}

type OmciContent [32]byte

type OmciMessage struct {
	TransactionId uint16
	MessageType   OmciMsgType
	DeviceId      uint8
	MessageId     OmciMessageIdentifier
	Content       OmciContent
}

const NumMibUploads byte = 9

type OnuKey struct {
	IntfId, OnuId uint32
}

type OnuOmciState struct {
	gemPortId    uint16
	mibUploadCtr uint16
	uniGInstance uint8
	pptpInstance uint8
	init  istate
}

type istate int

const (
	INCOMPLETE istate = iota
	DONE
)

type OmciMsgHandler func(class OmciClass, content OmciContent, key OnuKey) ([]byte, error)

var Handlers = map[OmciMsgType]OmciMsgHandler{
	MibReset:      mibReset,
	MibUpload:     mibUpload,
	MibUploadNext: mibUploadNext,
	Set:           set,
	Create:        create,
	Get:           get,
	GetAllAlarms:  getAllAlarms,
}

var OnuOmciStateMap = map[OnuKey]*OnuOmciState{}

func OmciRun(ctx context.Context, omciOut chan openolt.OmciMsg, omciIn chan openolt.OmciIndication, onumap map[uint32][] *device.Onu, errch chan error) {
	go func() { //For monitoring the OMCI states
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		for {
			select{
			case <- t.C:
				logger.Debug("Monitor omci init state")
				if isAllOmciInitDone(onumap) {
					logger.Info("OmciRun - All the omci initialization wes done")
					close(errch)
					return
				}
			case <- ctx.Done():
				logger.Debug("Omci Monitoring process was done")
				return
			}
		}
	}()

	go func(){
		defer logger.Debug("Omci response process was done")
		for {
			var resp openolt.OmciIndication
			select{
				case m := <-omciOut:
					transactionId, deviceId, msgType, class, instance, content, err := ParsePkt(m.Pkt)
					if err != nil {
						errch <- err
						return
					}

					logger.Debug("OmciRun - transactionId: %d msgType: %d, ME Class: %d, ME Instance: %d",
						transactionId, msgType, class, instance)

					key := OnuKey{m.IntfId, m.OnuId}
					if _, ok := OnuOmciStateMap[key]; !ok {
						OnuOmciStateMap[key] = NewOnuOmciState()
					}

					if _, ok := Handlers[msgType]; !ok {
						logger.Warn("Ignore omci msg (msgType %d not handled)", msgType)
						continue
					}

					resp.Pkt, err = Handlers[msgType](class, content, key)
					if err != nil {
						errch <- err
						return
					}
					resp.Pkt[0] = byte(transactionId >> 8)
					resp.Pkt[1] = byte(transactionId & 0xFF)
					resp.Pkt[2] = 0x2<<4 | byte(msgType)
					resp.Pkt[3] = deviceId
					resp.IntfId = m.IntfId
					resp.OnuId = m.OnuId
					omciIn <- resp
				case <-ctx.Done():
					return
			}
		}
	}()
}

func ParsePkt(pkt []byte) (uint16, uint8, OmciMsgType, OmciClass, uint16, OmciContent, error) {
	var m OmciMessage

	r := bytes.NewReader(HexDecode(pkt))

	if err := binary.Read(r, binary.BigEndian, &m); err != nil {
		logger.Error("binary.Read failed: %s", err)
		return 0, 0, 0, 0, 0, OmciContent{}, errors.New("binary.Read failed")
	}
	logger.Debug("OmciRun - TransactionId: %d MessageType: %d, ME Class: %d, ME Instance: %d, Content: %x",
		m.TransactionId, m.MessageType&0x0F, m.MessageId.Class, m.MessageId.Instance, m.Content)
	return m.TransactionId, m.DeviceId, m.MessageType & 0x0F, m.MessageId.Class, m.MessageId.Instance, m.Content, nil
}

func HexDecode(pkt []byte) []byte {
	// Convert the hex encoding to binary
	// TODO - Change openolt adapter to send raw binary instead of hex encoded
	p := make([]byte, len(pkt)/2)
	for i, j := 0, 0; i < len(pkt); i, j = i+2, j+1 {
		// Go figure this ;)
		u := (pkt[i] & 15) + (pkt[i]>>6)*9
		l := (pkt[i+1] & 15) + (pkt[i+1]>>6)*9
		p[j] = u<<4 + l
	}
	logger.Debug("Omci decoded: %x.", p)
	return p
}

func NewOnuOmciState() *OnuOmciState {
	return &OnuOmciState{gemPortId: 0, mibUploadCtr: 0, uniGInstance: 1, pptpInstance: 1}
}

func mibReset(class OmciClass, content OmciContent, key OnuKey) ([]byte, error) {
	var pkt []byte

	logger.Debug("Omci MibReset")

	pkt = []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	return pkt, nil
}

func mibUpload(class OmciClass, content OmciContent, key OnuKey) ([]byte, error) {
	var pkt []byte

	logger.Debug("Omci MibUpload")

	pkt = []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	pkt[9] = NumMibUploads // Number of subsequent MibUploadNext cmds

	return pkt, nil
}

func mibUploadNext(class OmciClass, content OmciContent, key OnuKey) ([]byte, error) {
	var pkt []byte

	logger.Debug("Omci MibUploadNext")

	state := OnuOmciStateMap[key]

	switch state.mibUploadCtr {
	case 0:
		// ANI-G
		pkt = []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00,
			0x01, 0x07, 0x80, 0x01, 0xff, 0xff, 0x01, 0x00,
			0x08, 0x00, 0x30, 0x00, 0x00, 0x05, 0x09, 0x00,
			0x00, 0xe0, 0x54, 0xff, 0xff, 0x00, 0x00, 0x0c,
			0x63, 0x81, 0x81, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	case 1, 2, 3, 4:
		// UNI-G
		pkt = []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00,
			0x01, 0x08, 0x01, 0x01, 0xf8, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		pkt[11] = state.uniGInstance // ME Instance
		state.uniGInstance++
	case 5, 6, 7, 8:
		pkt = []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00,
			0x00, 0x0b, 0x01, 0x01, 0xff, 0xfe, 0x00, 0x2f,
			0x00, 0x00, 0x00, 0x00, 0x03, 0x05, 0xee, 0x00,
			0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		pkt[11] = state.pptpInstance // ME Instance
		state.pptpInstance++
	default:
		logger.Error("Invalid MibUpload request %d", state.mibUploadCtr)
		return nil, errors.New("Invalid MibUpload request")
	}

	state.mibUploadCtr++
	return pkt, nil
}

func set(class OmciClass, content OmciContent, key OnuKey) ([]byte, error) {
	var pkt []byte

	pkt = []byte{
		0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	logger.Debug("Omci Set")

	return pkt, nil
}

func create(class OmciClass, content OmciContent, key OnuKey) ([]byte, error) {
	var pkt []byte

	if class == GEMPortNetworkCTP {
		if onuOmciState, ok := OnuOmciStateMap[key]; !ok {
			logger.Error("ONU Key Error - IntfId: %d, OnuId:", key.IntfId, key.OnuId)
			return nil, errors.New("ONU Key Error")
		} else {
			onuOmciState.gemPortId = binary.BigEndian.Uint16(content[:2])
			logger.Debug("Gem Port Id %d", onuOmciState.gemPortId)
			OnuOmciStateMap[key].init = DONE
		}
	}

	pkt = []byte{
		0x00, 0x00, 0x00, 0x00, 0x01, 0x10, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	logger.Debug("Omci Create")

	return pkt, nil
}

func get(class OmciClass, content OmciContent, key OnuKey) ([]byte, error) {
	var pkt []byte

	pkt = []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x2d, 0x02, 0x01,
		0x00, 0x20, 0xc0, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	logger.Debug("Omci Get")

	return pkt, nil
}

func getAllAlarms(class OmciClass, content OmciContent, key OnuKey) ([]byte, error) {
	var pkt []byte

	pkt = []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00,
		0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	logger.Debug("Omci GetAllAlarms")

	return pkt, nil
}

func isAllOmciInitDone(onumap map[uint32][] *device.Onu) bool {
	for _, onus := range onumap {
		for _, onu := range onus{
			key := OnuKey{onu.IntfID, onu.OnuID}
			state := OnuOmciStateMap[key]
			if state.init == INCOMPLETE{
				return false
			}
		}
	}
	return true
}

