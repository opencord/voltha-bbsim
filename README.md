# 1. Overview & Features

The BBSim (Broadband Simulator) is for emulating the control message response (e.g. OLTInd, DHCP, EAPOL, OpenOMCI messages etc..) sent from OLT and ONUs which are connected to VOLTHA Adapter (OpenOLT Adapter).
It is implemetend as a software process which runs outside VOLTHA, and acts as if it was a OLT connected to multiple ONUs.
This enables the scalability test of VOLTHA / ONOS without actual hardware OLT / ONUs.
You can use BBSim container for a scalability test for VOLTHA, DHCP L2 relay (https://github.com/opencord/dhcpl2relay) and AAA (AAA (ONOS 1.10)#ActivateAAAapp) applications on ONOS.
The difference from the existing PONsim is to focus on emulating control messages, not data-path traffic which PONsim targets.

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
      Wait time (sec) for activation WPA supplicants (default 10)
  -d string
      Debug Level(TRACE DEBUG INFO WARN ERROR) (default "DEBUG")
  -dw int
      Wait time (sec) for activation DHCP clients (default 20)
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
      Interval each ONU Discovery Indication (ms) (default 1000)
```
