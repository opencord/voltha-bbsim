# Copyright 2018-present Open Networking Foundation
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

BBSIM_DEPS  = $(wildcard ./*.go)
DOCKERTAG  ?= "latest"
REGISTRY ?= ""

.PHONY: dep test clean docker

prereq:
	go get -u google.golang.org/grpc
	go get -v github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway
	go get -v github.com/golang/protobuf/protoc-gen-go
	go get -v github.com/google/gopacket

bbsim: prereq protos/openolt.pb.go dep
	go build -i -v -o $@

dep: protos/openolt.pb.go
	go get -v -d ./...

protos/openolt.pb.go: openolt.proto
	@protoc -I . \
	-I${GOPATH}/src \
	-I${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
	--go_out=plugins=grpc:protos/ \
	$<

test:
	go test -v ./...
	go test -v ./... -cover

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	gometalinter --vendor --exclude ../../golang.org --skip protos --sort path --sort line ./...

clean:
	rm -f bbsim openolt/openolt.pb.go

docker:
	docker build -t ${REGISTRY}voltha/voltha-bbsim:${DOCKERTAG} .
