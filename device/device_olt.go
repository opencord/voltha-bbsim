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
	"strconv"
	"sync"
)

//State represents the OLT States
type State int

// Device interface provides common methods for OLT and ONU devices
type Device interface {
	Initialize()
	UpdateIntState(intstate State)
	GetIntState() State
	GetDevkey() Devkey
}

// Devkey key for OLT/ONU devices
type Devkey struct {
	ID     uint32
	Intfid uint32
}

// Olt structure consists required fields for OLT
type Olt struct {
	ID                 uint32
	NumPonIntf         uint32
	NumNniIntf         uint32
	Mac                string
	SerialNumber       string
	Manufacture        string
	Name               string
	InternalState      State
	OperState          string
	NniIntfs           []Port
	PonIntfs           []Port
	HeartbeatSignature uint32
	mu                 *sync.Mutex
}

// AlarmState informs about the present state of the supported alarms
type AlarmState uint32

const (
	// PonLosCleared alarm state for PON-LOS
	PonLosCleared AlarmState = iota
	// NniLosCleared alarm state for NNI-LOS
	NniLosCleared
	// PonLosRaised alarm state for PON-LOS
	PonLosRaised
	// NniLosRaised  for NNI-LOS
	NniLosRaised
)

// Port info for NNI and PON ports
type Port struct {
	Type       string
	IntfID     uint32
	OperState  string
	AlarmState AlarmState
	PortStats  PortStats
}

// PortStats for NNI and PON ports
type PortStats struct {
	Packets uint64
}

// Constants for Port types
const (
	IntfPon = "pon"
	IntfNni = "nni"
)

/* OltState
OLT_INACTIVE -> OLT_PREACTIVE -> ACTIVE
        (ActivateOLT)      (Enable)
       <-              <-
*/

// Constants for OLT states
const (
	OltInactive  State = iota // OLT/ONUs are not instantiated
	OltPreactive              // Before PacketInDaemon Running
	OltActive                 // After PacketInDaemon Running
)

// OLTAlarmStateToString is used to get alarm state as string
var OLTAlarmStateToString = map[AlarmState]string{
	PonLosCleared: "PonLosCleared",
	NniLosCleared: "NniLosCleared",
	PonLosRaised:  "PonLosRaised",
	NniLosRaised:  "NniLosRaised",
}

// NewOlt initialises the new olt variable with the given values
func NewOlt(oltid uint32, npon uint32, nnni uint32) *Olt {
	olt := Olt{}
	olt.ID = oltid
	olt.NumPonIntf = npon
	olt.NumNniIntf = nnni
	olt.Name = "BBSIM OLT"
	olt.InternalState = OltInactive
	olt.OperState = "up"
	olt.Manufacture = "BBSIM"
	olt.SerialNumber = "BBSIMOLT00" + strconv.FormatInt(int64(oltid), 10)
	olt.NniIntfs = make([]Port, olt.NumNniIntf)
	olt.PonIntfs = make([]Port, olt.NumPonIntf)
	olt.HeartbeatSignature = oltid
	olt.mu = &sync.Mutex{}
	for i := uint32(0); i < olt.NumNniIntf; i++ {
		olt.NniIntfs[i].IntfID = i
		olt.NniIntfs[i].OperState = "up"
		olt.NniIntfs[i].Type = IntfNni
		olt.NniIntfs[i].AlarmState = NniLosCleared
	}
	for i := uint32(0); i < olt.NumPonIntf; i++ {
		olt.PonIntfs[i].IntfID = i
		olt.PonIntfs[i].OperState = "up"
		olt.PonIntfs[i].Type = IntfPon
		olt.PonIntfs[i].AlarmState = PonLosCleared
	}
	return &olt
}

// Initialize method initializes NNI and PON ports
func (olt *Olt) Initialize() {
	olt.InternalState = OltInactive
	olt.OperState = "up"
	for i := uint32(0); i < olt.NumNniIntf; i++ {
		olt.NniIntfs[i].IntfID = i
		olt.NniIntfs[i].OperState = "up"
		olt.NniIntfs[i].Type = IntfNni
		olt.NniIntfs[i].AlarmState = NniLosCleared
	}
	for i := uint32(olt.NumNniIntf); i < olt.NumPonIntf; i++ {
		olt.PonIntfs[i].IntfID = i
		olt.PonIntfs[i].OperState = "up"
		olt.PonIntfs[i].Type = IntfPon
		olt.PonIntfs[i].AlarmState = PonLosCleared
	}
}

// GetIntState returns internal state of OLT
func (olt *Olt) GetIntState() State {
	olt.mu.Lock()
	defer olt.mu.Unlock()
	return olt.InternalState
}

// GetDevkey returns device key of OLT
func (olt *Olt) GetDevkey() Devkey {
	return Devkey{ID: olt.ID}
}

// UpdateIntState method updates OLT internal state
func (olt *Olt) UpdateIntState(intstate State) {
	olt.mu.Lock()
	defer olt.mu.Unlock()
	olt.InternalState = intstate
}

// UpdateNniPortState updates the status of the nni-port
func (olt *Olt) UpdateNniPortState(portID uint32, alarmState AlarmState, operState string) {
	olt.NniIntfs[portID].AlarmState = alarmState
	olt.NniIntfs[portID].OperState = operState
}

// UpdatePonPortState updates the status of the pon-port
func (olt *Olt) UpdatePonPortState(portID uint32, alarmState AlarmState, operState string) {
	olt.PonIntfs[portID].AlarmState = alarmState
	olt.PonIntfs[portID].OperState = operState
}
