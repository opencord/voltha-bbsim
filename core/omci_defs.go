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
	"errors"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
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

func ParsePkt(pkt []byte) (uint16, uint8, OmciMsgType, OmciClass, uint16, OmciContent, error) {
	var m OmciMessage

	r := bytes.NewReader(pkt)

	if err := binary.Read(r, binary.BigEndian, &m); err != nil {
		logger.Error("binary.Read failed: %s", err)
		return 0, 0, 0, 0, 0, OmciContent{}, errors.New("binary.Read failed")
	}
	logger.Debug("OmciRun - TransactionId: %d MessageType: %d, ME Class: %d, ME Instance: %d, Content: %x",
		m.TransactionId, m.MessageType&0x0F, m.MessageId.Class, m.MessageId.Instance, m.Content)
	return m.TransactionId, m.DeviceId, m.MessageType & 0x0F, m.MessageId.Class, m.MessageId.Instance, m.Content, nil
}
