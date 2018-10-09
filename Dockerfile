# Copyright 2018 the original author or authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# bbsim dockerfile

ARG TAG=latest
ARG REGISTRY=
ARG REPOSITORY=

#builder parent
FROM ubuntu:16.04

MAINTAINER Voltha Community <info@opennetworking.org>

# Install required packages
RUN apt-get update && apt-get install -y wget git make libpcap-dev gcc unzip
ARG version="1.9.3."
RUN wget https://storage.googleapis.com/golang/go${version}linux-amd64.tar.gz -P /tmp \
    && tar -C /usr/local -xzf /tmp/go${version}linux-amd64.tar.gz \
    && rm /tmp/go${version}linux-amd64.tar.gz

# Set PATH
ENV GOPATH $HOME/go
ENV PATH /usr/local/go/bin:/go/bin:$PATH

# Copy source code
RUN mkdir -p $GOPATH/src/gerrit.opencord.org/voltha-bbsim
COPY . $GOPATH/src/gerrit.opencord.org/voltha-bbsim

# Install golang protobuf and pcap support
RUN wget https://github.com/google/protobuf/releases/download/v3.6.0/protoc-3.6.0-linux-x86_64.zip -P /tmp/ \
&& unzip /tmp/protoc-3.6.0-linux-x86_64.zip -d /tmp/ \
&& mv /tmp/bin/* /usr/local/bin/ \
&& mv /tmp/include/* /usr/local/include/ \
&& go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway \
&& go get -u github.com/golang/protobuf/protoc-gen-go \
&& go get -u github.com/google/gopacket/pcap \
&& go get -u golang.org/x/net/context \
&& go get -u google.golang.org/grpc

# ... Install utilities & config
RUN apt-get update && apt-get install -y wpasupplicant isc-dhcp-server
COPY ./config/wpa_supplicant.conf /etc/wpa_supplicant/
COPY ./config/isc-dhcp-server /etc/default/
COPY ./config/dhcpd.conf /etc/dhcp/
RUN mv /usr/sbin/dhcpd /usr/local/bin/ \
&& mv /sbin/dhclient /usr/local/bin/

WORKDIR $GOPATH/src/gerrit.opencord.org/voltha-bbsim
RUN make bbsim

