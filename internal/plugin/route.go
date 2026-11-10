package plugin

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/yuluo688/credit-manager/internal/service"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func routeModel(raw []byte) ([]byte, error) {
	var req rpcModelRouteRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	if isNativeImageProtocol(req.SourceFormat) || isImageOnlyModel(req.RequestedModel) {
		return okEnvelope(pluginapi.ModelRouteResponse{Handled: false, Reason: "native_image_protocol"})
	}
	svc := service.Current()
	if svc == nil {
		return okEnvelope(pluginapi.ModelRouteResponse{Handled: false})
	}
	if _, _, err := svc.ResolveIdentity(context.Background(), req.Headers, req.Metadata); err != nil {
		return okEnvelope(pluginapi.ModelRouteResponse{Handled: false})
	}
	return okEnvelope(pluginapi.ModelRouteResponse{
		Handled:    true,
		TargetKind: pluginapi.ModelRouteTargetSelf,
		Reason:     "credit_manager_authenticated",
	})
}

func isNativeImageProtocol(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "openai-image", "openai-video":
		return true
	default:
		return false
	}
}

func isImageOnlyModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	switch model {
	case "gpt-image-1", "gpt-image-1.5", "gpt-image-2", "grok-imagine-image", "grok-imagine-image-quality":
		return true
	default:
		return strings.Contains(model, "imagine-image") || strings.Contains(model, "imagine-video")
	}
}
