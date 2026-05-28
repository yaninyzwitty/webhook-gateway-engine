package server

import (
	"fmt"
	"log/slog"
	"net"

	deliveryv1 "github.com/yaninyzwitty/webhook-gateway-service/gen/delivery/v1"
	endpointv1 "github.com/yaninyzwitty/webhook-gateway-service/gen/endpoint/v1"
	eventv1 "github.com/yaninyzwitty/webhook-gateway-service/gen/event/v1"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/service"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/store"
	"google.golang.org/grpc"
)

type Server struct {
	grpc *grpc.Server
	port int
}

func New(st *store.Store, port int) *Server {
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(loggingInterceptor, recoveryInterceptor))

	eventServiceHandler := service.NewEventService(st)
	endpointServiceHandler := service.NewEndpointService(st)
	deliveryServiceHandler := service.NewDeliveryService(st)

	// register services
	eventv1.RegisterEventServiceServer(grpcServer, eventServiceHandler)
	endpointv1.RegisterEndpointServiceServer(grpcServer, endpointServiceHandler)
	deliveryv1.RegisterDeliveryServiceServer(grpcServer, deliveryServiceHandler)
	return &Server{
		grpc: grpcServer,
		port: port,
	}
}

func (s *Server) Run() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	slog.Info("starting gRPC server", "port", s.port)

	return s.grpc.Serve(lis)
}

func (s *Server) GracefulStop() {
	s.grpc.GracefulStop()
	slog.Info("gRPC server stopped")
}

func (s *Server) Stop() {
	s.grpc.Stop()
	slog.Info("gRPC server force stopped")
}
