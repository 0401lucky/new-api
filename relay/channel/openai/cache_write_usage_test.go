package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResponsesHandlersPropagateCacheWriteTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":20,"cached_creation_tokens":10,"cache_write_tokens":30}}}`

	t.Run("responses", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
		usage, apiErr := OaiResponsesHandler(ctx, nil, resp)
		require.Nil(t, apiErr)
		require.Equal(t, 30, usage.PromptTokensDetails.CacheWriteTokens)
		require.Equal(t, 10, usage.PromptTokensDetails.CachedCreationTokens)
	})

	t.Run("compact", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
		usage, apiErr := OaiResponsesCompactionHandler(ctx, resp)
		require.Nil(t, apiErr)
		require.Equal(t, 30, usage.PromptTokensDetails.CacheWriteTokens)
		require.Equal(t, 10, usage.PromptTokensDetails.CachedCreationTokens)
	})

	t.Run("stream", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		streamBody := "data: {\"type\":\"response.completed\",\"response\":" + body + "}\n\ndata: [DONE]\n\n"
		resp := &http.Response{
			Body:   io.NopCloser(strings.NewReader(streamBody)),
			Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		}
		usage, apiErr := OaiResponsesStreamHandler(ctx, &relaycommon.RelayInfo{}, resp)
		require.Nil(t, apiErr)
		require.Equal(t, 30, usage.PromptTokensDetails.CacheWriteTokens)
		require.Equal(t, 10, usage.PromptTokensDetails.CachedCreationTokens)
	})
}

func TestOpenAIHandlerNormalizesInputCacheWriteTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := `{"usage":{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":20,"cached_creation_tokens":10,"cache_write_tokens":30}}}`
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}

	usage, apiErr := OpenaiHandlerWithUsage(ctx, nil, resp)

	require.Nil(t, apiErr)
	require.Equal(t, 30, usage.PromptTokensDetails.CacheWriteTokens)
	require.Equal(t, 10, usage.PromptTokensDetails.CachedCreationTokens)
}
