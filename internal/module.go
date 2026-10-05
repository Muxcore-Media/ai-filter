package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	manifest "github.com/Muxcore-Media/ai-filter"
	"github.com/Muxcore-Media/contracts-ai/infer"
	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"google.golang.org/grpc"
)

type Module struct {
	id, grpcAddr, httpAddr string
	mu                     sync.RWMutex
	profiles               map[string]Profile
	plans                  []Plan
	grpcSrv                *grpc.Server
	lis                    net.Listener
	httpSrv                *http.Server
}

func NewModule() *Module { return New(Config{}) }

type Config struct{ ID, GRPCAddr, HTTPAddr string }

func New(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "ai-filter"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = "127.0.0.1:9768"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = "127.0.0.1:9769"
	}
	if v := os.Getenv("AI_FILTER_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("MUXCORE_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	m := &Module{id: cfg.ID, grpcAddr: cfg.GRPCAddr, httpAddr: cfg.HTTPAddr, profiles: map[string]Profile{}}
	for _, p := range builtinProfiles() {
		m.profiles[p.ID] = p
	}
	return m
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID: m.id, Name: "AI Filter", Version: modulesdk.ManifestVersion(manifest.ManifestJSON),
		Roles: []string{"ai"}, Description: "Admin-configurable AI content filter: bleep words and remove scene types",
		Author: "Muxcore-Media", Capabilities: []string{"ai.filter", "settings"},
		MinCoreVersion: MinCoreVersion, HTTPAddr: m.grpcAddr,
	}
}

func (m *Module) Init(context.Context) error { return nil }
func (m *Module) Start(ctx context.Context) error {
	return startHTTPGRPC(ctx, m.id, &m.grpcAddr, &m.httpAddr, &m.lis, &m.grpcSrv, &m.httpSrv, m, m.routes)
}
func (m *Module) Stop(ctx context.Context) error { return stopServers(ctx, m.grpcSrv, m.httpSrv) }
func (m *Module) Health(context.Context) error   { return nil }
func (m *Module) GRPCListenAddr() string         { return m.grpcAddr }
func (m *Module) HTTPListenAddr() string         { return m.httpAddr }
func (m *Module) Settings() []contracts.SettingDef {
	return []contracts.SettingDef{{Key: "default_profile", Label: "Default profile", Type: contracts.SettingTypeString, Value: "r-to-pg13", Group: "AI"}}
}
func (m *Module) UpdateSetting(key, value string) error {
	if key != "default_profile" {
		return fmt.Errorf("unknown setting %q", key)
	}
	return nil
}

func (m *Module) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/profiles", func(w http.ResponseWriter, _ *http.Request) {
		m.mu.RLock()
		defer m.mu.RUnlock()
		out := make([]Profile, 0, len(m.profiles))
		for _, p := range m.profiles {
			out = append(out, p)
		}
		writeJSON(w, map[string]any{"profiles": out})
	})
	mux.HandleFunc("POST /v1/profiles", func(w http.ResponseWriter, r *http.Request) {
		var p Profile
		if !readJSON(w, r, &p) {
			return
		}
		if p.ID == "" || p.Name == "" {
			http.Error(w, `{"error":"id and name required"}`, http.StatusBadRequest)
			return
		}
		if p.BleepToken == "" {
			p.BleepToken = "bleep"
		}
		p.Enabled = true
		m.mu.Lock()
		m.profiles[p.ID] = p
		m.mu.Unlock()
		writeJSON(w, p)
	})
	mux.HandleFunc("POST /v1/apply", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MediaID   string                `json:"media_id"`
			ProfileID string                `json:"profile_id"`
			Cues      []infer.TranscriptCue `json:"cues"`
			Scenes    []SceneTag            `json:"scenes"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		m.mu.RLock()
		p, ok := m.profiles[body.ProfileID]
		m.mu.RUnlock()
		if !ok {
			http.Error(w, `{"error":"unknown profile"}`, http.StatusNotFound)
			return
		}
		scenes := body.Scenes
		if len(scenes) == 0 {
			scenes = inferScenesFromCues(body.Cues)
		}
		plan := applyFilter(p, body.MediaID, body.Cues, scenes)
		plan.PlanID = fmt.Sprintf("plan-%d", len(m.plans)+1)
		m.mu.Lock()
		m.plans = append(m.plans, plan)
		m.mu.Unlock()
		writeJSON(w, plan)
	})
}

func (m *Module) handleMesh(_ context.Context, method string, payload []byte) ([]byte, error) {
	switch method {
	case "ListProfiles":
		m.mu.RLock()
		defer m.mu.RUnlock()
		out := make([]Profile, 0, len(m.profiles))
		for _, p := range m.profiles {
			out = append(out, p)
		}
		return json.Marshal(map[string]any{"profiles": out})
	case "Apply":
		var body struct {
			MediaID   string                `json:"media_id"`
			ProfileID string                `json:"profile_id"`
			Cues      []infer.TranscriptCue `json:"cues"`
			Scenes    []SceneTag            `json:"scenes"`
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return nil, err
		}
		m.mu.RLock()
		p, ok := m.profiles[body.ProfileID]
		m.mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("unknown profile")
		}
		scenes := body.Scenes
		if len(scenes) == 0 {
			scenes = inferScenesFromCues(body.Cues)
		}
		plan := applyFilter(p, body.MediaID, body.Cues, scenes)
		return json.Marshal(plan)
	default:
		return nil, fmt.Errorf("unknown method %s", method)
	}
}
