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

import "sync"

type DeviceState int

type Device interface {
	Initialize()
	UpdateIntState(intstate DeviceState)
	GetIntState() DeviceState
	GetDevkey() Devkey
}

type Devkey struct {
	ID uint32
	Intfid uint32
}

type Olt struct {
	ID                 uint32
	NumPonIntf         uint32
	NumNniIntf         uint32
	Mac                string
	SerialNumber       string
	Manufacture        string
	Name               string
	InternalState      DeviceState
	OperState          string
	Intfs              []intf
	HeartbeatSignature uint32
	mu            *sync.Mutex
}

type intf struct {
	Type      string
	IntfID    uint32
	OperState string
}

/* OltState
OLT_INACTIVE -> OLT_PREACTIVE -> ACTIVE
    (ActivateOLT)   (Enable)
       <-              <-
*/

const (
	OLT_INACTIVE DeviceState  = iota // OLT/ONUs are not instantiated
	OLT_PREACTIVE        // Before PacketInDaemon Running
	OLT_ACTIVE            // After PacketInDaemon Running
)

func NewOlt(oltid uint32, npon uint32, nnni uint32) *Olt {
	olt := Olt{}
	olt.ID = oltid
	olt.NumPonIntf = npon
	olt.NumNniIntf = nnni
	olt.Name = "BBSIM OLT"
	olt.InternalState = OLT_INACTIVE
	olt.OperState = "up"
	olt.Intfs = make([]intf, olt.NumPonIntf+olt.NumNniIntf)
	olt.HeartbeatSignature = oltid
	olt.mu = &sync.Mutex{}
	for i := uint32(0); i < olt.NumNniIntf; i++ {
		olt.Intfs[i].IntfID = i
		olt.Intfs[i].OperState = "up"
		olt.Intfs[i].Type = "nni"
	}
	for i := uint32(olt.NumNniIntf); i < olt.NumPonIntf+olt.NumNniIntf; i++ {
		olt.Intfs[i].IntfID = i
		olt.Intfs[i].OperState = "up"
		olt.Intfs[i].Type = "pon"
	}
	return &olt
}

func (olt *Olt) Initialize() {
	olt.InternalState = OLT_INACTIVE
	olt.OperState = "up"
	for i := uint32(0); i < olt.NumNniIntf; i++ {
		olt.Intfs[i].IntfID = i
		olt.Intfs[i].OperState = "up"
		olt.Intfs[i].Type = "nni"
	}
	for i := uint32(olt.NumNniIntf); i < olt.NumPonIntf+olt.NumNniIntf; i++ {
		olt.Intfs[i].IntfID = i
		olt.Intfs[i].OperState = "up"
		olt.Intfs[i].Type = "pon"
	}
}

func (olt *Olt) GetIntState() DeviceState {
	olt.mu.Lock()
	defer olt.mu.Unlock()
	return olt.InternalState
}

func (olt *Olt) GetDevkey () Devkey {
	return Devkey{ID: olt.ID}
}

func (olt *Olt) UpdateIntState(intstate DeviceState) {
	olt.mu.Lock()
	defer olt.mu.Unlock()
	olt.InternalState = intstate
}