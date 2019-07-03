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
VERSION     ?= $(shell cat ./VERSION)
DOCKER_TAG  ?= ${VERSION}

## Docker related
DOCKER_BUILD_ARGS        ?=
DOCKER_TAG               ?= ${VERSION}

## Docker labels. Only set ref and commit date if committed
DOCKER_LABEL_VCS_URL     ?= $(shell git remote get-url $(shell git remote))
DOCKER_LABEL_VCS_REF     ?= $(shell git diff-index --quiet HEAD -- && git rev-parse HEAD || echo "unknown")
DOCKER_LABEL_COMMIT_DATE ?= $(shell git diff-index --quiet HEAD -- && git show -s --format=%cd --date=iso-strict HEAD || echo "unknown" )
DOCKER_LABEL_BUILD_DATE  ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

.PHONY: dep test clean docker

prereq:
	go get -v google.golang.org/grpc
	go get -v github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway
	go get -v github.com/golang/protobuf/protoc-gen-go
	go get -v github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger
	go get -v github.com/google/gopacket
	go get -u -v github.com/opencord/omci-sim

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
	api/bbsim.proto

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

docker-build:
	docker build -t voltha/voltha-bbsim:${DOCKER_TAG} .
	docker save voltha/voltha-bbsim:${DOCKER_TAG} -o voltha-bbsim_${DOCKER_TAG}.tgz

docker-push:
	docker push voltha/voltha-bbsim:${DOCKER_TAG}