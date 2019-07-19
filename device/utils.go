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
	"github.com/opencord/voltha-bbsim/common/logger"
	log "github.com/sirupsen/logrus"
	"strconv"
)

// OnuToSn returns serial number in string format for given ONU
func OnuToSn(onu *Onu) string {
	return string(onu.SerialNumber.VendorId) + ConvB2S(onu.SerialNumber.VendorSpecific)
}

// LoggerWithOnu method logs ONU fields
func LoggerWithOnu(onu *Onu) *log.Entry {

	if onu == nil {
		logger.Warn("utils.LoggerWithOnu has been called without Onu")
		return logger.GetLogger()
	}

	return logger.GetLogger().WithFields(log.Fields{
		"serial_number": OnuToSn(onu),
		"interfaceID":   onu.IntfID,
		"onuID":         onu.OnuID,
		"oltID":         onu.OltID,
	})
}

// ConvB2S converts byte array to string
func ConvB2S(b []byte) string {
	s := ""
	for _, i := range b {
		s = s + strconv.FormatInt(int64(i/16), 16) + strconv.FormatInt(int64(i%16), 16)
	}
	return s
}
