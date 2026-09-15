package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	sqlitedriver "github.com/glebarez/go-sqlite"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const (
	DonationMaxItems     = 100
	DonationMaxKeyBytes  = 4096
	DonationMaxBodyBytes = 1 << 20

	// DonationModeAuto keeps the original automatic model probe. Manual review is
	// an explicit opt-in that is only accepted from a receiver announcing
	// DonationFeatureManualReview.
	DonationModeAuto            = "auto"
	DonationModeManualReview    = "manual_review"
	DonationFeatureManualReview = "manual_review_v1"

	// The remote staging retention is frozen at 7 days by the original protocol.
	DonationStagingRetentionSeconds = 604800

	// Review and test vocabularies are closed sets shared with the receiver.
	DonationActionEnterReview = "enter_review"
	DonationActionApprove     = "approve"
	DonationActionReject      = "reject"

	DonationActionPending  = "pending"
	DonationActionApplied  = "applied"
	DonationActionRejected = "rejected"

	// Item receipts carry decisions, distinct from review command kinds.
	DonationDecisionApproved = "approved"
	DonationDecisionRejected = "rejected"

	DonationTestRunning     = "running"
	DonationTestSucceeded   = "succeeded"
	DonationTestFailed      = "failed"
	DonationTestCancelled   = "cancelled"
	DonationTestInterrupted = "interrupted"

	// Review notes are bounded by the protocol and never leave the admin boundary
	// except as a donor-visible rejection reason.
	DonationMaxNoteBytes = 2048
	// A single controlled chat test is bounded end to end on both sides.
	DonationMaxPromptBytes       = 16 << 10
	DonationMaxTestRequestBytes  = 64 << 10
	DonationDefaultOutputTokens  = 1024
	DonationMaxOutputTokens      = 4096
	DonationMaxTestResponseBytes = 128 << 10
	DonationMaxTestEventBytes    = 64 << 10
	DonationTestTotalTimeout     = 120
)

// NormalizeDonationValidationMode canonicalizes the stored campaign/batch mode.
// An empty value is the released representation of auto.
func NormalizeDonationValidationMode(value string) (string, bool) {
	switch value {
	case "", DonationModeAuto:
		return DonationModeAuto, true
	case DonationModeManualReview:
		return DonationModeManualReview, true
	default:
		return "", false
	}
}

var (
	ErrDonationInput       = errors.New("invalid donation request")
	ErrDonationConflict    = errors.New("donation request conflicts with its saved identity")
	ErrDonationUnavailable = errors.New("donation campaign or connection is unavailable")
	ErrDonationSecret      = errors.New("donation identity material is missing or invalid")
	ErrDonationReceipt     = errors.New("invalid donation receipt")
	ErrDonationAccount     = errors.New("donation account is disabled")
	ErrDonationReview      = errors.New("donation review action is not applicable")
	ErrDonationTestBusy    = errors.New("a donation test is already running for this item")
	errDonationRace        = errors.New("retry donation transaction")
)

// Donation records are main-database business records. They have no cascading
// associations with users, remote credentials, campaigns, or the log database.
type DonationCampaign struct {
	ID             int    `json:"id" gorm:"primaryKey"`
	Version        int    `json:"version"`
	Name           string `json:"name" gorm:"size:120"`
	Description    string `json:"description" gorm:"type:text"`
	InstanceID     string `json:"instance_id" gorm:"size:36"`
	SourceID       string `json:"-" gorm:"size:36"`
	GroupID        uint64 `json:"group_id"`
	GroupName      string `json:"group_name" gorm:"size:255"`
	TargetRevision string `json:"target_revision" gorm:"size:64"`
	// ValidationMode is "auto" for every campaign created before manual review
	// existed; the migration backfills the released NULL/empty representation.
	ValidationMode string `json:"validation_mode" gorm:"size:24"`
	RewardQuota    int    `json:"reward_quota" gorm:"type:bigint"`
	Enabled        bool   `json:"enabled"`
	CreatedAtMS    int64  `json:"created_at_ms"`
	UpdatedAtMS    int64  `json:"updated_at_ms"`
}

type DonationCampaignRevision struct {
	ID          uint64 `json:"-" gorm:"primaryKey"`
	CampaignID  int    `json:"campaign_id" gorm:"uniqueIndex:idx_donation_campaign_version,priority:1"`
	Version     int    `json:"version" gorm:"uniqueIndex:idx_donation_campaign_version,priority:2"`
	Snapshot    string `json:"-" gorm:"type:text"`
	CreatedAtMS int64  `json:"created_at_ms"`
}

type DonationBatch struct {
	ID              string `json:"id" gorm:"primaryKey;size:36"`
	UserID          int    `json:"user_id" gorm:"index;uniqueIndex:idx_donation_user_request,priority:1"`
	Username        string `json:"username" gorm:"size:64"`
	LinuxDOID       string `json:"linux_do_id" gorm:"size:255"`
	RequestKey      string `json:"request_key" gorm:"size:36;uniqueIndex:idx_donation_user_request,priority:2"`
	RequestDigest   string `json:"-" gorm:"size:64"`
	SecretID        string `json:"-" gorm:"size:64"`
	CampaignID      int    `json:"campaign_id" gorm:"index"`
	CampaignVersion int    `json:"campaign_version"`
	CampaignName    string `json:"campaign_name" gorm:"size:120"`
	InstanceID      string `json:"instance_id" gorm:"size:36"`
	SourceID        string `json:"-" gorm:"size:36"`
	GroupID         uint64 `json:"group_id" gorm:"index"`
	GroupName       string `json:"group_name" gorm:"size:255"`
	TargetRevision  string `json:"-" gorm:"size:64"`
	RewardQuota     int    `json:"reward_quota" gorm:"type:bigint"`
	// ValidationMode is frozen at submission from the campaign snapshot and is
	// never rewritten by later campaign edits or by enter_review.
	ValidationMode string `json:"validation_mode" gorm:"size:24"`
	ReceptionState string `json:"reception_state" gorm:"size:24"`
	LastError      string `json:"last_error" gorm:"size:64"`
	CreatedAtMS    int64  `json:"created_at_ms" gorm:"index"`
	UpdatedAtMS    int64  `json:"updated_at_ms"`
	NeedsRecovery  bool   `json:"-" gorm:"index:idx_donation_recovery,priority:1"`
	NextPollAtMS   int64  `json:"-" gorm:"index:idx_donation_recovery,priority:2"`
	// ColdPollAtMS schedules the low-rate, GET-only reconciliation used by
	// batches that only wait for a human decision.
	ColdPollAtMS int64  `json:"-"`
	LeaseUntilMS int64  `json:"-"`
	LeaseToken   string `json:"-" gorm:"size:36"`
	PollAttempts int    `json:"-"`
	SendAttempts int    `json:"-"`
}

type DonationItem struct {
	ID            string  `json:"id" gorm:"primaryKey;size:36"`
	BatchID       string  `json:"batch_id" gorm:"size:36;index;uniqueIndex:idx_donation_batch_line,priority:1"`
	Line          int     `json:"line" gorm:"uniqueIndex:idx_donation_batch_line,priority:2"`
	Fingerprint   string  `json:"-" gorm:"size:67;index"`
	Generation    int64   `json:"-"`
	KeyMask       string  `json:"key_mask" gorm:"size:32"`
	Dispatch      bool    `json:"-"`
	DuplicateOf   string  `json:"-" gorm:"size:36"`
	State         string  `json:"state" gorm:"size:24;index"`
	ReasonCode    string  `json:"reason_code" gorm:"size:64"`
	Retryable     bool    `json:"retryable"`
	CredentialID  *uint64 `json:"credential_id" gorm:"index"`
	AcceptedAtMS  *int64  `json:"accepted_at_ms"`
	RewardState   string  `json:"reward_state" gorm:"size:24;index"`
	RewardReason  string  `json:"reward_reason" gorm:"size:64"`
	RewardedQuota int     `json:"rewarded_quota" gorm:"type:bigint"`
	RewardedAtMS  *int64  `json:"rewarded_at_ms"`
	CreatedAtMS   int64   `json:"created_at_ms"`
	UpdatedAtMS   int64   `json:"updated_at_ms"`

	// Manual-review facts. They are independent from the original
	// DonationResource.Generation and from the automatic probe state.
	ItemRevision         int64  `json:"item_revision"`
	EffectiveMode        string `json:"effective_mode" gorm:"size:24"`
	ReviewTargetRevision string `json:"review_target_revision" gorm:"size:64"`
	EntryActionID        string `json:"entry_action_id" gorm:"size:36"`
	ReviewActionID       string `json:"review_action_id" gorm:"size:36"`
	ReviewState          string `json:"review_state" gorm:"size:24;index"`
	ReviewDecision       string `json:"-" gorm:"size:24"`
	ReviewedAtMS         *int64 `json:"reviewed_at_ms"`
	StagingExpiresAtMS   *int64 `json:"staging_expires_at_ms"`

	// ReviewNote is a projection only: it carries the donor-visible note of the
	// final applied reject action and is never stored on the item row.
	ReviewNote string `json:"review_note,omitempty" gorm:"-"`
}

// DonationReviewAction is the durable local ledger of a review intent. The
// action_id is the exact UUID sent to the receiver, so a lost response is
// reconciled by replaying the same identity instead of guessing.
type DonationReviewAction struct {
	ID                   uint64 `json:"-" gorm:"primaryKey"`
	ActorID              int    `json:"actor_id" gorm:"uniqueIndex:idx_donation_review_actor,priority:1"`
	ActionID             string `json:"action_id" gorm:"size:36;uniqueIndex;uniqueIndex:idx_donation_review_actor,priority:2"`
	BatchID              string `json:"batch_id" gorm:"size:36;index"`
	ItemID               string `json:"item_id" gorm:"size:36;index"`
	Kind                 string `json:"kind" gorm:"size:24"`
	ExpectedItemRevision int64  `json:"expected_item_revision"`
	ReviewTargetRevision string `json:"review_target_revision" gorm:"size:64"`
	Note                 string `json:"note" gorm:"type:text"`
	Status               string `json:"status" gorm:"size:24;index"`
	ReasonCode           string `json:"reason_code" gorm:"size:64"`
	EffectRevision       int64  `json:"effect_revision"`
	AppliedAtMS          *int64 `json:"applied_at_ms"`
	CreatedAtMS          int64  `json:"created_at_ms"`
	UpdatedAtMS          int64  `json:"updated_at_ms"`
}

// DonationTestAttempt stores only controlled metadata. The prompt, the full key
// and the model output are never persisted; PromptDigest is an HMAC summary.
type DonationTestAttempt struct {
	ID                   uint64 `json:"-" gorm:"primaryKey"`
	ActorID              int    `json:"-" gorm:"uniqueIndex:idx_donation_test_actor,priority:1"`
	TestID               string `json:"test_id" gorm:"size:36;uniqueIndex;uniqueIndex:idx_donation_test_actor,priority:2"`
	BatchID              string `json:"batch_id" gorm:"size:36;index"`
	ItemID               string `json:"item_id" gorm:"size:36;index"`
	StartRevision        int64  `json:"start_revision"`
	ReviewTargetRevision string `json:"target_revision" gorm:"size:64"`
	Model                string `json:"model" gorm:"size:128"`
	Stream               bool   `json:"stream"`
	PromptDigest         string `json:"-" gorm:"size:64"`
	PromptBytes          int    `json:"prompt_bytes"`
	State                string `json:"state" gorm:"size:24;index"`
	ReasonCode           string `json:"reason_code" gorm:"size:64"`
	StatusCode           int    `json:"status_code"`
	OutputBytes          int    `json:"output_bytes"`
	InputTokens          int    `json:"input_tokens"`
	OutputTokens         int    `json:"output_tokens"`
	StartedAtMS          int64  `json:"started_at_ms"`
	FinishedAtMS         *int64 `json:"finished_at_ms"`
}

type DonationResource struct {
	Fingerprint    string `json:"-" gorm:"primaryKey;size:67"`
	SecretID       string `json:"-" gorm:"size:64"`
	OwnerItemID    string `json:"-" gorm:"size:36"`
	Generation     int64  `json:"-"`
	Acquired       bool   `json:"-"`
	AcceptedItemID string `json:"-" gorm:"size:36"`
	UpdatedAtMS    int64  `json:"-"`
}

type DonationReward struct {
	ID           string `json:"id" gorm:"primaryKey;size:36"`
	Fingerprint  string `json:"-" gorm:"size:67;not null;uniqueIndex"`
	ItemID       string `json:"item_id" gorm:"size:36;not null;uniqueIndex"`
	UserID       int    `json:"user_id" gorm:"index"`
	Quota        int    `json:"quota" gorm:"type:bigint"`
	CreditedAtMS int64  `json:"credited_at_ms"`
}

type DonationRetry struct {
	ID          string `json:"-" gorm:"primaryKey;size:36"`
	UserID      int    `json:"-" gorm:"uniqueIndex:idx_donation_retry_request,priority:1"`
	RequestKey  string `json:"-" gorm:"size:36;uniqueIndex:idx_donation_retry_request,priority:2"`
	BatchID     string `json:"-" gorm:"size:36;index"`
	Digest      string `json:"-" gorm:"size:64"`
	ItemIDsJSON string `json:"-" gorm:"type:text"`
	Done        bool   `json:"-"`
	CreatedAtMS int64  `json:"-"`
}

type DonationEvent struct {
	ID          uint64 `json:"id" gorm:"primaryKey"`
	ItemID      string `json:"item_id" gorm:"size:36;index"`
	State       string `json:"state" gorm:"size:24"`
	ReasonCode  string `json:"reason_code" gorm:"size:64"`
	CreatedAtMS int64  `json:"created_at_ms"`
}

// MigrateDonations is an additive evolution of the released donation schema. It
// never rewrites the frozen campaign revision JSON, request digests, ownership
// rows, or the permanent-reward uniqueness constraints.
func MigrateDonations(db *gorm.DB) error {
	if err := db.AutoMigrate(&DonationSecret{}, &DonationConnection{}, &DonationCampaign{}, &DonationCampaignRevision{},
		&DonationBatch{}, &DonationItem{}, &DonationResource{}, &DonationReward{}, &DonationRetry{}, &DonationEvent{},
		&DonationReviewAction{}, &DonationTestAttempt{}); err != nil {
		return err
	}
	// AutoMigrate adds the new columns as NULL/empty on an existing database.
	// Every released row predates manual review, so the released representation
	// is canonicalized to auto instead of being left ambiguous.
	return db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []any{&DonationCampaign{}, &DonationBatch{}} {
			var unknown int64
			if err := tx.Model(table).Where("validation_mode IS NOT NULL AND validation_mode NOT IN ?", []string{"", DonationModeAuto, DonationModeManualReview}).Count(&unknown).Error; err != nil {
				return err
			}
			if unknown > 0 {
				return ErrDonationReceipt
			}
			if err := tx.Model(table).Where("validation_mode IS NULL OR validation_mode = ?", "").
				Update("validation_mode", DonationModeAuto).Error; err != nil {
				return err
			}
		}
		var invalid int64
		manualBatches := tx.Model(&DonationBatch{}).Select("id").Where("validation_mode = ?", DonationModeManualReview)
		if err := tx.Model(&DonationItem{}).Where("effective_mode IS NOT NULL AND effective_mode NOT IN ?", []string{"", DonationModeAuto, DonationModeManualReview}).Count(&invalid).Error; err != nil {
			return err
		}
		if invalid > 0 {
			return ErrDonationReceipt
		}
		if err := tx.Model(&DonationItem{}).Where("(effective_mode IS NULL OR effective_mode = ?) AND (batch_id IN (?) OR entry_action_id <> ? OR review_action_id <> ? OR review_decision <> ?)", "", manualBatches, "", "", "").Count(&invalid).Error; err != nil {
			return err
		}
		if invalid > 0 {
			return ErrDonationReceipt
		}
		return tx.Model(&DonationItem{}).Where("effective_mode IS NULL OR effective_mode = ?", "").
			Update("effective_mode", DonationModeAuto).Error
	})
}

func ValidDonationID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 4 && id.Variant() == uuid.RFC4122 && id.String() == value
}

func donationUniqueError(err error) bool {
	var my *mysql.MySQLError
	var pg *pgconn.PgError
	var sq *sqlitedriver.Error
	return errors.Is(err, gorm.ErrDuplicatedKey) ||
		(errors.As(err, &my) && my.Number == 1062) ||
		(errors.As(err, &pg) && pg.Code == "23505") ||
		(errors.As(err, &sq) && (sq.Code() == 1555 || sq.Code() == 2067))
}

// A uniqueness collision must leave the failed PostgreSQL transaction before
// reading the winner. MySQL clientFoundRows must never decide insertion ownership.
func (s *DonationStore) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	var err error
	for attempt := range 8 {
		err = s.DB.WithContext(ctx).Transaction(fn)
		if err == nil {
			return nil
		}
		var my *mysql.MySQLError
		var pg *pgconn.PgError
		var sq *sqlitedriver.Error
		retry := errors.Is(err, errDonationRace) ||
			(errors.As(err, &my) && (my.Number == 1213 || my.Number == 1205)) ||
			(errors.As(err, &pg) && (pg.Code == "40001" || pg.Code == "40P01")) ||
			(errors.As(err, &sq) && (sq.Code()&255 == 5 || sq.Code()&255 == 6))
		if !retry {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(5*(attempt+1)) * time.Millisecond):
		}
	}
	return err
}

func validDonationQuota(quota int) bool { return quota > 0 && quota <= common.MaxWalletQuota }

// donationStagingDeadline is the local view of the frozen 7-day staging window.
// The receiver remains authoritative and can only be observed through
// review-context; this value bounds approvals and tests when it is not reported.
func donationStagingDeadline(now int64) *int64 {
	deadline := now + DonationStagingRetentionSeconds*1000
	return &deadline
}
