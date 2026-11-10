package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yuluo688/credit-manager/internal/service"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestPublicModelDirectoryRequest(t *testing.T) {
	for _, test := range []struct {
		method string
		path   string
		want   bool
	}{
		{"GET", "/v1/models", true},
		{"get", "/v1/models/", true},
		{"GET", "/v1beta/models", true},
		{"POST", "/v1/models", false},
		{"GET", "/v1/chat/completions", false},
		{"GET", "/v0/management/config", false},
	} {
		if got := publicModelDirectoryRequest(test.method, test.path); got != test.want {
			t.Errorf("publicModelDirectoryRequest(%q, %q) = %t, want %t", test.method, test.path, got, test.want)
		}
	}
}

func TestAllowedAuthQuotaRequest(t *testing.T) {
	for _, test := range []struct {
		method, url string
		want        bool
	}{
		{http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", true},
		{http.MethodGet, "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits", true},
		{http.MethodGet, "https://cli-chat-proxy.grok.com/v1/billing?format=credits", true},
		{http.MethodPost, "https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary", true},
		{http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", true},
		{http.MethodPost, "https://chatgpt.com/backend-api/wham/usage", false},
		{http.MethodGet, "http://chatgpt.com/backend-api/wham/usage", false},
		{http.MethodGet, "https://evil.example/backend-api/wham/usage", false},
		{http.MethodGet, "https://chatgpt.com.evil.example/backend-api/wham/usage", false},
		{http.MethodGet, "https://api.anthropic.com/api/oauth/usage", true},
		{http.MethodGet, "https://api.anthropic.com/api/oauth/profile", true},
		{http.MethodPost, "https://api.anthropic.com/api/oauth/profile", false},
		{http.MethodGet, "https://api.x.ai/v1/billing", true},
	} {
		if got := allowedAuthQuotaRequest(test.method, test.url); got != test.want {
			t.Errorf("allowedAuthQuotaRequest(%q, %q) = %t, want %t", test.method, test.url, got, test.want)
		}
	}
}

func TestPluginRegistrationIncludesImageFormats(t *testing.T) {
	reg := pluginRegistration(negotiateRPCSchema(0))
	if !reg.Capabilities.RequestInterceptor || !reg.Capabilities.RequestLifecyclePlugin || !reg.Capabilities.ResponseInterceptor || !reg.Capabilities.Scheduler {
		t.Fatalf("intercept capabilities missing: %+v", reg.Capabilities)
	}
	want := []string{"openai-image", "openai-video"}
	got := map[string]bool{}
	for _, format := range reg.Capabilities.ExecutorInputFormats {
		got[format] = true
	}
	for _, format := range want {
		if !got[format] {
			t.Fatalf("executor formats missing %q: %v", format, reg.Capabilities.ExecutorInputFormats)
		}
	}
}

func TestRouteModelLeavesImageRequestsToNativeExecutor(t *testing.T) {
	raw, err := json.Marshal(pluginapi.ModelRouteRequest{
		SourceFormat:   "openai-image",
		RequestedModel: "grok-imagine-image",
		Headers:        http.Header{"Authorization": []string{"Bearer tk-test"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := routeModel(raw)
	if err != nil {
		t.Fatal(err)
	}
	var env envelope
	if err := json.Unmarshal(resp, &env); err != nil || !env.OK {
		t.Fatalf("envelope = %+v err=%v", env, err)
	}
	var routed pluginapi.ModelRouteResponse
	if err := json.Unmarshal(env.Result, &routed); err != nil {
		t.Fatal(err)
	}
	if routed.Handled {
		t.Fatalf("image route handled=%t, want native fallback", routed.Handled)
	}
}

func TestPluginRegistrationNotExclusive(t *testing.T) {
	reg := pluginRegistration(negotiateRPCSchema(0))
	if reg.Capabilities.FrontendAuthProviderExclusive {
		t.Fatal("FrontendAuthProviderExclusive must be false")
	}
}

func TestRouteModelNonPluginKeyFallsBack(t *testing.T) {
	newLifecycleTestService(t)
	raw, err := json.Marshal(pluginapi.ModelRouteRequest{
		SourceFormat:   "openai",
		RequestedModel: "gpt-4o",
		Headers:        http.Header{"Authorization": []string{"Bearer sk-existing-host-key"}},
		Metadata:       map[string]any{"caller_scope": "scope-for-host-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := routeModel(raw)
	if err != nil {
		t.Fatal(err)
	}
	var env envelope
	if err := json.Unmarshal(resp, &env); err != nil || !env.OK {
		t.Fatalf("envelope = %+v err=%v", env, err)
	}
	var routed pluginapi.ModelRouteResponse
	if err := json.Unmarshal(env.Result, &routed); err != nil {
		t.Fatal(err)
	}
	if routed.Handled {
		t.Fatalf("non-plugin key was routed to plugin executor: %+v", routed)
	}
}

func TestRouteModelPluginKeyHandled(t *testing.T) {
	ctx := context.Background()
	svc := newLifecycleTestService(t)
	_, material, err := svc.MintKey(ctx, service.BootstrapCallerID, "route-test", 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(pluginapi.ModelRouteRequest{
		SourceFormat:   "openai",
		RequestedModel: "gpt-4o",
		Headers:        http.Header{"Authorization": []string{"Bearer " + material.Plaintext}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := routeModel(raw)
	if err != nil {
		t.Fatal(err)
	}
	var env envelope
	if err := json.Unmarshal(resp, &env); err != nil || !env.OK {
		t.Fatalf("envelope = %+v err=%v", env, err)
	}
	var routed pluginapi.ModelRouteResponse
	if err := json.Unmarshal(env.Result, &routed); err != nil {
		t.Fatal(err)
	}
	if !routed.Handled || routed.TargetKind != pluginapi.ModelRouteTargetSelf {
		t.Fatalf("plugin key route handled=%t target=%s, want handled=true target=self", routed.Handled, routed.TargetKind)
	}
}

func TestInterceptRequestAfterAuthNonPluginKeyAllowsImage(t *testing.T) {
	newLifecycleTestService(t)
	request := pluginapi.RequestInterceptRequest{
		RequestID:    "image-non-plugin",
		SourceFormat: "openai-image",
		Model:        "grok-imagine-image",
		Headers:      http.Header{"Authorization": []string{"Bearer sk-host-key"}},
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	responseRaw, err := interceptRequestAfterAuth(raw)
	if err != nil {
		t.Fatal(err)
	}
	var env envelope
	if err := json.Unmarshal(responseRaw, &env); err != nil || !env.OK {
		t.Fatalf("interceptor envelope = %+v err=%v", env, err)
	}
	var response pluginapi.RequestInterceptResponse
	if err := json.Unmarshal(env.Result, &response); err != nil {
		t.Fatal(err)
	}
	if response.Terminate || response.StatusCode != 0 {
		t.Fatalf("non-plugin image request was rejected: terminate=%t status=%d", response.Terminate, response.StatusCode)
	}
}
