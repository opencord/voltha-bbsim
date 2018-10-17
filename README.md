# 1. Overview

The BBSim (Broadband Simulator) is for emulating the control message response sent from OLT and ONUs which are connected to VOLTHA Adapter.
It is implemetend as a software process which runs outside VOLTHA, and acts as if it was a OLT connected to multiple ONUs.
This enables the scalability test of VOLTHA without actual hardware OLT / ONUs.
The difference from the existing PONsim is to focus on emulating control messages, not data-path traffic which PONsim targets.

The BBSim container contains wpa_supplicants and dhcp clients for supporting AAA client (wpa_supplicant) emulation & DHCP client emulation.

```
==============
VOLTHA
BBSim Adapter
==============
 |
 |  gRPC connections
 |
==============
BBSim containers
(Each BBSim container corresponds to a single olt)
==============
```

# 2. Build, Run BBSim and VOLTHA-CLI commands
```
# Build and Run Docker container
git clone https://github.com/opencord/voltha-bbsim
cd voltha-bbsim
make docker
docker run -it --rm --privileged=true --expose=50060 --network=compose_default voltha/voltha-bbsim /go/src/gerrit.opencord.org/voltha-bbsim/bbsim -n 16

# After this, execute the following commands from VOLTHA-CLI
(voltha) health
{
    "state": "HEALTHY"
}
(voltha) preprovision_olt -t bbsimolt -H <BBSim Docker container IP>
success (device id = <deviceid>)
(voltha) enable
enabling <deviceid>
waiting for device to be enabled...
success (device id = <deviceid>)
(voltha) devices
## You can see the list of devices (OLT/ONUs) ##
```

# 3. How to use BBSim

```
# Note: 2018/10/8 The current version only supports AAA emulations.
Usage of ./bbsim:
  -H string
    	IP address:port (default ":50060")
  -a int
    	Wait time (sec) for activation WPA supplicants (default 30)
  -d int
    	Wait time (sec) for activation DHCP clients (default 10)
  -i int
    	Number of PON-IF ports (default 1)
  -id int
    	OLT-ID (default: 0)
  -m string
    	Emulation mode (default, aaa, both (aaa & dhcp)) (default "default")
  -n int
    	Number of ONUs per PON-IF port (default 1)
  -s string
    	DHCP Server IP Address (default "182.21.0.1")
```
