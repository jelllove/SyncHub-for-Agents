package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDeviceFlowPendingThenSucceeds(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/login/device/code":
			_, _ = io.WriteString(writer, `{"device_code":"backend-only","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","expires_in":60,"interval":1}`)
		case "/login/oauth/access_token":
			if polls.Add(1) == 1 {
				_, _ = io.WriteString(writer, `{"error":"authorization_pending"}`)
			} else {
				_, _ = io.WriteString(writer, `{"access_token":"gho_secret","token_type":"bearer","scope":"repo"}`)
			}
		case "/user":
			if request.Header.Get("Authorization") != "Bearer gho_secret" {
				t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(writer, `{"id":42,"login":"alice"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := NewOAuthClient("client-id", server.Client())
	client.BaseURL = server.URL
	client.APIURL = server.URL
	client.Sleep = func(context.Context, time.Duration) error { return nil }

	start, err := client.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if start.UserCode != "ABCD-EFGH" || start.VerificationURI != "https://github.com/login/device" {
		t.Fatalf("Start() = %#v", start)
	}
	account, token, err := client.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if account.ID != 42 || account.Login != "alice" || token != "gho_secret" {
		t.Fatalf("account=%#v token=%q", account, token)
	}
}

func TestDeviceFlowRejectsAccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/login/device/code" {
			_, _ = io.WriteString(writer, `{"device_code":"private","user_code":"ABCD","verification_uri":"https://github.com/login/device","expires_in":60,"interval":1}`)
			return
		}
		_, _ = io.WriteString(writer, `{"error":"access_denied"}`)
	}))
	defer server.Close()

	client := NewOAuthClient("client-id", server.Client())
	client.BaseURL = server.URL
	client.Sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := client.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Wait(context.Background()); err != ErrOAuthDenied {
		t.Fatalf("Wait() error = %v, want ErrOAuthDenied", err)
	}
}
