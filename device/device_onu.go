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

package device

import (
	"reflect"
	"sync"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"gerrit.opencord.org/voltha-bbsim/protos"
	log "github.com/sirupsen/logrus"
)

const (
	ONU_INACTIVE   DeviceState = iota	//TODO: Each stage name should be more accurate
	ONU_ACTIVE
	ONU_OMCIACTIVE
	ONU_FREE
)

// Onu structure stores information of ONUs
type Onu struct {
	InternalState DeviceState
	OltID         uint32
	IntfID        uint32
	OperState     string
	SerialNumber  *openolt.SerialNumber
	OnuID         uint32
	GemportID     uint16
	mu            *sync.Mutex
}

// NewSN constructs and returns serial number based on the OLT ID, intf ID and ONU ID
func NewSN(oltid uint32, intfid uint32, onuid uint32) []byte {
	sn := []byte{0, byte(oltid % 256), byte(intfid), byte(onuid)}
	return sn
}

// NewOnus initializes and returns slice of Onu objects
func NewOnus(oltid uint32, intfid uint32, nonus uint32, nnni uint32) []*Onu {
	onus := []*Onu{}
	for i := 0; i < int(nonus); i++ {
		onu := Onu{}
		onu.InternalState = ONU_FREE
		onu.mu = &sync.Mutex{}
		onu.IntfID = intfid
		onu.OltID = oltid
		onu.OperState = "down"
		onu.SerialNumber = new(openolt.SerialNumber)
		onu.SerialNumber.VendorId = []byte("BBSM")
		onu.SerialNumber.VendorSpecific = NewSN(oltid, intfid, uint32(i))
		onu.GemportID = 0
		onus = append(onus, &onu)
	}
	return onus
}

// Initialize method initializes ONU state to up and ONU_INACTIVE
func (onu *Onu) Initialize() {
	onu.OperState = "up"
	onu.InternalState = ONU_INACTIVE
}

// ValidateONU method validate ONU based on the serial number in onuMap
func ValidateONU(targetonu openolt.Onu, regonus map[uint32][]*Onu) bool {
	for _, onus := range regonus {
		for _, onu := range onus {
			if ValidateSN(*targetonu.SerialNumber, *onu.SerialNumber) {
				return true
			}
		}
	}
	return false
}

// ValidateSN compares two serial numbers and returns result as true/false
func ValidateSN(sn1 openolt.SerialNumber, sn2 openolt.SerialNumber) bool {
	return reflect.DeepEqual(sn1.VendorId, sn2.VendorId) && reflect.DeepEqual(sn1.VendorSpecific, sn2.VendorSpecific)
}

// UpdateOnusOpStatus method updates ONU oper status
func UpdateOnusOpStatus(ponif uint32, onus []*Onu, opstatus string) {
	for _, onu := range onus {
		onu.OperState = opstatus
		logger.WithFields(log.Fields{
			"onu":           onu.SerialNumber,
			"pon_interface": ponif,
		}).Info("ONU discovered.")
	}
}

// UpdateIntState method updates ONU internal state
func (onu *Onu) UpdateIntState(intstate DeviceState) {
	onu.mu.Lock()
	defer onu.mu.Unlock()
	onu.InternalState = intstate
}

// GetDevkey returns ONU device key
func (onu *Onu) GetDevkey () Devkey {
	return Devkey{ID: onu.OnuID, Intfid:onu.IntfID}
}

// GetIntState returns ONU internal state
func (onu *Onu) GetIntState() DeviceState {
	onu.mu.Lock()
	defer onu.mu.Unlock()
	return onu.InternalState
}
