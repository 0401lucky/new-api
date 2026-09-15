package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// DonationReviewInput is the strict allowlisted body of a review action.
type DonationReviewInput struct {
	Kind                 string
	ExpectedItemRevision int64
	ReviewTargetRevision string
	Note                 string
}

// DonationTestInput is the strict allowlisted body of a controlled test.
type DonationTestInput struct {
	ExpectedItemRevision int64
	ReviewTargetRevision string
	Model                string
	Prompt               string
	SystemPrompt         string
	MaxOutputTokens      int
	Stream               bool
}

// DonationTestAttemptView is the persisted metadata of one test. The prompt and
// the model output are never part of it.
type DonationTestAttemptView struct {
	TestID               string `json:"test_id"`
	ActorID              int    `json:"actor_id"`
	BatchID              string `json:"batch_id"`
	ItemID               string `json:"item_id"`
	StartRevision        int64  `json:"start_revision"`
	ReviewTargetRevision string `json:"target_revision"`
	Model                string `json:"model"`
	Stream               bool   `json:"stream"`
	PromptBytes          int    `json:"prompt_bytes"`
	State                string `json:"state"`
	ReasonCode           string `json:"reason_code"`
	StatusCode           int    `json:"status_code"`
	OutputBytes          int    `json:"output_bytes"`
	InputTokens          int    `json:"input_tokens,omitempty"`
	OutputTokens         int    `json:"output_tokens,omitempty"`
	StartedAtMS          int64  `json:"started_at_ms"`
	FinishedAtMS         *int64 `json:"finished_at_ms"`
}

func DonationTestAttemptViewOf(attempt model.DonationTestAttempt) DonationTestAttemptView {
	return DonationTestAttemptView{TestID: attempt.TestID, ActorID: attempt.ActorID, BatchID: attempt.BatchID, ItemID: attempt.ItemID,
		StartRevision: attempt.StartRevision, ReviewTargetRevision: attempt.ReviewTargetRevision, Model: attempt.Model,
		Stream: attempt.Stream, PromptBytes: attempt.PromptBytes, State: attempt.State, ReasonCode: attempt.ReasonCode,
		StatusCode: attempt.StatusCode, OutputBytes: attempt.OutputBytes, InputTokens: attempt.InputTokens,
		OutputTokens: attempt.OutputTokens, StartedAtMS: attempt.StartedAtMS, FinishedAtMS: attempt.FinishedAtMS}
}

// DonationReviewActionView is the allowlisted ledger projection of one intent.
type DonationReviewActionView struct {
	ActionID             string `json:"action_id"`
	BatchID              string `json:"batch_id"`
	ItemID               string `json:"item_id"`
	ActorID              int    `json:"actor_id"`
	Kind                 string `json:"kind"`
	ExpectedItemRevision int64  `json:"expected_item_revision"`
	ReviewTargetRevision string `json:"review_target_revision"`
	Status               string `json:"status"`
	ReasonCode           string `json:"reason_code"`
	EffectRevision       int64  `json:"effect_revision"`
	Note                 string `json:"note,omitempty"`
	AppliedAtMS          *int64 `json:"applied_at_ms"`
	CreatedAtMS          int64  `json:"created_at_ms"`
}

func DonationReviewActionViewOf(action model.DonationReviewAction) DonationReviewActionView {
	return DonationReviewActionView{ActionID: action.ActionID, BatchID: action.BatchID, ItemID: action.ItemID,
		ActorID: action.ActorID, Kind: action.Kind, ExpectedItemRevision: action.ExpectedItemRevision,
		ReviewTargetRevision: action.ReviewTargetRevision, Status: action.Status, ReasonCode: action.ReasonCode,
		EffectRevision: action.EffectRevision, Note: action.Note, AppliedAtMS: action.AppliedAtMS, CreatedAtMS: action.CreatedAtMS}
}

// DonationRecordView keeps administrator history behind the same field
// allowlists as individual action/test lookups. No chat content is projected.
type DonationRecordView struct {
	model.DonationRecord
	PendingReviewAction *DonationReviewActionView  `json:"pending_review_action"`
	LatestTest          *DonationTestAttemptView   `json:"latest_test"`
	RecentReviewActions []DonationReviewActionView `json:"recent_review_actions"`
	RecentTests         []DonationTestAttemptView  `json:"recent_tests"`
}

func DonationRecordViewOf(record model.DonationRecord) DonationRecordView {
	view := DonationRecordView{DonationRecord: record, RecentReviewActions: make([]DonationReviewActionView, 0, len(record.RecentReviewActions)),
		RecentTests: make([]DonationTestAttemptView, 0, len(record.RecentTests))}
	if record.PendingReviewAction != nil {
		action := DonationReviewActionViewOf(*record.PendingReviewAction)
		view.PendingReviewAction = &action
	}
	if record.LatestTest != nil {
		attempt := DonationTestAttemptViewOf(*record.LatestTest)
		view.LatestTest = &attempt
	}
	for _, action := range record.RecentReviewActions {
		view.RecentReviewActions = append(view.RecentReviewActions, DonationReviewActionViewOf(action))
	}
	for _, attempt := range record.RecentTests {
		view.RecentTests = append(view.RecentTests, DonationTestAttemptViewOf(attempt))
	}
	return view
}

// ReviewContext reads the receiver's safe review context and records the
// observed revision and staging deadline. It never returns private execution
// configuration.
func (s *DonationService) ReviewContext(ctx context.Context, itemID string) (DonationReviewContextView, error) {
	item, batch, err := s.Store.Item(ctx, itemID)
	if err != nil {
		return DonationReviewContextView{}, err
	}
	client, _, caps, err := s.checkedClientCaps(ctx, batch.InstanceID, batch.SourceID)
	if err != nil {
		return DonationReviewContextView{}, err
	}
	defer client.close()
	if !supportsDonationManualReview(caps) {
		return DonationReviewContextView{}, &DonationRemoteError{Reason: "manual_review_unsupported"}
	}
	// A context contains only part of the item facts. Reconcile a complete
	// receipt before exposing its version for a new decision.
	receipt, err := client.batch(ctx, batch)
	if err != nil {
		return DonationReviewContextView{}, err
	}
	if err := s.Store.ApplyReceipt(ctx, receipt); err != nil {
		return DonationReviewContextView{}, err
	}
	view, err := client.reviewContext(ctx, batch.ID, item.ID)
	if err != nil {
		return view, err
	}
	if view.GroupID != batch.GroupID {
		return view, model.ErrDonationReceipt
	}
	return view, nil
}

// Review persists the intent first and then reconciles it with the receiver
// using the same action UUID.
func (s *DonationService) Review(ctx context.Context, actorID int, itemID, actionID string, input DonationReviewInput) (model.DonationReviewAction, error) {
	intent, err := s.Store.PrepareReviewAction(ctx, actorID, itemID, actionID, input.Kind, input.ExpectedItemRevision, input.ReviewTargetRevision, input.Note)
	if err != nil {
		return intent.Action, err
	}
	if intent.Action.Status != model.DonationActionPending {
		return intent.Action, nil
	}
	client, _, err := s.checkedClient(ctx, intent.Batch.InstanceID, intent.Batch.SourceID)
	if err != nil {
		_ = s.Store.MarkBatchError(ctx, intent.Batch.ID, donationRemoteReason(err))
		return intent.Action, err
	}
	defer client.close()
	result, err := client.reviewAction(ctx, intent.Action.BatchID, intent.Action.ItemID, intent.Action.ActionID,
		intent.Action.Kind, intent.Action.ExpectedItemRevision, intent.Action.ReviewTargetRevision, intent.Action.Note, intent.Action.ActorID)
	if err != nil {
		// A timeout, a 404, and a plain conflict never prove the command was not
		// executed, so the intent stays pending for reconciliation.
		_ = s.Store.MarkBatchError(ctx, intent.Batch.ID, donationRemoteReason(err))
		return intent.Action, err
	}
	saved, err := s.Store.ApplyReviewOutcome(ctx, intent.Action, result.Outcome, result.ReasonCode, result.EffectRevision, result.ReviewTargetRevision, result.AppliedAtMS)
	if err != nil {
		return intent.Action, err
	}
	if saved.Status == model.DonationActionApplied {
		_ = s.ReconcileBatch(ctx, intent.Batch)
	}
	return saved, nil
}

// BeginTest validates the request and reserves the test ID before any upstream
// call. A replayed ID returns its persisted metadata and never calls again.
func (s *DonationService) BeginTest(ctx context.Context, actorID int, itemID, testID string, input DonationTestInput) (model.DonationTestAttempt, model.DonationBatch, bool, error) {
	if !model.ValidDonationID(testID) || !model.ValidDonationID(itemID) || !utf8.ValidString(input.Prompt) || !utf8.ValidString(input.SystemPrompt) {
		return model.DonationTestAttempt{}, model.DonationBatch{}, false, model.ErrDonationInput
	}
	if err := validateDonationTestInput(input, donationLocalReviewLimits()); err != nil {
		return model.DonationTestAttempt{}, model.DonationBatch{}, false, err
	}
	digest, err := s.Store.TestRequestDigest(input.Prompt, input.SystemPrompt, input.MaxOutputTokens)
	if err != nil {
		return model.DonationTestAttempt{}, model.DonationBatch{}, false, err
	}
	intent, err := s.Store.PrepareTestAttempt(ctx, actorID, itemID, testID, input.ExpectedItemRevision, input.ReviewTargetRevision,
		input.Model, input.Stream, len(input.Prompt)+len(input.SystemPrompt), digest)
	if err != nil {
		return intent.Attempt, intent.Batch, false, err
	}
	return intent.Attempt, intent.Batch, intent.Created, nil
}

// ResolveTestBudget revalidates the effective bounds against the authenticated
// receiver before the reservation is spent.
func (s *DonationService) ResolveTestBudget(ctx context.Context, attempt model.DonationTestAttempt, batch model.DonationBatch, input DonationTestInput) (context.Context, context.CancelFunc, DonationReviewLimits, error) {
	limits, _, err := s.testLimits(ctx, batch)
	if err != nil {
		s.finishTestInterrupted(ctx, attempt.ID, "connection_unavailable")
		return ctx, func() {}, DonationReviewLimits{}, err
	}
	if err := validateDonationTestInput(input, limits); err != nil {
		s.finishTestInterrupted(ctx, attempt.ID, "invalid_format")
		return ctx, func() {}, limits, err
	}
	bounded, cancel := context.WithTimeout(ctx, time.Duration(limits.TotalTimeoutSeconds)*time.Second)
	return bounded, cancel, limits, nil
}

// RunTest performs one controlled, non-streaming chat test against the sole
// staging key of this donation item.
func (s *DonationService) RunTest(ctx context.Context, attempt model.DonationTestAttempt, batch model.DonationBatch, input DonationTestInput) (DonationTestResult, error) {
	ctx, cancel, limits, err := s.ResolveTestBudget(ctx, attempt, batch, input)
	if err != nil {
		return DonationTestResult{}, err
	}
	defer cancel()
	client, _, err := s.checkedClient(ctx, batch.InstanceID, batch.SourceID)
	if err != nil {
		s.finishTestInterrupted(ctx, attempt.ID, donationRemoteReason(err))
		return DonationTestResult{}, err
	}
	defer client.close()
	result, err := client.test(ctx, batch.ID, attempt.ItemID, attempt.TestID, donationTestRequestFor(attempt, input), limits)
	if err != nil {
		state, reason := donationTestTransportOutcome(err)
		return DonationTestResult{}, errors.Join(err, s.finishTest(ctx, attempt.ID, state, reason, 0, 0, 0, 0, 0))
	}
	text := newDonationRedactor(client.token).sanitize(result.Text)
	result.recordUsage()
	if result.State != model.DonationTestRunning {
		if err := s.finishTest(ctx, attempt.ID, result.State, result.ReasonCode, result.StatusCode, result.OutputBytes, result.InputTokens, result.OutputTokens, result.FinishedAtMS); err != nil {
			return DonationTestResult{}, err
		}
	}
	if result.State != model.DonationTestSucceeded {
		text = ""
	}
	result.Text = text
	return result, nil
}

// StreamTest forwards the receiver's controlled meta/delta/done protocol. The
// caller owns the SSE response writer.
func (s *DonationService) StreamTest(ctx context.Context, attempt model.DonationTestAttempt, batch model.DonationBatch, input DonationTestInput, writer http.ResponseWriter, flush func()) error {
	ctx, cancel, limits, err := s.ResolveTestBudget(ctx, attempt, batch, input)
	if err != nil {
		return err
	}
	defer cancel()
	client, _, err := s.checkedClient(ctx, batch.InstanceID, batch.SourceID)
	if err != nil {
		s.finishTestInterrupted(ctx, attempt.ID, donationRemoteReason(err))
		return err
	}
	defer client.close()
	request := donationTestRequestFor(attempt, input)
	request.Stream = true
	response, err := client.testStreamRequest(ctx, batch.ID, attempt.ItemID, attempt.TestID, request, limits)
	if err != nil {
		state, reason := donationTestTransportOutcome(err)
		return errors.Join(err, s.finishTest(ctx, attempt.ID, state, reason, 0, 0, 0, 0, 0))
	}
	defer response.Body.Close()
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType == "application/json" {
		// A receiver replay returns durable metadata, never the lost body and
		// never another upstream request, even when the original call streamed.
		result, err := readDonationTestResult(response.Body, limits)
		if err == nil {
			err = validateDonationTestResult(result, batch.ID, attempt.ItemID, request, limits)
		}
		if err != nil || result.Text != "" {
			return errors.Join(model.ErrDonationReceipt, err, s.finishTest(ctx, attempt.ID, model.DonationTestInterrupted, "invalid_response", 0, 0, 0, 0, 0))
		}
		result.recordUsage()
		if result.State != model.DonationTestRunning {
			if err := s.finishTest(ctx, attempt.ID, result.State, result.ReasonCode, result.StatusCode, result.OutputBytes, result.InputTokens, result.OutputTokens, result.FinishedAtMS); err != nil {
				return err
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		_, err = writer.Write(mustDonationJSON(map[string]any{"success": true, "message": "", "data": result}))
		return err
	}
	if mediaType != "text/event-stream" {
		return errors.Join(model.ErrDonationReceipt, s.finishTest(ctx, attempt.ID, model.DonationTestInterrupted, "invalid_response", 0, 0, 0, 0, 0))
	}
	stream := &donationTestStream{writer: writer, flush: flush, redactor: newDonationRedactor(client.token), idle: time.Duration(limits.IdleTimeoutSeconds) * time.Second}
	finalState, reason, status, output := model.DonationTestInterrupted, "invalid_stream", 0, 0
	rawOutput, visibleOutput, startedAtMS, finishedAtMS := 0, false, int64(0), int64(0)
	err = readDonationTestStream(response.Body, limits, func(event string, data []byte) error {
		switch event {
		case "delta":
			var delta struct {
				Text string `json:"text"`
			}
			if common.Unmarshal(data, &delta) != nil || !utf8.ValidString(delta.Text) {
				return model.ErrDonationReceipt
			}
			rawOutput += len(delta.Text)
			visibleOutput = visibleOutput || strings.TrimSpace(delta.Text) != ""
			if rawOutput > limits.MaxResponseBytes {
				return model.ErrDonationReceipt
			}
			safe := stream.redactor.push(delta.Text)
			if safe == "" {
				return nil
			}
			output += len(safe)
			if output > limits.MaxResponseBytes {
				return model.ErrDonationReceipt
			}
			return stream.writeEvent("delta", mustDonationJSON(struct {
				Text string `json:"text"`
			}{safe}))
		case "done":
			var done donationTestDone
			if common.Unmarshal(data, &done) != nil || !donationTerminalTestState(done.State) || done.FinishedAtMS < startedAtMS ||
				done.OutputBytes < 0 || done.OutputBytes > limits.MaxResponseBytes || done.StatusCode < 0 || done.StatusCode > 599 ||
				(done.Usage != nil && (done.Usage.InputTokens < 0 || done.Usage.OutputTokens < 0)) ||
				model.DonationTestReason(done.ReasonCode) != done.ReasonCode ||
				(done.State == model.DonationTestSucceeded && (!visibleOutput || done.ReasonCode != "" || done.StatusCode < 200 || done.StatusCode >= 300 || done.OutputBytes != rawOutput)) {
				return model.ErrDonationReceipt
			}
			finalState, reason, status, finishedAtMS = done.State, done.ReasonCode, done.StatusCode, done.FinishedAtMS
			stream.usage = done.Usage
			// Any text still held back by the redactor is emitted under the same
			// field allowlist before the terminal event.
			if tail := stream.redactor.flush(); tail != "" {
				output += len(tail)
				if output > limits.MaxResponseBytes {
					return model.ErrDonationReceipt
				}
				if err := stream.writeEvent("delta", mustDonationJSON(struct {
					Text string `json:"text"`
				}{tail})); err != nil {
					return err
				}
			}
			return nil
		case "meta":
			if output != 0 || stream.started {
				return model.ErrDonationReceipt
			}
			var meta donationTestMeta
			if common.Unmarshal(data, &meta) != nil || meta.TestID != attempt.TestID || meta.BatchID != batch.ID ||
				meta.ItemID != attempt.ItemID || meta.Model != attempt.Model || meta.StartRevision != attempt.StartRevision ||
				meta.TargetRevision != attempt.ReviewTargetRevision || meta.StartedAtMS <= 0 {
				return model.ErrDonationReceipt
			}
			stream.started, startedAtMS = true, meta.StartedAtMS
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set("Cache-Control", "no-store")
			writer.Header().Set("X-Accel-Buffering", "no")
			return stream.writeEvent("meta", mustDonationJSON(meta))
		default:
			return model.ErrDonationReceipt
		}
	})
	if err != nil {
		finalState = model.DonationTestInterrupted
		if errors.Is(err, context.Canceled) {
			finalState = model.DonationTestCancelled
		} else if errors.Is(err, context.DeadlineExceeded) {
			finalState = model.DonationTestFailed
		}
		reason = donationStreamReason(err)
		finishedAtMS = time.Now().UnixMilli()
	}
	inputTokens, outputTokens := 0, 0
	if stream.usage != nil {
		inputTokens, outputTokens = stream.usage.InputTokens, stream.usage.OutputTokens
	}
	if finishErr := s.finishTest(ctx, attempt.ID, finalState, reason, status, output, inputTokens, outputTokens, finishedAtMS); finishErr != nil {
		finalState, reason = model.DonationTestInterrupted, "internal_error"
		err = errors.Join(err, finishErr)
	}
	if !stream.started {
		return err
	}
	// Success is emitted only after the complete protocol and durable local
	// metadata. A failed cleanup is never advertised as a successful test.
	writeErr := stream.writeEvent("done", mustDonationJSON(donationTestDone{State: finalState, ReasonCode: reason,
		StatusCode: status, OutputBytes: output, FinishedAtMS: finishedAtMS, Usage: stream.usage}))
	return errors.Join(err, writeErr)
}

func (s *DonationService) testLimits(ctx context.Context, batch model.DonationBatch) (DonationReviewLimits, DonationCapabilities, error) {
	client, _, caps, err := s.checkedClientCaps(ctx, batch.InstanceID, batch.SourceID)
	if err != nil {
		return DonationReviewLimits{}, caps, err
	}
	client.close()
	if !supportsDonationManualReview(caps) {
		return DonationReviewLimits{}, caps, &DonationRemoteError{Reason: "manual_review_unsupported"}
	}
	return donationReviewLimits(caps), caps, nil
}

// TestAttempt reads persisted metadata only; it never recovers the body or
// re-calls the upstream.
func (s *DonationService) TestAttempt(ctx context.Context, itemID, testID string) (DonationTestAttemptView, error) {
	attempt, err := s.Store.TestAttemptForItem(ctx, itemID, testID)
	if err != nil {
		return DonationTestAttemptView{}, err
	}
	if attempt.State == model.DonationTestRunning {
		_, batch, err := s.Store.Item(ctx, itemID)
		if err != nil {
			return DonationTestAttemptView{}, err
		}
		client, _, err := s.checkedClient(ctx, batch.InstanceID, batch.SourceID)
		if err != nil {
			return DonationTestAttemptView{}, err
		}
		defer client.close()
		result, err := client.testStatus(ctx, attempt)
		if err != nil {
			return DonationTestAttemptView{}, err
		}
		if donationTerminalTestState(result.State) {
			result.recordUsage()
			if err := s.finishTest(ctx, attempt.ID, result.State, result.ReasonCode, result.StatusCode, result.OutputBytes, result.InputTokens, result.OutputTokens, result.FinishedAtMS); err != nil {
				return DonationTestAttemptView{}, err
			}
			attempt, err = s.Store.TestAttemptForItem(ctx, itemID, testID)
			if err != nil {
				return DonationTestAttemptView{}, err
			}
		}
	}
	return DonationTestAttemptViewOf(attempt), nil
}

// ReviewActionView returns one persisted intent with its confirmed result.
func (s *DonationService) ReviewActionView(ctx context.Context, itemID, actionID string) (model.DonationReviewAction, error) {
	return s.Store.ReviewActionForItem(ctx, itemID, actionID)
}

func (s *DonationService) finishTest(ctx context.Context, id uint64, state, reason string, status, outputBytes, inputTokens, outputTokens int, finishedAtMS int64) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.Store.FinishTestAttempt(cleanup, id, state, reason, status, outputBytes, inputTokens, outputTokens, finishedAtMS)
}

func (s *DonationService) finishTestInterrupted(ctx context.Context, id uint64, reason string) {
	if err := s.finishTest(ctx, id, model.DonationTestInterrupted, reason, 0, 0, 0, 0, 0); err != nil {
		common.SysError("donation test metadata could not be persisted")
	}
}

func donationTestRequestFor(attempt model.DonationTestAttempt, input DonationTestInput) donationTestRequest {
	return donationTestRequest{TestID: attempt.TestID, Actor: "user:" + strconv.Itoa(attempt.ActorID), ExpectedItemRevision: attempt.StartRevision,
		ReviewTargetRevision: attempt.ReviewTargetRevision, Model: attempt.Model, Prompt: input.Prompt,
		SystemPrompt: input.SystemPrompt, MaxOutputTokens: input.MaxOutputTokens, Stream: input.Stream}
}

// validateDonationTestInput enforces the same bounds the receiver enforces,
// including the serialized request size.
func validateDonationTestInput(input DonationTestInput, limits DonationReviewLimits) error {
	if !utf8.ValidString(input.Model) || len(input.Model) == 0 || len(input.Model) > 128 || strings.TrimSpace(input.Prompt) == "" {
		return model.ErrDonationInput
	}
	if !utf8.ValidString(input.Prompt) || !utf8.ValidString(input.SystemPrompt) || len(input.Prompt)+len(input.SystemPrompt) > limits.MaxPromptBytes {
		return model.ErrDonationInput
	}
	if input.ExpectedItemRevision < 0 || !donationTargetRevisionValid(input.ReviewTargetRevision) || input.MaxOutputTokens < 1 || input.MaxOutputTokens > limits.MaxOutputTokens {
		return model.ErrDonationInput
	}
	request := donationTestRequest{TestID: "00000000-0000-4000-8000-000000000000", Actor: "user:9223372036854775807", ExpectedItemRevision: input.ExpectedItemRevision,
		ReviewTargetRevision: input.ReviewTargetRevision, Model: input.Model, Prompt: input.Prompt,
		SystemPrompt: input.SystemPrompt, MaxOutputTokens: input.MaxOutputTokens, Stream: input.Stream}
	encoded, err := common.Marshal(request)
	if err != nil || len(encoded) > limits.MaxRequestBytes {
		return model.ErrDonationInput
	}
	return nil
}

func donationTerminalTestState(state string) bool {
	switch state {
	case model.DonationTestSucceeded, model.DonationTestFailed, model.DonationTestCancelled, model.DonationTestInterrupted:
		return true
	default:
		return false
	}
}

// donationTestTransportOutcome separates "the receiver answered with a failure"
// from "we do not know whether the call ran".
func donationTestTransportOutcome(err error) (string, string) {
	if errors.Is(err, context.Canceled) {
		return model.DonationTestCancelled, "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return model.DonationTestFailed, "timeout"
	}
	var remote *DonationRemoteError
	if errors.As(err, &remote) {
		switch remote.Reason {
		case "connection_unavailable", "invalid_response", "test_unavailable", "item_not_found", "review_not_found":
			return model.DonationTestInterrupted, remote.Reason
		}
		return model.DonationTestFailed, remote.Reason
	}
	if errors.Is(err, model.ErrDonationInput) {
		return model.DonationTestFailed, "invalid_format"
	}
	return model.DonationTestInterrupted, "connection_unavailable"
}

func donationStreamReason(err error) string {
	var remote *DonationRemoteError
	switch {
	case errors.As(err, &remote):
		return remote.Reason
	case errors.Is(err, model.ErrDonationReceipt):
		return "invalid_stream"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	default:
		return "connection_unavailable"
	}
}

func mustDonationJSON(value any) []byte {
	encoded, err := common.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}

type donationTestStream struct {
	writer   http.ResponseWriter
	flush    func()
	redactor *donationRedactor
	usage    *donationTestUsage
	started  bool
	idle     time.Duration
}

func (s *donationTestStream) writeEvent(event string, data []byte) error {
	if len(data) > model.DonationMaxTestEventBytes {
		return model.ErrDonationReceipt
	}
	controller := http.NewResponseController(s.writer)
	if err := controller.SetWriteDeadline(time.Now().Add(s.idle)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if _, err := fmt.Fprintf(s.writer, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	if err := controller.Flush(); err != nil {
		if !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		if s.flush != nil {
			s.flush()
		}
	}
	return nil
}

// readDonationTestStream parses the controlled event protocol. Unknown fields,
// unknown event names, and oversized events are protocol violations.
func readDonationTestStream(body io.Reader, limits DonationReviewLimits, onEvent func(event string, data []byte) error) error {
	const maxLine = 1 << 16
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), maxLine)
	event, data := "", make([]byte, 0, 1024)
	seen := make(map[string]int)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if event == "" {
				continue
			}
			if len(data) > limits.MaxEventBytes {
				return model.ErrDonationReceipt
			}
			seen[event]++
			if (event != "delta" && seen[event] > 1) ||
				(event != "meta" && seen["meta"] != 1) ||
				(event != "meta" && event != "delta" && event != "done") {
				return model.ErrDonationReceipt
			}
			if err := onEvent(event, data); err != nil {
				return err
			}
			if event == "done" {
				return nil
			}
			event, data = "", data[:0]
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		switch name {
		case "event":
			event = value
		case "data":
			if event != "" && event != "meta" && event != "delta" && event != "done" {
				return model.ErrDonationReceipt
			}
			if len(data) > 0 {
				data = append(data, '\n')
			}
			if len(data)+len(value) > limits.MaxEventBytes {
				return model.ErrDonationReceipt
			}
			data = append(data, value...)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return model.ErrDonationReceipt
}

const donationRedactionHold = 512

// donationRedactionPatterns are the credential shapes a model may echo back.
// A complete secret is removed even when it arrives split across events.
var donationRedactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{16,}`),
	regexp.MustCompile(`AIza[A-Za-z0-9_\-]{20,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9\-]{10,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
}

// donationRedactor removes known secrets and credential-shaped text from an
// incremental stream. It holds back a bounded tail so a secret split across
// events is never emitted in parts.
type donationRedactor struct {
	secrets []string
	carry   string
}

func newDonationRedactor(secrets ...string) *donationRedactor {
	kept := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if len(secret) >= 8 {
			kept = append(kept, secret)
		}
	}
	return &donationRedactor{secrets: kept}
}

func (r *donationRedactor) push(text string) string {
	if text == "" {
		return ""
	}
	combined := r.carry + text
	hold := donationRedactionHold
	for _, secret := range r.secrets {
		hold = max(hold, len(secret)-1)
	}
	cutoff := max(0, len(combined)-hold)
	// Detect complete matches before splitting the buffer. If a match crosses
	// the cutoff, retain it intact so neither fragment can bypass replacement.
	for _, secret := range r.secrets {
		for offset := 0; offset < cutoff; {
			index := strings.Index(combined[offset:], secret)
			if index < 0 {
				break
			}
			index += offset
			if index < cutoff && index+len(secret) > cutoff {
				cutoff = index
			}
			offset = index + len(secret)
		}
	}
	for _, pattern := range donationRedactionPatterns {
		for _, match := range pattern.FindAllStringIndex(combined, -1) {
			if match[0] < cutoff && match[1] > cutoff {
				cutoff = match[0]
			}
		}
	}
	for cutoff > 0 && !utf8.RuneStart(combined[cutoff]) {
		cutoff--
	}
	r.carry = combined[cutoff:]
	return r.sanitize(combined[:cutoff])
}

func (r *donationRedactor) flush() string {
	emitted := r.sanitize(r.carry)
	r.carry = ""
	return emitted
}

func (r *donationRedactor) sanitize(text string) string {
	if text == "" {
		return ""
	}
	for _, secret := range r.secrets {
		text = strings.ReplaceAll(text, secret, "[redacted]")
	}
	for _, pattern := range donationRedactionPatterns {
		text = pattern.ReplaceAllString(text, "[redacted]")
	}
	return text
}
