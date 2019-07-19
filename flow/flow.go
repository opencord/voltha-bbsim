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

package flow

import (
	"math/rand"
	"time"

	"github.com/google/gopacket"
	"github.com/opencord/voltha-bbsim/common/logger"
	"github.com/opencord/voltha-bbsim/device"
	openolt "github.com/opencord/voltha-protos/go/openolt"
	log "github.com/sirupsen/logrus"
)

var flowManager Manager

// Manager interface for common methods of controller
type Manager interface {
	AddFlow(flow *openolt.Flow) error
	DeleteFlow(flow *openolt.Flow) error
	DeleteAllFlows() error
	PortUp(portID uint32) error
	PortDown(portID uint32) error
	GetFlow(onuID uint32) ([]*openolt.Flow, error)
	InitializePacketInStream(s openolt.Openolt_EnableIndicationServer)
	PacketOut(packet gopacket.Packet, s string, u uint32) error
}

// DefaultFlowController empty struct
type DefaultFlowController struct {
}

// InitializeFlowManager starts godc controller
func InitializeFlowManager(OltID uint32) {
	// Initialize flow controller as per custom implementation
	logger.Debug("InitializeFlowManager for OLT %d", OltID)
	flowManager = InitializeDefaultFlowController()
	return
}

// InitializePacketInStream initializes the stream to send packets towards VOLTHA
func InitializePacketInStream(s openolt.Openolt_EnableIndicationServer) {
	flowManager.InitializePacketInStream(s)
}

// AddFlow abstracts actual implementation of flow addition
func AddFlow(flow *openolt.Flow) error {
	return flowManager.AddFlow(flow)
}

// DeleteFlow abstracts actual implementation of flow deletion
func DeleteFlow(flow *openolt.Flow) error {
	return flowManager.DeleteFlow(flow)
}

// DeleteAllFlows abstracts actual implementation of flow deletion
func DeleteAllFlows() error {
	return flowManager.DeleteAllFlows()
}

// PortUp abstracts actual implementation of port up
func PortUp(portID uint32) error {
	return flowManager.PortUp(portID)
}

// PortDown abstracts actual implementation of port down
func PortDown(portID uint32) error {
	return flowManager.PortDown(portID)
}

// PacketOut abstracts actual implementation of sending packet out
func PacketOut(packet gopacket.Packet, intfType string, intfID uint32) error {
	return flowManager.PacketOut(packet, intfType, intfID)
}

// GetPortStats return stats for specified interface
func GetPortStats(portStats *device.PortStats) *openolt.PortStatistics {

	// increment current packet count by random number
	pkts := portStats.Packets + uint64((rand.Intn(50)+1)*10)
	portStats.Packets = pkts
	logger.Info("Packet count %d", portStats.Packets)

	// fill all other stats based on packet count
	nextPortStats := &openolt.PortStatistics{
		RxBytes:        pkts * 64,
		RxPackets:      pkts,
		RxUcastPackets: pkts * 40 / 100,
		RxMcastPackets: pkts * 30 / 100,
		RxBcastPackets: pkts * 30 / 100,
		RxErrorPackets: 0,
		TxBytes:        pkts * 64,
		TxPackets:      pkts,
		TxUcastPackets: pkts * 40 / 100,
		TxMcastPackets: pkts * 30 / 100,
		TxBcastPackets: pkts * 30 / 100,
		TxErrorPackets: 0,
		RxCrcErrors:    0,
		BipErrors:      0,
		Timestamp:      uint32(time.Now().Unix()),
	}

	return nextPortStats
}

// InitializeDefaultFlowController method to initialize default controller
func InitializeDefaultFlowController() Manager {
	logger.Debug("Default controller initialized")
	return new(DefaultFlowController)
}

// AddFlow method implemented for DefaultFlowController
func (fc *DefaultFlowController) AddFlow(flow *openolt.Flow) error {
	logger.WithFields(log.Fields{
		"flow_eth_type": flow.Classifier.EthType,
		"ovid":          flow.Classifier.OVid,
		"ivid":          flow.Classifier.IVid,
		"onu_id":        flow.OnuId,
		"flow_id":       flow.FlowId,
		"flow_type":     flow.FlowType,
	}).Debugf("AddFlow invoked for onu %d", flow.OnuId)
	return nil
}

// DeleteFlow implemented for DefaultFlowController
func (fc *DefaultFlowController) DeleteFlow(flow *openolt.Flow) error {
	logger.Debug("DeleteFlow invoked %v", flow)
	return nil
}

// DeleteAllFlows implemented for DefaultFlowController
func (fc *DefaultFlowController) DeleteAllFlows() error {
	logger.Debug("DeleteAllFlows invoked")
	return nil
}

// GetFlow implemented for DefaultFlowController
func (fc *DefaultFlowController) GetFlow(onuID uint32) ([]*openolt.Flow, error) {
	return nil, nil
}

// PortUp implemented for DefaultFlowController
func (fc *DefaultFlowController) PortUp(portID uint32) error {
	logger.Debug("PortUp invoked %d", portID)
	return nil
}

// PortDown implemented for DefaultFlowController
func (fc *DefaultFlowController) PortDown(portID uint32) error {
	logger.Debug("PortDown invoked %d", portID)
	return nil
}

// InitializePacketInStream implemented for DefaultFlowController
func (fc *DefaultFlowController) InitializePacketInStream(s openolt.Openolt_EnableIndicationServer) {
	logger.Debug("Initialize Openolt stream")
}

// PacketOut implemented for DefaultFlowController
func (fc *DefaultFlowController) PacketOut(pkt gopacket.Packet, intfType string, intfID uint32) error {
	logger.Debug("PacketOut invoked intfType: %s, intfID: %d", intfType, intfID)
	return nil
}
