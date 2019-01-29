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

package main

import (
	"log"

	"gerrit.opencord.org/voltha-bbsim/common/logger"
	"gerrit.opencord.org/voltha-bbsim/core"
)

func printBanner() {
	log.Println("     ________    _______   ________                 ")
	log.Println("    / ____   | / ____   | / ______/  __            ")
	log.Println("   / /____/ / / /____/ / / /_____   /_/            ")
	log.Println("  / _____  | / _____  | /______  | __  __________ ")
	log.Println(" / /____/ / / /____/ / _______/ / / / / __  __  / ")
	log.Println("/________/ /________/ /________/ /_/ /_/ /_/ /_/  ")
}

func main() {

	// CLI Shows up
	printBanner()
	opt := core.GetOptions()
	logger.Setup(opt.KafkaBroker, opt.Debuglvl)

	mediator := core.NewMediator(opt)

	mediator.Start()
}
