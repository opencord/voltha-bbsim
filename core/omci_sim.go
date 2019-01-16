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
	"gerrit.opencord.org/voltha-bbsim/common/logger"
)

func OmciSim(intfId uint32, onuId uint32, request []byte) ([]byte, error) {
	var resp []byte

	transactionId, deviceId, msgType, class, instance, content, err := ParsePkt(request)
	if err != nil {
		return resp, &OmciError{"Cannot parse omci msg"}
	}

	logger.Debug("OmciRun - transactionId: %d msgType: %d, ME Class: %d, ME Instance: %d",
		transactionId, msgType, class, instance)

	key := OnuKey{intfId, onuId}
	if _, ok := OnuOmciStateMap[key]; !ok {
		OnuOmciStateMap[key] = NewOnuOmciState()
	}

	if _, ok := Handlers[msgType]; !ok {
		logger.Warn("Ignore omci msg (msgType %d not handled)", msgType)
		return resp, &OmciError{"Unimplemented omci msg"}
	}

	resp, err = Handlers[msgType](class, content, key)
	if err != nil {
		return resp, err
	}
	resp[0] = byte(transactionId >> 8)
	resp[1] = byte(transactionId & 0xFF)
	resp[2] = 0x2<<4 | byte(msgType)
	resp[3] = deviceId

	return resp, nil
}
