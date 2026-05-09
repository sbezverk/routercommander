#!/bin/bash

# REMOTE_LOOPBACK is loopback of the remote DRC (showld be specified in a form of A.A.A.A/32)
# REMOTE_DESTINATION is destination ip for failed mesh ping session (showld be specified in a form of B.B.B.B/N, where N remote prefix length3
# VRF_NAME   is vrf for the impacted flow
# INGRESS_LC is a line card hosting the source of mesh ping traffic
# EGRESS_LC is a line card facing upstream ORC routers
# TIME_STAMP is a time stamp when the incident occurred, with 1 hour precision, example: Jul 10 0[8-9]

REMOTE_LOOPBACK="1.1.1.1/32"
LOCAL_LOOPBACK="2.2.2.2/32"
REMOTE_DESTINATION="10.101.3.1/30"
VRF_NAME="GI"
INGRESS_LC="0/0/CPU0"
EGRESS_LC="0/0/CPU0"
TIME_STAMP="Jul\( \)+14\( \)+\(13\|14\):"

cp ./generic_0.4.0.yaml ./generic_0.4.0_populated.yaml

if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        # Linux
sed -i  "s|{{.REMOTE_LOOPBACK}}|${REMOTE_LOOPBACK}|g" ./generic_0.4.0_populated.yaml
sed -i  "s|{{.LOCAL_LOOPBACK}}|${LOCAL_LOOPBACK}|g" ./generic_0.4.0_populated.yaml
sed -i  "s|{{.REMOTE_DESTINATION}}|${REMOTE_DESTINATION}|g" ./generic_0.4.0_populated.yaml
sed -i  "s|{{.VRF_NAME}}|${VRF_NAME}|g" ./generic_0.4.0_populated.yaml
sed -i  "s|{{.INGRESS_LC}}|${INGRESS_LC}|g" ./generic_0.4.0_populated.yaml
sed -i  "s|{{.EGRESS_LC}}|${EGRESS_LC}|g" ./generic_0.4.0_populated.yaml
sed -i  "s|{{.TIME_STAMP}}|${TIME_STAMP}|g" ./generic_0.4.0_populated.yaml

elif [[ "$OSTYPE" == "darwin"* ]]; then
        # Mac OSX
sed -i '' "s|{{.REMOTE_LOOPBACK}}|${REMOTE_LOOPBACK}|g" ./generic_0.4.0_populated.yaml
sed -i '' "s|{{.REMOTE_DESTINATION}}|${REMOTE_DESTINATION}|g" ./generic_0.4.0_populated.yaml
sed -i '' "s|{{.VRF_NAME}}|${VRF_NAME}|g" ./generic_0.4.0_populated.yaml
sed -i '' "s|{{.INGRESS_LC}}|${INGRESS_LC}|g" ./generic_0.4.0_populated.yaml
sed -i '' "s|{{.EGRESS_LC}}|${EGRESS_LC}|g" ./generic_0.4.0_populated.yaml
sed -i '' "s|{{.TIME_STAMP}}|${TIME_STAMP}|g" ./generic_0.4.0_populated.yaml

# elif [[ "$OSTYPE" == "cygwin" ]]; then
        # POSIX compatibility layer and Linux environment emulation for Windows
# elif [[ "$OSTYPE" == "msys" ]]; then
        # Lightweight shell and GNU utilities compiled for Windows (part of MinGW)
# elif [[ "$OSTYPE" == "win32" ]]; then
        # I'm not sure this can happen.
# elif [[ "$OSTYPE" == "freebsd"* ]]; then
        # ...
# else
        # Unknown.
fi 
