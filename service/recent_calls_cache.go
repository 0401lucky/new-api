package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

const (
	RecentCallsContextKeyID       = "recent_calls_id"
	recentCallsCaptureContextKey  = "recent_calls_response_capture"
	defaultRecentCallsCapacity    = 100
	defaultMaxRequestBodyBytes    = 8 << 20
	defaultMaxResponseBodyBytes   = 256 << 10
	defaultRecentCallsTempPrefix  = "new-api-recent-calls-"
	defaultRecentCallsTempMarker  = ".new-api-recent-calls"
	defaultRecentCallsBodyReqName = "req_body.txt"
	defaultRecentCallsBodyResName = "resp_body.txt"
)

type RecentCallsCacheConfig struct {
	Capacity             int
	MaxRequestBodyBytes  int
	MaxResponseBodyBytes int
}

type RecentCallRequest struct {
	Method string            `json:"method"`
	Path   string            `json:"path"`
	Header map[string]string `json:"headers,omitempty"`

	BodyType   string `json:"body_type,omitempty"`
	Body       string `json:"body,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
	Omitted    bool   `json:"omitted,omitempty"`
	OmitReason string `json:"omit_reason,omitempty"`
}

type RecentCallResponse struct {
	StatusCode int               `json:"status_code"`
	Header     map[string]string `json:"headers,omitempty"`

	BodyType   string `json:"body_type,omitempty"`
	Body       string `json:"body,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
	Omitted    bool   `json:"omitted,omitempty"`
	OmitReason string `json:"omit_reason,omitempty"`
}

type RecentCallStream struct {
	Chunks              []string `json:"chunks,omitempty"`
	ChunksTruncated     bool     `json:"chunks_truncated,omitempty"`
	AggregatedText      string   `json:"aggregated_text,omitempty"`
	AggregatedTruncated bool     `json:"aggregated_truncated,omitempty"`
}

type RecentCallErrorInfo struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Status  int    `json:"status,omitempty"`
}

type RecentCallRecord struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"created_at"`

	UserID    int    `json:"user_id"`
	ChannelID int    `json:"channel_id,omitempty"`
	ModelName string `json:"model_name,omitempty"`

	Method string `json:"method"`
	Path   string `json:"path"`

	Request  RecentCallRequest    `json:"request"`
	Response *RecentCallResponse  `json:"response,omitempty"`
	Stream   *RecentCallStream    `json:"stream,omitempty"`
	Error    *RecentCallErrorInfo `json:"error,omitempty"`
}

type recentCallEntry struct {
	meta RecentCallRecord

	reqBodyPath  string
	respBodyPath string

	mu      sync.Mutex
	evicted bool
}

type recentCallsCache struct {
	cfg RecentCallsCacheConfig

	nextID atomic.Uint64

	mu     sync.RWMutex
	buffer []*recentCallEntry

	tempSessionDir string
}

type recentCallResponseCapture struct {
	limit     int
	buf       bytes.Buffer
	truncated bool
	mu        sync.Mutex
}

type recentCallResponseWriter struct {
	gin.ResponseWriter
	capture *recentCallResponseCapture
}

var recentCallsSingleton = newRecentCallsCache(RecentCallsCacheConfig{
	Capacity:             defaultRecentCallsCapacity,
	MaxRequestBodyBytes:  defaultMaxRequestBodyBytes,
	MaxResponseBodyBytes: defaultMaxResponseBodyBytes,
})

func RecentCallsCache() *recentCallsCache {
	return recentCallsSingleton
}

func NewRecentCallsCacheForTest(cfg RecentCallsCacheConfig) *recentCallsCache {
	return newRecentCallsCache(cfg)
}

func newRecentCallsCache(cfg RecentCallsCacheConfig) *recentCallsCache {
	if cfg.Capacity <= 0 {
		cfg.Capacity = defaultRecentCallsCapacity
	}
	if cfg.MaxRequestBodyBytes <= 0 {
		cfg.MaxRequestBodyBytes = defaultMaxRequestBodyBytes
	}
	if cfg.MaxResponseBodyBytes <= 0 {
		cfg.MaxResponseBodyBytes = defaultMaxResponseBodyBytes
	}

	return &recentCallsCache{
		cfg:            cfg,
		buffer:         make([]*recentCallEntry, cfg.Capacity),
		tempSessionDir: initRecentCallsTempDir(),
	}
}

func (cch *recentCallsCache) BeginFromContext(c *gin.Context, info *relaycommon.RelayInfo, rawRequestBody []byte) uint64 {
	if cch == nil || c == nil {
		return 0
	}

	id := cch.nextID.Add(1)
	path := ""
	if c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	method := ""
	if c.Request != nil {
		method = c.Request.Method
	}

	modelName := ""
	if info != nil {
		modelName = info.OriginModelName
		if modelName == "" {
			modelName = info.UpstreamModelName
		}
	}

	rec := RecentCallRecord{
		ID:        id,
		CreatedAt: time.Now().UTC(),
		UserID:    common.GetContextKeyInt(c, constant.ContextKeyUserId),
		ChannelID: common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		ModelName: modelName,
		Method:    method,
		Path:      path,
		Request: RecentCallRequest{
			Method: method,
			Path:   path,
			Header: sanitizeRecentCallHeaders(c.Request.Header),
		},
	}

	contentType := ""
	if c.Request != nil {
		contentType = c.Request.Header.Get("Content-Type")
	}
	bodyType, body, truncated, omitted, omitReason := encodeRecentCallBody(contentType, rawRequestBody, cch.cfg.MaxRequestBodyBytes)
	rec.Request.BodyType = bodyType
	rec.Request.Truncated = truncated
	rec.Request.Omitted = omitted
	rec.Request.OmitReason = omitReason

	entry := &recentCallEntry{meta: rec}
	entry.reqBodyPath = cch.pathForID(id, defaultRecentCallsBodyReqName)
	if !omitted && body != "" {
		if entry.reqBodyPath == "" {
			entry.meta.Request.Omitted = true
			entry.meta.Request.OmitReason = "temp_dir_unavailable"
		} else if err := os.WriteFile(entry.reqBodyPath, []byte(body), 0o600); err != nil {
			entry.meta.Request.Omitted = true
			entry.meta.Request.OmitReason = "temp_write_failed"
			entry.reqBodyPath = ""
		}
	}

	c.Set(RecentCallsContextKeyID, id)
	cch.put(entry)
	return id
}

func (cch *recentCallsCache) UpsertErrorByContext(c *gin.Context, errMsg string, errType string, errCode string, status int) {
	if cch == nil || c == nil {
		return
	}
	id := getRecentCallID(c)
	if id == 0 {
		return
	}

	entry := cch.get(id)
	if entry == nil {
		return
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.evicted {
		return
	}
	entry.meta.Error = &RecentCallErrorInfo{
		Message: errMsg,
		Type:    errType,
		Code:    errCode,
		Status:  status,
	}
}

func (cch *recentCallsCache) UpsertResponseByContext(c *gin.Context, rawResponseBody []byte, truncated bool) {
	if cch == nil || c == nil {
		return
	}
	id := getRecentCallID(c)
	if id == 0 {
		return
	}

	entry := cch.get(id)
	if entry == nil {
		return
	}

	contentType := c.Writer.Header().Get("Content-Type")
	bodyType, body, bodyTruncated, omitted, omitReason := encodeRecentCallBody(contentType, rawResponseBody, cch.cfg.MaxResponseBodyBytes)
	if truncated {
		bodyTruncated = true
	}

	statusCode := c.Writer.Status()
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.evicted {
		return
	}

	entry.meta.Response = &RecentCallResponse{
		StatusCode: statusCode,
		Header:     sanitizeRecentCallHeaders(c.Writer.Header()),
		BodyType:   bodyType,
		Truncated:  bodyTruncated,
		Omitted:    omitted,
		OmitReason: omitReason,
	}
	if statusCode < http.StatusBadRequest {
		entry.meta.Error = nil
	}

	entry.respBodyPath = cch.pathForID(id, defaultRecentCallsBodyResName)
	if !omitted && body != "" {
		if entry.respBodyPath == "" {
			entry.meta.Response.Omitted = true
			entry.meta.Response.OmitReason = "temp_dir_unavailable"
		} else if err := os.WriteFile(entry.respBodyPath, []byte(body), 0o600); err != nil {
			entry.meta.Response.Omitted = true
			entry.meta.Response.OmitReason = "temp_write_failed"
			entry.respBodyPath = ""
		}
	}

	if strings.Contains(strings.ToLower(contentType), "text/event-stream") && body != "" {
		stream := parseRecentCallEventStream(body)
		if bodyTruncated {
			stream.ChunksTruncated = true
			stream.AggregatedTruncated = true
		}
		entry.meta.Stream = stream
	}
}

func (cch *recentCallsCache) Get(id uint64) (*RecentCallRecord, bool) {
	entry := cch.get(id)
	if entry == nil {
		return nil, false
	}
	return cch.materializeEntry(entry, true)
}

func (cch *recentCallsCache) List(limit int, beforeID uint64) []*RecentCallRecord {
	if cch == nil {
		return nil
	}
	if limit <= 0 {
		limit = cch.cfg.Capacity
	}
	if limit > cch.cfg.Capacity {
		limit = cch.cfg.Capacity
	}

	cch.mu.RLock()
	items := make([]*recentCallEntry, 0, limit)
	for _, entry := range cch.buffer {
		if entry == nil {
			continue
		}
		if beforeID != 0 && entry.meta.ID >= beforeID {
			continue
		}
		items = append(items, entry)
	}
	cch.mu.RUnlock()

	sort.Slice(items, func(i, j int) bool { return items[i].meta.ID > items[j].meta.ID })
	if len(items) > limit {
		items = items[:limit]
	}

	out := make([]*RecentCallRecord, 0, len(items))
	for _, entry := range items {
		rec, ok := cch.materializeEntry(entry, false)
		if ok {
			out = append(out, rec)
		}
	}
	return out
}

func (cch *recentCallsCache) TempSessionDirForTest() string {
	if cch == nil {
		return ""
	}
	return cch.tempSessionDir
}

func (cch *recentCallsCache) get(id uint64) *recentCallEntry {
	if cch == nil || id == 0 {
		return nil
	}
	cch.mu.RLock()
	defer cch.mu.RUnlock()
	idx := int(id % uint64(cch.cfg.Capacity))
	entry := cch.buffer[idx]
	if entry == nil || entry.meta.ID != id {
		return nil
	}
	return entry
}

func (cch *recentCallsCache) put(entry *recentCallEntry) {
	if cch == nil || entry == nil {
		return
	}
	idx := int(entry.meta.ID % uint64(cch.cfg.Capacity))
	cch.mu.Lock()
	old := cch.buffer[idx]
	cch.buffer[idx] = entry
	cch.mu.Unlock()

	if old != nil {
		old.mu.Lock()
		old.evicted = true
		reqPath := old.reqBodyPath
		respPath := old.respBodyPath
		old.mu.Unlock()
		removeRecentCallFiles(reqPath, respPath)
	}
}

func (cch *recentCallsCache) materializeEntry(entry *recentCallEntry, includeBody bool) (*RecentCallRecord, bool) {
	if entry == nil {
		return nil, false
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.evicted {
		return nil, false
	}

	dup := entry.meta
	if dup.Response != nil {
		resp := *dup.Response
		dup.Response = &resp
	}
	if dup.Stream != nil {
		stream := *dup.Stream
		if dup.Stream.Chunks != nil {
			stream.Chunks = append([]string(nil), dup.Stream.Chunks...)
		}
		dup.Stream = &stream
	}
	if dup.Error != nil {
		errInfo := *dup.Error
		dup.Error = &errInfo
	}

	if !includeBody {
		return &dup, true
	}

	if entry.reqBodyPath != "" && !dup.Request.Omitted {
		if body, err := os.ReadFile(entry.reqBodyPath); err == nil {
			dup.Request.Body = string(body)
		}
	}
	if entry.respBodyPath != "" && dup.Response != nil && !dup.Response.Omitted {
		if body, err := os.ReadFile(entry.respBodyPath); err == nil {
			dup.Response.Body = string(body)
		}
	}
	return &dup, true
}

func AttachRecentCallResponseCapture(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	if getRecentCallID(c) == 0 {
		return
	}
	if _, ok := c.Get(recentCallsCaptureContextKey); ok {
		return
	}

	capture := &recentCallResponseCapture{
		limit: RecentCallsCache().cfg.MaxResponseBodyBytes,
	}
	c.Set(recentCallsCaptureContextKey, capture)
	c.Writer = &recentCallResponseWriter{
		ResponseWriter: c.Writer,
		capture:        capture,
	}
}

func FinalizeRecentCallResponse(c *gin.Context) {
	if c == nil {
		return
	}
	value, ok := c.Get(recentCallsCaptureContextKey)
	if !ok {
		return
	}
	capture, ok := value.(*recentCallResponseCapture)
	if !ok || capture == nil {
		return
	}
	body, truncated := capture.snapshot()
	RecentCallsCache().UpsertResponseByContext(c, body, truncated)
}

func (w *recentCallResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if n > 0 && w.capture != nil {
		w.capture.append(data[:n])
	}
	return n, err
}

func (w *recentCallResponseWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	if n > 0 && w.capture != nil {
		w.capture.append([]byte(s[:n]))
	}
	return n, err
}

func (c *recentCallResponseCapture) append(data []byte) {
	if c == nil || len(data) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.limit <= 0 {
		c.limit = defaultMaxResponseBodyBytes
	}
	if c.buf.Len() >= c.limit {
		c.truncated = true
		return
	}
	remaining := c.limit - c.buf.Len()
	if len(data) > remaining {
		_, _ = c.buf.Write(data[:remaining])
		c.truncated = true
		return
	}
	_, _ = c.buf.Write(data)
}

func (c *recentCallResponseCapture) snapshot() ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf.Bytes()...), c.truncated
}

func getRecentCallID(c *gin.Context) uint64 {
	if c == nil {
		return 0
	}
	raw, ok := c.Get(RecentCallsContextKeyID)
	if !ok {
		return 0
	}
	id, _ := raw.(uint64)
	return id
}

func encodeRecentCallBody(contentType string, body []byte, limit int) (bodyType string, encoded string, truncated bool, omitted bool, omitReason string) {
	if len(body) == 0 {
		return "unknown", "", false, true, "empty"
	}

	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.HasPrefix(ct, "application/json") ||
		strings.HasPrefix(ct, "text/") ||
		strings.Contains(ct, "application/x-www-form-urlencoded") ||
		strings.Contains(ct, "application/problem+json") {
		bodyType = "text"
		if strings.HasPrefix(ct, "application/json") || strings.Contains(ct, "+json") {
			bodyType = "json"
		}
		s := string(body)
		if limit > 0 && len(s) > limit {
			s = s[:limit]
			truncated = true
		}
		return bodyType, s, truncated, false, ""
	}

	if strings.Contains(ct, "multipart/form-data") {
		return "binary", "", false, true, "multipart_form_data"
	}

	if strings.HasPrefix(ct, "application/octet-stream") {
		b := body
		if limit > 0 && len(b) > limit {
			b = b[:limit]
			truncated = true
		}
		return "binary", base64.StdEncoding.EncodeToString(b), truncated, false, ""
	}

	s := string(body)
	if limit > 0 && len(s) > limit {
		s = s[:limit]
		truncated = true
	}
	return "unknown", s, truncated, false, ""
}

func sanitizeRecentCallHeaders(headers http.Header) map[string]string {
	if headers == nil {
		return nil
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if len(v) == 0 {
			continue
		}
		if isSensitiveRecentCallHeader(k) {
			out[k] = maskRecentCallHeader(v[0])
			continue
		}
		out[k] = strings.Join(v, ", ")
	}
	return out
}

func isSensitiveRecentCallHeader(key string) bool {
	switch strings.ToLower(key) {
	case "authorization", "proxy-authorization", "x-api-key", "x-goog-api-key", "cookie", "set-cookie":
		return true
	default:
		return false
	}
}

func maskRecentCallHeader(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "***" + value[len(value)-4:]
}

func parseRecentCallEventStream(body string) *RecentCallStream {
	stream := &RecentCallStream{}
	lines := strings.Split(body, "\n")
	var text strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ":") || !strings.HasPrefix(trimmed, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		stream.Chunks = append(stream.Chunks, payload)
		text.WriteString(extractRecentCallTextDelta(payload))
	}
	stream.AggregatedText = text.String()
	return stream
}

func extractRecentCallTextDelta(payload string) string {
	var obj map[string]interface{}
	if err := common.Unmarshal([]byte(payload), &obj); err != nil {
		return ""
	}

	if delta, ok := stringField(obj, "delta"); ok {
		return delta
	}
	if text, ok := stringField(obj, "text"); ok {
		return text
	}

	if choices, ok := obj["choices"].([]interface{}); ok {
		var out strings.Builder
		for _, rawChoice := range choices {
			choice, ok := rawChoice.(map[string]interface{})
			if !ok {
				continue
			}
			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				if content, ok := stringField(delta, "content"); ok {
					out.WriteString(content)
				}
				if text, ok := stringField(delta, "text"); ok {
					out.WriteString(text)
				}
			}
			if text, ok := stringField(choice, "text"); ok {
				out.WriteString(text)
			}
		}
		return out.String()
	}

	if deltaObj, ok := obj["delta"].(map[string]interface{}); ok {
		if text, ok := stringField(deltaObj, "text"); ok {
			return text
		}
		if content, ok := stringField(deltaObj, "content"); ok {
			return content
		}
	}

	if candidates, ok := obj["candidates"].([]interface{}); ok {
		return extractGeminiCandidateText(candidates)
	}
	return ""
}

func extractGeminiCandidateText(candidates []interface{}) string {
	var out strings.Builder
	for _, rawCandidate := range candidates {
		candidate, ok := rawCandidate.(map[string]interface{})
		if !ok {
			continue
		}
		content, ok := candidate["content"].(map[string]interface{})
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]interface{})
		if !ok {
			continue
		}
		for _, rawPart := range parts {
			part, ok := rawPart.(map[string]interface{})
			if !ok {
				continue
			}
			if text, ok := stringField(part, "text"); ok {
				out.WriteString(text)
			}
		}
	}
	return out.String()
}

func stringField(obj map[string]interface{}, key string) (string, bool) {
	value, ok := obj[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	return text, true
}

func initRecentCallsTempDir() string {
	base := os.TempDir()
	dir, err := os.MkdirTemp(base, defaultRecentCallsTempPrefix)
	if err != nil {
		return ""
	}
	_ = os.WriteFile(filepath.Join(dir, defaultRecentCallsTempMarker), []byte("ok\n"), 0o600)

	entries, err := os.ReadDir(base)
	if err != nil {
		return dir
	}
	for _, de := range entries {
		if !de.IsDir() || !strings.HasPrefix(de.Name(), defaultRecentCallsTempPrefix) {
			continue
		}
		full := filepath.Join(base, de.Name())
		if sameRecentCallPath(full, dir) {
			continue
		}
		if _, err := os.Stat(filepath.Join(full, defaultRecentCallsTempMarker)); err == nil {
			_ = os.RemoveAll(full)
		}
	}
	return dir
}

func sameRecentCallPath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil && errB == nil {
		return strings.EqualFold(aa, bb)
	}
	return a == b
}

func (cch *recentCallsCache) pathForID(id uint64, name string) string {
	if cch == nil || id == 0 || name == "" || cch.tempSessionDir == "" {
		return ""
	}
	return filepath.Join(cch.tempSessionDir, fmt.Sprintf("%d_%s", id, name))
}

func removeRecentCallFiles(paths ...string) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		_ = os.Remove(path)
	}
}
