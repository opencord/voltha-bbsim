This directory contains protobuf files for the BBSim control API.

# Examples

```
# start BBSim 

    ./bbsim -i 8 -n 16 -ia false

# get OLT status

    curl -X GET http://127.0.0.1:50062/v1/olt | jq

# get status of the OLT's PON ports

    curl -X GET http://127.0.0.1:50062/v1/olt/ports/pon | jq

# get status of all active ONUs

    curl -X GET http://127.0.0.1:50062/v1/olt/onus | jq

# get the status of the ONU with ONU-ID 2 (on PON port 1)

    curl -X GET "http://127.0.0.1:50062/v1/olt/onus?onu_id=2&pon_port_id=1" | jq

# activate single ONU with a given serial number:

    curl -X POST http://127.0.0.1:50062/v1/olt/ports/1/onus/BBSM00000201 | jq

# get the status of an ONU using its serial number:

    curl -X GET http://127.0.0.1:50062/v1/olt/onus/BBSM00000201 | jq

# deactivate ONU using its serial number:

    curl -X DELETE  http://127.0.0.1:50062/v1/olt/onus/BBSM00000201 | jq

# or 

    curl -X DELETE  http://127.0.0.1:50062/v1/olt/onus?onu_serial=BBSM00000201 | jq
```


# Activate multiple ONUs
```
cat <<EOF > onus_request.json
{
  "onus": [
    {
      "pon_port_id": 1,
      "onu_serial": "BBSIMONU0001"
    },
    {
      "pon_port_id": 2,
      "onu_serial": "BBSIMONU0002"
    }
  ]
}
EOF


    curl -H  "Content-Type: application/json" -X POST "http://127.0.0.1:50062/v1/olt/onus" -d @onus_request.json


```
