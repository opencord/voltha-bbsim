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

VERSION                  ?= $(shell cat ./VERSION)
BBSIM_DEPS                = $(wildcard ./*.go)

## Docker related
DOCKER_REGISTRY          ?= ""
DOCKER_REPOSITORY        ?= voltha/
DOCKER_BUILD_ARGS        ?=
DOCKER_TAG               ?= ${VERSION}
BBSIM_IMAGENAME          := ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}voltha-bbsim:${DOCKER_TAG}

## Docker labels. Only set ref and commit date if committed
DOCKER_LABEL_VCS_URL     ?= $(shell git remote get-url $(shell git remote))
DOCKER_LABEL_VCS_REF     ?= $(shell git diff-index --quiet HEAD -- && git rev-parse HEAD || echo "unknown")
DOCKER_LABEL_COMMIT_DATE ?= $(shell git diff-index --quiet HEAD -- && git show -s --format=%cd --date=iso-strict HEAD || echo "unknown" )
DOCKER_LABEL_BUILD_DATE  ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

bbsim: dep bbsimapi
	GO111MODULE=on go build -i -v -o $@

dep:
	GO111MODULE=off go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway
	GO111MODULE=off go get -v github.com/golang/protobuf/protoc-gen-go
	GO111MODULE=off go get -v github.com/google/gopacket
	GO111MODULE=on go mod download all

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

test: dep bbsimapi
	GO111MODULE=on go test -v ./...
	GO111MODULE=on go test -v ./... -cover

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
	docker build ${DOCKER_BUILD_ARGS} -t ${BBSIM_IMAGENAME} .

docker-save:
	docker save ${BBSIM_IMAGENAME} -o voltha-bbsim_${DOCKER_TAG}.tgz

docker-push:
	docker push ${BBSIM_IMAGENAME}
