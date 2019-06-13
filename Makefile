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
	go get -v google.golang.org/grpc
	go get -v github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway
	go get -v github.com/golang/protobuf/protoc-gen-go
	go get -v github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger
	go get -v github.com/google/gopacket
	go get -v github.com/opencord/omci-sim

bbsim: prereq protos/openolt.pb.go bbsimapi dep
	go build -i -v -o $@

dep: protos/openolt.pb.go bbsimapi
	go get -v -d ./...

protos/openolt.pb.go: openolt.proto
	@protoc -I . \
	-I${GOPATH}/src \
	-I${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
	--go_out=plugins=grpc:protos/ \
	$<

bbsimapi: api/bbsim.proto
	@protoc -I ./api \
	-I${GOPATH}/src \
	-I${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
	-I${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway \
	--go_out=plugins=grpc:api/ \
	--grpc-gateway_out=logtostderr=true,allow_delete_body=true:api/ \
	bbsim.proto

swagger:						 ## Generate swagger documentation for BBsim API
	@protoc -I ./api \
	-I${GOPATH}/src \
	-I${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
	-I${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway \
	--swagger_out=logtostderr=true,allow_delete_body=true:api/swagger/ \
	bbsim.proto

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
	@rm -vf bbsim \
			protos/openolt.pb.go \
			api/bbsim.pb.go \
	        api/bbsim.pb.gw.go \
	        api/swagger/*.json

docker:
	docker build -t ${REGISTRY}voltha/voltha-bbsim:${DOCKERTAG} .
	docker save voltha/voltha-bbsim:${DOCKERTAG} -o voltha-bbsim_${DOCKERTAG}.tgz
