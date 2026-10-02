package server

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
)

// Tool diagnostics wrap the SDK's validation and typed-handler conversion. All
// registered tools share this boundary; non-tool protocol methods are untouched.
func toolDiagnostics(logger core.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			id := uuid.NewString()
			ctx = diagnostics.WithRequestID(ctx, id)
			ctx = mtlog.PushProperty(ctx, "RequestID", id)
			result, err := next(ctx, method, req)
			outcome, code := "success", ""
			if err != nil {
				outcome = "error"
				wireCode := int64(jsonrpc.CodeInternalError)
				public := diagnostics.Public("protocol_error")
				var wire *jsonrpc.Error
				if errors.As(err, &wire) {
					wireCode = wire.Code
					if wireCode == jsonrpc.CodeInvalidParams {
						public = diagnostics.Public("invalid_arguments")
					}
				}
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					public = diagnostics.Classify(err)
				}
				code = public.Code
				data, _ := json.Marshal(map[string]any{"request_id": id, "error": public})
				logger.WithContext(ctx).Error("MCP request {RequestID} completed: {Outcome} {ErrorCode}", id, outcome, code)
				return nil, &jsonrpc.Error{Code: wireCode, Message: public.Message, Data: data}
			}
			if toolResult, ok := result.(*mcp.CallToolResult); ok && toolResult != nil {
				// Copy the result and metadata so a shared handler result cannot be mutated.
				copyResult := *toolResult
				copyResult.Meta = mcp.Meta{}
				for key, value := range toolResult.Meta {
					copyResult.Meta[key] = value
				}
				copyResult.Meta[diagnostics.RequestIDMeta] = id
				if toolResult.IsError {
					outcome = "error"
					public := diagnostics.Classify(toolResult.GetError())
					code = public.Code
					payload, _ := json.Marshal(map[string]any{"request_id": id, "error": public})
					copyResult.Content = []mcp.Content{&mcp.TextContent{Text: string(payload)}}
					copyResult.StructuredContent = nil
					copyResult.Meta[diagnostics.ErrorMeta] = public
				}
				result = &copyResult
			}
			if outcome == "error" {
				logger.WithContext(ctx).Error("MCP request {RequestID} completed: {Outcome} {ErrorCode}", id, outcome, code)
			} else {
				logger.WithContext(ctx).Information("MCP request {RequestID} completed: {Outcome}", id, outcome)
			}
			return result, nil
		}
	}
}
