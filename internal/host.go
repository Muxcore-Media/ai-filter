package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Muxcore-Media/core/pkg/contracts"
	meshv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/mesh/v1"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"google.golang.org/grpc"
)

type settingsMesh interface {
	Settings() []contracts.SettingDef
	UpdateSetting(key, value string) error
	handleMesh(ctx context.Context, method string, payload []byte) ([]byte, error)
}

func startHTTPGRPC(
	ctx context.Context, id string, grpcAddr, httpAddr *string, lis *net.Listener,
	grpcSrv **grpc.Server, httpSrv **http.Server, h settingsMesh, routes func(*http.ServeMux),
) error {
	var lc net.ListenConfig
	gLis, err := lc.Listen(ctx, "tcp", *grpcAddr)
	if err != nil {
		return err
	}
	*lis = gLis
	*grpcAddr = gLis.Addr().String()
	*grpcSrv = grpc.NewServer()
	meshv1.RegisterModuleMeshServer(*grpcSrv, &genericMesh{settings: modulesdk.SettingsHandlerFromProvider(h), h: h})
	go func() {
		slog.Info("gRPC listening", "id", id, "addr", *grpcAddr)
		if e := (*grpcSrv).Serve(gLis); e != nil {
			slog.Error("gRPC", "error", e)
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	routes(mux)
	hLis, err := lc.Listen(ctx, "tcp", *httpAddr)
	if err != nil {
		return err
	}
	*httpAddr = hLis.Addr().String()
	*httpSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if e := (*httpSrv).Serve(hLis); e != nil && e != http.ErrServerClosed {
			slog.Error("HTTP", "error", e)
		}
	}()
	return nil
}

type genericMesh struct {
	meshv1.UnimplementedModuleMeshServer
	settings modulesdk.SettingsHandler
	h        settingsMesh
}

func (s *genericMesh) Call(ctx context.Context, req *meshv1.CallRequest) (*meshv1.CallResponse, error) {
	switch req.GetMethod() {
	case "Settings":
		raw, err := json.Marshal(s.settings.List())
		if err != nil {
			return &meshv1.CallResponse{Error: err.Error()}, nil
		}
		return &meshv1.CallResponse{Payload: raw}, nil
	case "UpdateSetting":
		var body struct{ Key, Value string }
		_ = json.Unmarshal(req.GetPayload(), &body)
		if err := s.settings.Update(body.Key, body.Value); err != nil {
			return &meshv1.CallResponse{Error: err.Error()}, nil
		}
		return &meshv1.CallResponse{Payload: []byte(`{"ok":true}`)}, nil
	default:
		raw, err := s.h.handleMesh(ctx, req.GetMethod(), req.GetPayload())
		if err != nil {
			return &meshv1.CallResponse{Error: err.Error()}, nil
		}
		return &meshv1.CallResponse{Payload: raw}, nil
	}
}

func (s *genericMesh) StreamCall(meshv1.ModuleMesh_StreamCallServer) error {
	return fmt.Errorf("StreamCall not supported")
}

func stopServers(ctx context.Context, g *grpc.Server, h *http.Server) error {
	if g != nil {
		g.GracefulStop()
	}
	if h != nil {
		return h.Shutdown(ctx)
	}
	return nil
}

func readJSON(w http.ResponseWriter, r *http.Request, dest any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
		return false
	}
	if err := json.Unmarshal(body, dest); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
