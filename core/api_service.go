/*
 * Copyright 2018-present Open Networking Foundation

 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at

 * http://www.apache.org/licenses/LICENSE-2.0

 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package core

import (
	"context"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	api "github.com/opencord/voltha-bbsim/api"
	"github.com/opencord/voltha-bbsim/common/logger"
	"github.com/opencord/voltha-bbsim/device"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Response Constants
const (
	RequestAccepted = "API request accepted"
	OLTNotEnabled   = "OLT not enabled"
	RequestFailed   = "API request failed"
	SoftReboot      = "soft-reboot"
	HardReboot      = "hard-reboot"
	DeviceTypeOlt   = "olt"
	DeviceTypeOnu   = "onu"
)

// OLTStatus method returns OLT status.
func (s *Server) OLTStatus(ctx context.Context, in *api.Empty) (*api.OLTStatusResponse, error) {
	logger.Trace("OLTStatus request received")
	oltInfo := &api.OLTStatusResponse{
		Olt: &api.OLTInfo{
			OltId:     int64(s.Olt.ID),
			OltSerial: s.Olt.SerialNumber,
			OltIp:     getOltIP().String(),
			OltState:  s.Olt.OperState,
			OltVendor: s.Olt.Manufacture,
		},
	}

	for _, nniPort := range s.Olt.NniIntfs {
		nniPortInfo, _ := s.fetchPortDetail(nniPort.IntfID, nniPort.Type)
		oltInfo.Ports = append(oltInfo.Ports, nniPortInfo)
	}
	for _, ponPort := range s.Olt.PonIntfs {
		ponPortInfo, _ := s.fetchPortDetail(ponPort.IntfID, ponPort.Type)
		oltInfo.Ports = append(oltInfo.Ports, ponPortInfo)
	}

	logger.Info("OLT Info: %v\n", oltInfo)
	return oltInfo, nil
}

// PortStatus method returns Port status.
func (s *Server) PortStatus(ctx context.Context, in *api.PortInfo) (*api.Ports, error) {
	logger.Trace("PortStatus() invoked")
	ports := &api.Ports{}
	portInfo, err := s.fetchPortDetail(in.PortId, in.PortType)

	if err != nil {
		return &api.Ports{}, status.Errorf(codes.InvalidArgument, err.Error())
	}

	ports.Ports = append(ports.Ports, portInfo)
	return ports, nil
}

// ONUStatus method returns ONU status.
func (s *Server) ONUStatus(ctx context.Context, in *api.ONURequest) (*api.ONUs, error) {
	logger.Trace("ONUStatus request received")
	if in.GetOnu() != nil {
		logger.Debug("Received single ONU: %+v, %d\n", in.GetOnu(), in.GetOnu().PonPortId)
		return s.handleONUStatusRequest(in.GetOnu())
	}

	logger.Debug("Received all ONUS status request")
	onuInfo := &api.ONUs{}
	for intfid := range s.Onumap {
		for _, onu := range s.Onumap[intfid] {
			if onu.InternalState != device.OnuFree {
				onuInfo.Onus = append(onuInfo.Onus, copyONUInfo(onu))
			}
		}
	}
	return onuInfo, nil
}

// ONUActivate method handles ONU activate requests from user.
func (s *Server) ONUActivate(ctx context.Context, in *api.ONURequest) (*api.BBSimResponse, error) {
	logger.Trace("ONUActivate request received")
	logger.Debug("Received values: %+v\n", in)

	var onuInfo []*api.ONUInfo
	// Activate single ONU
	if in.GetOnu() != nil {
		logger.Debug("Received single ONU: %+v\n", in.GetOnu())
		onuInfo = append(onuInfo, in.GetOnu())
	} else if len(in.GetOnusBatch().GetOnus()) != 0 { // Activate multiple ONUs
		logger.Debug("Received multiple ONUs")
		onuInfo = in.GetOnusBatch().GetOnus()
	} else {
		logger.Debug("Received empty request body")
		return &api.BBSimResponse{}, status.Errorf(codes.InvalidArgument, RequestFailed)
	}
	resp, err := s.handleONUActivate(onuInfo)
	return resp, err
}

// ONUDeactivate method handles ONU deactivation request.
func (s *Server) ONUDeactivate(ctx context.Context, in *api.ONURequest) (*api.BBSimResponse, error) {
	logger.Info("ONUDeactivate request received")

	// deactivate single ONU
	if in.GetOnu() != nil {
		logger.Debug("Received single ONU: %+v\n", in.GetOnu())
		err := s.handleONUDeactivate(in.GetOnu())
		if err != nil {
			return &api.BBSimResponse{}, status.Errorf(codes.Aborted, RequestFailed)
		}
	} else if len(in.GetOnusBatch().GetOnus()) != 0 { // bulk deactivate
		logger.Debug("Received multiple ONUs")
		for _, onuinfo := range in.GetOnusBatch().GetOnus() {
			logger.Debug("ONU values: %+v\n", onuinfo)
			err := s.handleONUDeactivate(onuinfo)
			if err != nil {
				return &api.BBSimResponse{}, status.Errorf(codes.Aborted, RequestFailed)
			}
		}
	} else {
		// Empty request body is passed, delete all ONUs from all PON ports
		for intfID := range s.Onumap {
			if err := s.DeactivateAllOnuByIntfID(intfID); err != nil {
				logger.Error("Failed in ONUDeactivate: %v", err)
				return &api.BBSimResponse{}, status.Errorf(codes.Aborted, RequestFailed)
			}
		}
	}

	return &api.BBSimResponse{StatusMsg: RequestAccepted}, nil

}

// GenerateONUAlarm RPC generates alarm for the onu
func (s *Server) GenerateONUAlarm(ctx context.Context, in *api.ONUAlarmRequest) (*api.BBSimResponse, error) {
	logger.Trace("GenerateONUAlarms() invoked")
	if in.OnuSerial == "" {
		return &api.BBSimResponse{}, status.Errorf(codes.InvalidArgument, "serial number can not be blank")
	}
	if len(in.OnuSerial) != SerialNumberLength {
		return &api.BBSimResponse{}, status.Errorf(codes.InvalidArgument, "invalid serial number given (length mismatch)")
	}
	if in.Status != "on" && in.Status != "off" {
		return &api.BBSimResponse{}, status.Errorf(codes.InvalidArgument, "invalid alarm status provided")
	}
	if s.alarmCh == nil {
		return &api.BBSimResponse{}, status.Errorf(codes.FailedPrecondition, "alarm-channel not created, can not send alarm")
	}
	resp, err := s.handleOnuAlarm(in)
	return resp, err
}

// GenerateOLTAlarm RPC generates alarm for the OLT
func (s *Server) GenerateOLTAlarm(ctx context.Context, in *api.OLTAlarmRequest) (*api.BBSimResponse, error) {
	logger.Trace("GenerateOLTAlarm() invoked")
	if in.Status != "on" && in.Status != "off" {
		return &api.BBSimResponse{}, status.Errorf(codes.InvalidArgument, "invalid alarm status provided")
	}
	if s.alarmCh == nil {
		return &api.BBSimResponse{}, status.Errorf(codes.FailedPrecondition, "alarm-channel not created, can not send alarm")
	}
	resp, err := s.handleOltAlarm(in)
	if err != nil {
		return resp, err
	}
	return &api.BBSimResponse{StatusMsg: RequestAccepted}, nil
}

// PerformDeviceAction rpc take the device request and performs OLT and ONU hard and soft reboot
func (s *Server) PerformDeviceAction(ctx context.Context, in *api.DeviceAction) (*api.BBSimResponse, error) {
	logger.Trace("PerformDeviceAction() invoked")
	if s.deviceActionCh == nil {
		return &api.BBSimResponse{}, status.Errorf(codes.FailedPrecondition, "device action channel not created, can not entertain request")
	}
	in, err := s.validateDeviceActionRequest(in)
	if err != nil {
		return &api.BBSimResponse{}, status.Errorf(codes.InvalidArgument, err.Error())
	}
	s.deviceActionCh <- in
	return &api.BBSimResponse{StatusMsg: RequestAccepted}, nil
}

// GetFlows returns all flows or flows for specified ONU
func (s *Server) GetFlows(ctx context.Context, in *api.ONUInfo) (*api.Flows, error) {
	logger.Info("GetFlow request received")
	flows := &api.Flows{}
	if in.OnuSerial == "" {
		for _, flow := range s.FlowMap {
			flowInfo := flow
			flows.Flows = append(flows.Flows, flowInfo)
		}
	} else {
		serialNumber, err := getOpenoltSerialNumber(in.OnuSerial)
		if err != nil {
			return flows, status.Errorf(codes.InvalidArgument, err.Error())
		}
		onu, found := s.getOnuFromSNmap(serialNumber)
		if !found {
			return flows, status.Errorf(codes.InvalidArgument, "ONU with serial number %s not activated yet", in.OnuSerial)
		}
		for _, flowKey := range onu.Flows {
			flow := s.FlowMap[flowKey]
			flowInfo := flow
			flows.Flows = append(flows.Flows, flowInfo)
		}
	}
	return flows, nil
}

// NewMgmtAPIServer method starts BBSim gRPC server.
func NewMgmtAPIServer(addrport string) (l net.Listener, g *grpc.Server, e error) {
	logger.Info("BBSim gRPC server listening %s ...", addrport)
	g = grpc.NewServer()
	l, e = net.Listen("tcp", addrport)
	return
}

// StartRestGatewayService method starts REST server for BBSim.
func StartRestGatewayService(grpcAddress string, hostandport string) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	// Register REST endpoints
	err := api.RegisterBBSimServiceHandlerFromEndpoint(ctx, mux, grpcAddress, opts)
	if err != nil {
		logger.Error("%v", err)
		return
	}

	logger.Info("BBSim REST server listening %s ...", hostandport)
	err = http.ListenAndServe(hostandport, mux)
	if err != nil {
		logger.Error("%v", err)
		return
	}
	return
}
