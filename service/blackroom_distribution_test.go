package service

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupBlackroomDistributionTest(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	previousRDB, previousRedis := common.RDB, common.RedisEnabled
	previousSecret := common.CryptoSecret
	previousSetting := *operation_setting.GetBlackroomSetting()
	previousMode := gin.Mode()
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	common.RedisEnabled = true
	common.CryptoSecret = "distribution-test-secret"
	operation_setting.GetBlackroomSetting().Enabled = true
	gin.SetMode(gin.TestMode)
	client := common.RDB
	t.Cleanup(func() {
		require.NoError(t, client.Close())
		common.RDB, common.RedisEnabled = previousRDB, previousRedis
		common.CryptoSecret = previousSecret
		*operation_setting.GetBlackroomSetting() = previousSetting
		gin.SetMode(previousMode)
	})
	return server
}

func TestBlackroomDistributionSlidingWindowsAndRepeatedPrompts(t *testing.T) {
	setupBlackroomDistributionTest(t)
	ctx := context.Background()
	observation := blackroomDistributionObservation{UserID: 1, TokenID: 10, Username: "alice", TokenName: "personal", SystemHash: "fixed", FirstUserHash: "chat", Subnet: "8.8.8.0/24", ObservedAt: 1000}
	for _, at := range []int64{1000, 1000, 1010} {
		observation.ObservedAt = at
		require.NoError(t, recordBlackroomDistribution(ctx, observation))
	}
	observation.FirstUserHash = "another-chat"
	observation.Subnet = "1.1.1.0/24"
	observation.ObservedAt = 1040
	require.NoError(t, recordBlackroomDistribution(ctx, observation))

	result, err := ListBlackroomDistribution(ctx, "", 1, 20, 1040)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	token := result.Items[0].Tokens[0]
	assert.EqualValues(t, 4, token.RequestCount)
	assert.Equal(t, BlackroomPromptSummary{SampleCount: 4, UniqueCount: 1, DominantShare: 1}, token.SystemPrompt)
	assert.Equal(t, BlackroomPromptSummary{SampleCount: 4, UniqueCount: 2, DominantShare: 0.75}, token.FirstUserPrompt)

	result, err = ListBlackroomDistribution(ctx, "", 1, 20, 1060)
	require.NoError(t, err)
	token = result.Items[0].Tokens[0]
	assert.EqualValues(t, 4, token.RequestCount)
	assert.EqualValues(t, 2, token.FirstUserPrompt.SampleCount, "the exact 60-second boundary expires")
	assert.Equal(t, 0.5, token.FirstUserPrompt.DominantShare)

	result, err = ListBlackroomDistribution(ctx, "", 1, 20, 1600)
	require.NoError(t, err)
	token = result.Items[0].Tokens[0]
	assert.EqualValues(t, 2, token.RequestCount)
	assert.Equal(t, []string{"1.1.1.0/24", "8.8.8.0/24"}, token.Subnets)
	assert.Zero(t, token.SystemPrompt.SampleCount)
	result, err = ListBlackroomDistribution(ctx, "", 1, 20, 1640)
	require.NoError(t, err)
	assert.Empty(t, result.Items)
}

func TestBlackroomDistributionRanksEvidenceWithoutPoolingKeys(t *testing.T) {
	setupBlackroomDistributionTest(t)
	ctx := context.Background()
	for i := range 8 {
		require.NoError(t, recordBlackroomDistribution(ctx, blackroomDistributionObservation{
			UserID: 2, TokenID: 20, Username: "review-me", ObservedAt: 2000,
			SystemHash: strconv.Itoa(i), FirstUserHash: strconv.Itoa(i),
			Subnet: "network-" + strconv.Itoa(i%4), Country: []string{"US", "DE", "JP"}[i%3], ASN: strconv.Itoa(i%3 + 1),
			DefaultGo: true, HostingGo: true, Geo: true,
		}))
		// Two ordinary keys each have one consistent preset and first prompt.
		require.NoError(t, recordBlackroomDistribution(ctx, blackroomDistributionObservation{
			UserID: 1, TokenID: 10 + i%2, Username: "alice", ObservedAt: 2000,
			SystemHash: "preset-" + strconv.Itoa(i%2), FirstUserHash: "chat-" + strconv.Itoa(i%2),
		}))
	}
	result, err := ListBlackroomDistribution(ctx, "", 1, 20, 2000)
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	assert.Equal(t, 2, result.Items[0].UserID)
	assert.Equal(t, 100, result.Items[0].Score)
	assert.ElementsMatch(t, []string{"first_prompt_diversity", "system_prompt_diversity", "subnet_diversity", "geo_asn_diversity", "default_go_client", "hosting_go_client"}, result.Items[0].Reasons)
	assert.Zero(t, result.Items[1].Score)
	assert.Len(t, result.Items[1].Tokens, 2)
	assert.EqualValues(t, 8, result.Items[1].RequestCount)

	result, err = ListBlackroomDistribution(ctx, "ALICE", 100, 1, 2000)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Page, "oversized pages are clamped before multiplication")
	require.Len(t, result.Items, 1)
	assert.Equal(t, 1, result.Items[0].UserID)
	result, err = ListBlackroomDistribution(ctx, "2", 1, 20, 2000)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, 2, result.Items[0].UserID)
}

func TestBlackroomDistributionIncompleteAndLowSampleEvidenceDoesNotEscalate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		samples int64
		limited bool
		want    int
	}{
		{name: "low sample", samples: 7, want: 0},
		{name: "complete evidence", samples: 8, want: 80},
		{name: "incomplete evidence", samples: 8, limited: true, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := BlackroomDistributionToken{
				RequestCount: tc.samples, SystemPrompt: BlackroomPromptSummary{SampleCount: tc.samples, UniqueCount: 5, DominantShare: 0.2},
				FirstUserPrompt: BlackroomPromptSummary{SampleCount: tc.samples, UniqueCount: 5, DominantShare: 0.2},
				Subnets:         []string{"a", "b", "c", "d"}, Countries: []string{"US", "DE", "JP"}, ASNs: []int64{1, 2, 3},
				PromptLimited: tc.limited, NetworkLimited: tc.limited,
			}
			if tc.samples < 8 {
				token.Subnets, token.Countries, token.ASNs = nil, nil, nil
			}
			scoreBlackroomDistributionToken(&token)
			assert.Equal(t, tc.want, token.Score)
		})
	}
}

func TestBlackroomDistributionObserverIsPrivateIdempotentAndNormalizesNetworks(t *testing.T) {
	server := setupBlackroomDistributionTest(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("original request body"))
	c.Request.RemoteAddr = net.JoinHostPort("::ffff:8.8.8.8", "1234")
	c.Request.Header.Set("Authorization", "Bearer private-api-credential")
	c.Set("id", 3)
	c.Set("username", "private-user")
	c.Set("token_name", "personal")
	common.SetContextKey(c, constant.ContextKeyTokenId, 30)
	request := &dto.GeneralOpenAIRequest{Messages: []dto.Message{
		{Role: "system", Content: "Confidential system instructions"},
		{Role: "user", Content: "Confidential user prompt"},
	}}
	ObserveBlackroomDistribution(c, request)
	ObserveBlackroomDistribution(c, request)
	result, err := ListBlackroomDistribution(context.Background(), "", 1, 20, time.Now().Unix())
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	token := result.Items[0].Tokens[0]
	assert.EqualValues(t, 1, token.RequestCount)
	assert.Equal(t, []string{"8.8.8.0/24"}, token.Subnets)
	assert.False(t, c.IsAborted())
	body, err := io.ReadAll(c.Request.Body)
	require.NoError(t, err)
	assert.Equal(t, "original request body", string(body))
	assert.NotContains(t, server.Dump(), "Confidential")
	assert.NotContains(t, server.Dump(), "private-api-credential")
	apiJSON, err := common.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(apiJSON), "hash")
	assert.NotContains(t, string(apiJSON), "Confidential")
	server.FastForward(71 * time.Second)
	assert.False(t, server.Exists(blackroomDistributionPrefix+"3:30:prompts"), "idle prompt fingerprints have a short TTL")
}

func TestBlackroomDistributionPromptsPreserveFirstMessageAcrossProtocols(t *testing.T) {
	for _, tc := range []struct {
		name, body           string
		request              dto.Request
		wantSystem, wantUser []string
	}{
		{name: "chat excludes replies and media", request: &dto.GeneralOpenAIRequest{}, body: `{"messages":[{"role":"system","content":"preset"},{"role":"developer","content":"rules"},{"role":"user","content":[{"type":"text","text":"first"},{"type":"image_url","image_url":{"url":"private-image"}}]},{"role":"assistant","content":"reply"},{"role":"user","content":"follow-up"}]}`, wantSystem: []string{"system:preset", "developer:rules"}, wantUser: []string{"first"}},
		{name: "chat image-only first message", request: &dto.GeneralOpenAIRequest{}, body: `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"private-image"}}]},{"role":"user","content":"follow-up"}]}`},
		{name: "claude text blocks", request: &dto.ClaudeRequest{}, body: `{"system":[{"type":"text","text":"preset"}],"messages":[{"role":"user","content":[{"type":"text","text":"first"},{"type":"image","source":{"data":"private-image"}}]},{"role":"user","content":"follow-up"}]}`, wantSystem: []string{"preset"}, wantUser: []string{"first"}},
		{name: "gemini text parts", request: &dto.GeminiChatRequest{}, body: `{"systemInstruction":{"parts":[{"text":"preset"}]},"contents":[{"role":"user","parts":[{"text":"first"}]},{"role":"model","parts":[{"text":"reply"}]},{"role":"user","parts":[{"text":"follow-up"}]}]}`, wantSystem: []string{"preset"}, wantUser: []string{"first"}},
		{name: "responses skips tool items", request: &dto.OpenAIResponsesRequest{}, body: `{"instructions":"preset","input":[{"type":"function_call","arguments":"private tool input"},{"role":"user","content":[{"type":"input_text","text":"first"},{"type":"input_image","image_url":"private-image"}]},{"role":"user","content":"follow-up"}]}`, wantSystem: []string{"preset"}, wantUser: []string{"first"}},
		{name: "responses continuation is not a new first message", request: &dto.OpenAIResponsesRequest{}, body: `{"instructions":"preset","previous_response_id":"resp_previous","input":"follow-up"}`, wantSystem: []string{"preset"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, common.Unmarshal([]byte(tc.body), tc.request))
			system, first := blackroomDistributionPrompts(tc.request)
			assert.Equal(t, tc.wantSystem, system)
			assert.Equal(t, tc.wantUser, first)
		})
	}
}

func TestBlackroomDistributionFingerprintsUseFullTextAndKeyIsolation(t *testing.T) {
	setupBlackroomDistributionTest(t)
	prefix := strings.Repeat("shared-prefix", 20)
	first := blackroomPromptFingerprint(1, 10, "system", []string{prefix + "a"})
	assert.NotEqual(t, first, blackroomPromptFingerprint(1, 10, "system", []string{prefix + "b"}))
	assert.NotEqual(t, first, blackroomPromptFingerprint(2, 10, "system", []string{prefix + "a"}))
	assert.NotEqual(t, first, blackroomPromptFingerprint(1, 11, "system", []string{prefix + "a"}))
	assert.NotEqual(t, first, blackroomPromptFingerprint(1, 10, "first_user", []string{prefix + "a"}))
	assert.NotEqual(t, blackroomPromptFingerprint(1, 10, "system", []string{"a", "bc"}), blackroomPromptFingerprint(1, 10, "system", []string{"ab", "c"}))
}

func TestBlackroomDistributionRedisFailureDoesNotAbortRelay(t *testing.T) {
	server := setupBlackroomDistributionTest(t)
	server.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", 1)
	common.SetContextKey(c, constant.ContextKeyTokenId, 1)
	ObserveBlackroomDistribution(c, &dto.GeneralOpenAIRequest{})
	assert.False(t, c.IsAborted())
	_, err := ListBlackroomDistribution(context.Background(), "", 1, 20, time.Now().Unix())
	assert.Error(t, err, "a failed observation read must not look like an empty successful ranking")
	common.RedisEnabled = false
	result, err := ListBlackroomDistribution(context.Background(), "", 1, 20, time.Now().Unix())
	require.NoError(t, err)
	assert.False(t, result.Available)
	assert.Equal(t, "redis_disabled", result.UnavailableReason)
}
