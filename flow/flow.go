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
	"gerrit.opencord.org/voltha-bbsim/common/logger"
	openolt "gerrit.opencord.org/voltha-bbsim/protos"
)

var flowManager FlowManager

// FlowManager interface for common methods of controller
type FlowManager interface {
	AddFlow(flow *openolt.Flow) error
	DeleteFlow(flow *openolt.Flow) error
	PortUp(portID uint32) error
	PortDown(portID uint32) error
	GetFlow(onuID uint32) ([]*openolt.Flow, error)
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

// AddFlow abstracts actual implementation of flow addition
func AddFlow(flow *openolt.Flow) error {
	return flowManager.AddFlow(flow)
}

// DeleteFlow abstracts actual implementation of flow deletion
func DeleteFlow(flow *openolt.Flow) error {
	return flowManager.DeleteFlow(flow)
}

// PortUp abstracts actual implementation of port up
func PortUp(portID uint32) error {
	return flowManager.PortUp(portID)
}

// PortDown abstracts actual implementation of port down
func PortDown(portID uint32) error {
	return flowManager.PortDown(portID)
}

// InitializeDefaultFlowController method to initialize default controller
func InitializeDefaultFlowController() FlowManager {
	logger.Debug("Default controller initialized")
	return new(DefaultFlowController)
}

// AddFlow method implemented for DefaultFlowController
func (fc *DefaultFlowController) AddFlow(flow *openolt.Flow) error {
	logger.Debug("AddFlow invoked %v", flow)
	return nil
}

// DeleteFlow implemented for DefaultFlowController
func (fc *DefaultFlowController) DeleteFlow(flow *openolt.Flow) error {
	logger.Debug("DeleteFlow invoked %v", flow)
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
