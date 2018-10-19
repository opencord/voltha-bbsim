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
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"gerrit.opencord.org/voltha-bbsim/core"
	"gerrit.opencord.org/voltha-bbsim/protos"
)

func printBanner() {
	log.Println("     ________    _______   ________                 ")
	log.Println("    / ____   | / ____   | / ______/  __            ")
	log.Println("   / /____/ / / /____/ / / /_____   /_/            ")
	log.Println("  / _____  | / _____  | /______  | __  __________ ")
	log.Println(" / /____/ / / /____/ / _______/ / / / / __  __  / ")
	log.Println("/________/ /________/ /________/ /_/ /_/ /_/ /_/  ")
}

func getOptions() (uint32, string, uint32, uint32, uint32, int, int, string, int, core.Mode) {
	addressport := flag.String("H", ":50060", "IP address:port")
	oltid := flag.Int("id", 0, "OLT-ID")
	nintfs := flag.Int("i", 1, "Number of PON-IF ports")
	nonus := flag.Int("n", 1, "Number of ONUs per PON-IF port")
	modeopt := flag.String("m", "default", "Emulation mode (default, aaa, both (aaa & dhcp))")
	aaawait := flag.Int("a", 30, "Wait time (sec) for activation WPA supplicants")
	dhcpwait := flag.Int("d", 10, "Wait time (sec) for activation DHCP clients")
	dhcpservip := flag.String("s", "182.21.0.1", "DHCP Server IP Address")
	delay := flag.Int("delay", 1, "Delay between ONU events")
	mode := core.DEFAULT
	flag.Parse()
	if *modeopt == "aaa" {
		mode = core.AAA
	} else if *modeopt == "both" {
		mode = core.BOTH
	}
	address := strings.Split(*addressport, ":")[0]
	tmp, _ := strconv.Atoi(strings.Split(*addressport, ":")[1])
	port := uint32(tmp)
	return uint32(*oltid), address, port, uint32(*nintfs), uint32(*nonus), *aaawait, *dhcpwait, *dhcpservip, *delay, mode
}

func main() {
	// CLI Shows up
	printBanner()
	oltid, ip, port, npon, nonus, aaawait, dhcpwait, dhcpservip, delay, mode := getOptions()
	log.Printf("ip:%s, baseport:%d, npon:%d, nonus:%d, mode:%d\n", ip, port, npon, nonus, mode)

	// Set up gRPC Server
	var wg sync.WaitGroup

	addressport := ip + ":" + strconv.Itoa(int(port))
	endchan := make(chan int, 1)
	listener, gserver, err := core.CreateGrpcServer(oltid, npon, nonus, addressport)
	server := core.Create(oltid, npon, nonus, aaawait, dhcpwait, dhcpservip, delay, gserver, mode, endchan)
	if err != nil {
		log.Println(err)
	}
	openolt.RegisterOpenoltServer(gserver, server)

	wg.Add(1)
	go func() {
		defer wg.Done()
		gserver.Serve(listener)
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			fmt.Println("SIGINT", sig)
			close(c)
			close(server.Endchan)
			gserver.Stop()
		}
	}()
	wg.Wait()
	time.Sleep(5 * time.Second)
	log.Println("Reach to the end line")
}
