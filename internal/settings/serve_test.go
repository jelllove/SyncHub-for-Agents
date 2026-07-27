package settings

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestServeRespondsThenShutsDown(t *testing.T) {
	home := setupHome(t)

	addr, shutdown, err := Serve(home)
	if err != nil {
		t.Fatalf("Serve error: %v", err)
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("addr = %q, want 127.0.0.1:<port>", addr)
	}

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "AgentConfigSync Settings") {
		t.Error("body should contain the settings heading")
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
	if _, err := http.Get("http://" + addr + "/"); err == nil {
		t.Error("server should be down after shutdown")
	}
}
