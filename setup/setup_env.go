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

package setup

import (
	"os/exec"
	"time"
	"log"
	"gerrit.opencord.org/voltha-bbsim/common"
)

const (
	UNI_VETH_UP_PFX = "sim_uu"
	UNI_VETH_DW_PFX = "sim_ud"
	NNI_VETH_UP_PFX = "sim_nu"
	NNI_VETH_DW_PFX = "sim_nd"
)

func ActivateWPASups(vethnames []string, delay int) error {
	for _, vethname := range vethnames {
		time.Sleep(time.Duration(delay) * time.Second)
		logger.Debug("ActivateWPASups for interface %v\n", vethname)
		if err := activateWPASupplicant(vethname); err != nil {
			return err
		}
	}
	return nil
}

func ActivateDHCPClients(vethnames []string, delay int) error {
	for _, vethname := range vethnames {
		time.Sleep(time.Duration(delay) * time.Second)
		logger.Debug("activateDHCPClient for interface %v\n", vethname)
		if err := activateDHCPClient(vethname); err != nil {
			return err
		}
	}
	return nil
}

func KillProcess(name string) error {
	err := exec.Command("pkill", name).Run()
	if err != nil {
		logger.Error("Fail to pkill %s: %v\n", name, err)
		return err
	}
	logger.Info("Successfully killed %s\n", name)
	return nil
}

func TearVethDown(veths []string) {
	for _, veth := range veths {
		RemoveVeth(veth)
	}
}

func CreateVethPairs(name1 string, name2 string) (err error) {
	err = exec.Command("ip", "link", "add", name1, "type", "veth", "peer", "name", name2).Run()
	if err != nil {
		logger.Error("Fail to createVeth() for %s and %s veth creation error: %s\n", name1, name2, err.Error())
		return
	}
	logger.Info("%s & %s was created.", name1, name2)
	err = exec.Command("ip", "link", "set", name1, "up").Run()
	if err != nil {
		logger.Error("Fail to createVeth() veth1 up", err)
		return
	}
	err = exec.Command("ip", "link", "set", name2, "up").Run()
	if err != nil {
		logger.Error("Fail to createVeth() veth2 up", err)
		return
	}
	logger.Info("%s & %s was up.", name1, name2)
	return
}

func RemoveVeth(name string) {
	err := exec.Command("ip", "link", "del", name).Run()
	if err != nil {
		logger.Error("Fail to removeVeth()", err)
	}
	logger.Info("%s was removed.", name)
}

func RemoveVeths(names []string) {
	logger.Info("RemoveVeths() :%s\n", names)
	for _, name := range names {
		RemoveVeth(name)
	}
}

func activateWPASupplicant(vethname string) (err error) {
	cmd := "/sbin/wpa_supplicant"
	conf := "/etc/wpa_supplicant/wpa_supplicant.conf"
	err = exec.Command(cmd, "-D", "wired", "-i", vethname, "-c", conf).Start()
	if err != nil {
		logger.Error("Fail to activateWPASupplicant() for :%s %v\n", vethname, err)
		return
	}
	logger.Info("activateWPASupplicant() for :%s\n", vethname)
	return
}

func activateDHCPClient(vethname string) (err error) {
	logger.Debug("activateDHCPClient() start for: %s\n", vethname)
	cmd := exec.Command("/usr/local/bin/dhclient", vethname)
	// if err := cmd.Run(); err != nil {
	if err := cmd.Start(); err != nil {
		logger.Error("Fail to activateDHCPClient() for: %s", vethname)
		log.Panic(err)
	}

	// if the dhclient commands return we got an IP,
	// so kill it
	// if err := KillProcess("/usr/local/bin/dhclient"); err != nil {
	// 	log.Printf("[ERROR] Failed to kill activateDHCPClient() for: %s", vethname, err)
	// }

	logger.Debug("activateDHCPClient() done for: %s\n", vethname)
	return
}

func ActivateDHCPServer(veth string, serverip string) error {
	err := exec.Command("ip", "addr", "add", serverip, "dev", veth).Run()
	if err != nil {
		logger.Error("Fail to add ip to %s address: %s\n", veth, err)
		return err
	}
	err = exec.Command("ip", "link", "set", veth, "up").Run()
	if err != nil {
		logger.Error("Fail to set %s up: %s\n", veth, err)
		return err
	}
	cmd := "/usr/local/bin/dhcpd"
	conf := "/etc/dhcp/dhcpd.conf"
	err = exec.Command(cmd, "-cf", conf, veth).Run()
	if err != nil {
		logger.Error("Fail to activateDHCP Server (): %s\n", err)
		return err
	}
	logger.Info("DHCP Server is successfully activated !\n")
	return err
}
