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

	"github.com/opencord/voltha-bbsim/common/logger"
	openolt "github.com/opencord/voltha-protos/go/openolt"
	techprofile "github.com/opencord/voltha-protos/go/tech_profile"
	log "github.com/sirupsen/logrus"
)

// Constants for the ONU states
const (
	OnuFree State = iota // TODO: Each stage name should be more accurate
	OnuInactive
	OnuLosRaised
	OnuLosOnOltPonLos
	OnuOmciChannelLosRaised
	OnuActive
	OnuOmciActive
	OnuAuthenticated
)

// ONUState maps int value of device state to string
var ONUState = map[State]string{
	OnuFree:                 "ONU_FREE",
	OnuInactive:             "ONU_INACTIVE",
	OnuLosRaised:            "ONU_LOS_RAISED",
	OnuLosOnOltPonLos:       "ONU_LOS_ON_OLT_PON_LOS",
	OnuOmciChannelLosRaised: "ONU_OMCI_CHANNEL_LOS_RAISED",
	OnuActive:               "ONU_ACTIVE",
	OnuOmciActive:           "ONU_OMCIACTIVE",
	OnuAuthenticated:        "ONU_AUTHENTICATED",
}

// FlowKey used for FlowMap key
type FlowKey struct {
	FlowID        uint32
	FlowDirection string
}

// Onu structure stores information of ONUs
type Onu struct {
	InternalState State
	OltID         uint32
	IntfID        uint32
	OperState     string
	SerialNumber  *openolt.SerialNumber
	OnuID         uint32
	GemPortMap    map[uint32][]uint32 // alloc-id is used as key and corresponding gem-ports are stored in slice
	Tconts        *techprofile.TrafficSchedulers
	Flows         []FlowKey
	mu            *sync.Mutex
}

// NewSN constructs and returns serial number based on the OLT ID, intf ID and ONU ID
func NewSN(oltid uint32, intfid uint32, onuid uint32) []byte {
	sn := []byte{0, byte(oltid % 256), byte(intfid), byte(onuid)}
	return sn
}

// NewOnus initializes and returns slice of Onu objects
func NewOnus(oltid uint32, intfid uint32, nonus uint32) []*Onu {
	var onus []*Onu
	for i := 1; i <= int(nonus); i++ {
		onu := Onu{}
		onu.InternalState = OnuFree // New Onu Initialised with state ONU_FREE
		onu.mu = &sync.Mutex{}
		onu.IntfID = intfid
		onu.OltID = oltid
		onu.OperState = "down"
		onu.SerialNumber = new(openolt.SerialNumber)
		onu.SerialNumber.VendorId = []byte("BBSM")
		onu.SerialNumber.VendorSpecific = NewSN(oltid, intfid, uint32(i))
		onu.GemPortMap = make(map[uint32][]uint32)
		onus = append(onus, &onu)
	}
	return onus
}

// Initialize method initializes ONU state to up and ONU_INACTIVE
func (onu *Onu) Initialize() {
	onu.OperState = "up"
	onu.InternalState = OnuInactive
}

// ValidateSN compares two serial numbers and returns result as true/false
func ValidateSN(sn1 openolt.SerialNumber, sn2 openolt.SerialNumber) bool {
	return reflect.DeepEqual(sn1.VendorId, sn2.VendorId) && reflect.DeepEqual(sn1.VendorSpecific, sn2.VendorSpecific)
}

// UpdateOnusOpStatus method updates ONU oper status
func UpdateOnusOpStatus(ponif uint32, onu *Onu, opstatus string) {
	onu.OperState = opstatus
	logger.WithFields(log.Fields{
		"onu":           onu.SerialNumber,
		"pon_interface": ponif,
	}).Info("ONU OperState Updated")
}

// UpdateIntState method updates ONU internal state
func (onu *Onu) UpdateIntState(intstate State) {
	onu.mu.Lock()
	defer onu.mu.Unlock()
	onu.InternalState = intstate
}

// GetDevkey returns ONU device key
func (onu *Onu) GetDevkey() Devkey {
	return Devkey{ID: onu.OnuID, Intfid: onu.IntfID}
}

// GetIntState returns ONU internal state
func (onu *Onu) GetIntState() State {
	onu.mu.Lock()
	defer onu.mu.Unlock()
	return onu.InternalState
}

// DeleteFlow method search and delete flowKey from the onu flows slice
func (onu *Onu) DeleteFlow(key FlowKey) {
	for pos, flowKey := range onu.Flows {
		if flowKey == key {
			// delete the flowKey by shifting all flowKeys by one
			onu.Flows = append(onu.Flows[:pos], onu.Flows[pos+1:]...)
			t := make([]FlowKey, len(onu.Flows))
			copy(t, onu.Flows)
			onu.Flows = t
			break
		}
	}
}
