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

type oltState int

type Olt struct {
	ID                 uint32
	NumPonIntf         uint32
	NumNniIntf         uint32
	Mac                string
	SerialNumber       string
	Manufacture        string
	Name               string
	InternalState      *oltState
	OperState          string
	Intfs              []intf
	HeartbeatSignature uint32
}

type intf struct {
	Type      string
	IntfID    uint32
	OperState string
}

const (
	PRE_ENABLE oltState = iota
	OLT_UP
	PONIF_UP
	ONU_DISCOVERED
)

func NewOlt(oltid uint32, npon uint32, nnni uint32) *Olt {
	olt := Olt{}
	olt.ID = oltid
	olt.NumPonIntf = npon
	olt.NumNniIntf = nnni
	olt.Name = "BBSIM OLT"
	olt.InternalState = new(oltState)
	*olt.InternalState = PRE_ENABLE
	olt.OperState = "up"
	olt.Intfs = make([]intf, olt.NumPonIntf+olt.NumNniIntf)
	olt.HeartbeatSignature = oltid
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

func (olt *Olt) InitializeStatus() {
	*olt.InternalState = PRE_ENABLE
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
