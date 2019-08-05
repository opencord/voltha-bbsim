# 1. Overview & Features

The BBSim (Broadband Simulator) is a software simulator for emulating the control message response (e.g. OLTInd, DHCP, EAPOL, OpenOMCI messages etc..) sent from OLT and ONUs to VOLTHA/ONOS via OpenOLT Adapter.
This simulator is implemetend inside a Docker container, and acts as if it was a OLT device connected to multiple ONUs.
You can use this BBSim container for a scalability test for VOLTHA, DHCP L2 relay (https://github.com/opencord/dhcpl2relay) and AAA (https://github.com/opencord/aaa) applications on ONOS.
The difference from PONsim is that BBSim focuses on emulating control messages, not data-path traffic which PONsim targets.

```
                 +--------------------------------------------------+
                 |                      VOLTHA Core                 |
                 +--------------------------------------------------+
                 +--------------------------------------------------+
                 |                     OpenOLT Adapter              |
                 +------------------------^-------------------------+
                 +------------------------|-------------------------+
                 |Container---------------|-----------------------+ |
                 |        |BBSim          |                       | |
                 |        |  +------------v---------------------+ | |
                 |        |  |          gRPC Server             | | |
                 |        |  +------------^---------------------+ | |
                 |+------+|  +------------|---------------------+ | |
                 ||      ||  |CoreServer  |                     | | |
                 ||DHCP  ||  | +----------v-------------------+ | | |
                 ||Server <--->|         MainPktLoop          | | | |
                 ||      ||  | +---^--------^-------------^---+ | | |
                 |+------+|  +-----|--------|-------------|-----+ | |
                 |        |  +-----v---++---v-----++------v-----+ | |
                 |        |  |OMCI     ||EAPOL    || DHCP       | | |
                 |        |  |Responder||Responder|| Responder  | | |
                 |        |  +---------++---------++------------+ | |
                 |        +---------------------------------------+ |
                 +--------------------------------------------------+

```

# 2. Build, Run BBSim and VOLTHA-CLI commands
※ You need to configure SADIS before you start to run BBSim, if you want to test AAA/DHCP (Refer to section 3).
```
# Build and Run Docker container
git clone https://github.com/opencord/voltha-bbsim
cd voltha-bbsim
make docker
docker run -it --rm --privileged=true --expose=50060 --network=compose_default voltha/voltha-bbsim ./bbsim -n 16

# After this, execute the following commands from VOLTHA-CLI
(voltha) health
{
    "state": "HEALTHY"
}
(voltha) preprovision_olt -t openolt -H <BBSim Docker container IP>
success (device id = <deviceid>)
(voltha) enable
enabling <deviceid>
waiting for device to be enabled...
success (device id = <deviceid>)
(voltha) devices
## You can see the list of devices (OLT/ONUs) ##
```

# 3. Configuration of ONOS App side (VOLTHA 1.7, ONOS1.13.9.rc4)
You need to configure a few parameters for AAA/DHCPL2Relay app before running BBSim.
You only need to set 1. device id assigned to BBSim (i.e. of:*********), 2. c/sTag and Tech Profile with BBSim ONU instance device id, in .netconf file.
(Please refer to https://wiki.onosproject.org/display/ONOS/NETCONF about netconf support in ONOS)
```
    "org.opencord.dhcpl2relay" : {
           "dhcpl2relay" : {
               "dhcpServerConnectPoints" : [ "<device id assigned to BBSim>/65536" ],
               "useOltUplinkForServerPktInOut" : true
           }
    },

    ....
    
    "org.opencord.sadis" : {
        "sadis" : {
              "entries":[
              {
                "id" : "BBSIMOLT000",
                "hardwareIdentifier" : "de:ad:be:ef:ba:11",
                "uplinkPort" : 65536
              },
        //BBSim generates both device id and c/stag assigned to each ONU while incrementing them by 1 from BBSM00000100, 900.
        //The following lines are an example when we use for configuring 2 ONUs.
       {"id" : "BBSM00000100", "cTag" : 900, "sTag" : 900, "nasPortId" : "BBSM00000100", "technologyProfileId" : 64,  "upstreamBandwidthProfile":"High-Speed-Internet", "downstreamBandwidthProfile":"User1-Specific"},
       {"id" : "BBSM00000101", "cTag" : 901, "sTag" : 901, "nasPortId" : "BBSM00000101", "technologyProfileId" : 64,  "upstreamBandwidthProfile":"High-Speed-Internet", "downstreamBandwidthProfile":"User1-Specific"},

      ......
```


# 4. CLI options
```
Usage of ./bbsim:
  -H string
      IP address:port (default ":50060")
  -aw int
      Wait time (sec) for activation WPA supplicants after EAPOL flow entry installed (default 2)
  -d string
      Debug Level(TRACE DEBUG INFO WARN ERROR) (default "DEBUG")
  -dw int
      Wait time (sec) for activation DHCP clients after DHCP flow entry installed (default 2)
  -i int
      Number of PON-IF ports (default 1)
  -id int
      OLT-ID
  -k string
      Kafka broker
  -m string
      Emulation mode (default, aaa, both (aaa & dhcp)) (default "default")
  -n int
      Number of ONUs per PON-IF port (default 1)
  -s string
      DHCP Server IP Address (default "182.21.0.128")
  -v int
      Interval each Discovery Indication, in the form of unit+suffix, such as '10ms', '1s' or '1m' (default 0)
  -ia bool
      Interactive activation of ONUs: if true ONUs must be activated explicitly using the management API (no ONUs are activated at startup) (default false)
  -grpc int
      Management API gRPC port (default 50061)
  -rest int
      Management API rest port (default 50062)
```
