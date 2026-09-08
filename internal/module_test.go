package internal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestApplyHTTP(t *testing.T) {
	m := New(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(context.Background()) })
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + m.HTTPListenAddr() + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	body := `{"media_id":"m1","profile_id":"r-to-pg13","cues":[{"start_ms":0,"end_ms":1000,"text":"What the fuck"},{"start_ms":1000,"end_ms":4000,"text":"They have sex now"}]}`
	resp, err := http.Post("http://"+m.HTTPListenAddr()+"/v1/apply", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, raw)
	}
	var plan Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Bleeps) == 0 || len(plan.Cuts) == 0 {
		t.Fatalf("%s", raw)
	}
}
