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

type OmciMsg struct {
	IntfId uint32
	OnuId  uint32
	Pkt    []byte
}

func OmciRun(omciOut chan OmciMsg, omciIn chan OmciMsg) {
	for {
		var resp OmciMsg

		msg := <-omciOut

		resp.Pkt = []byte{
			0x00, 0x01, 0x2f, 0x0a, 0x00, 0x02, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x28, 0x1f,
			0x75, 0x69, 0xaa}

		resp.IntfId = msg.IntfId
		resp.OnuId = msg.OnuId
		omciIn <- resp
	}
}
