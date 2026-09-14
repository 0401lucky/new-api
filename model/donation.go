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
)

var (
	ErrDonationInput       = errors.New("invalid donation request")
	ErrDonationConflict    = errors.New("donation request conflicts with its saved identity")
	ErrDonationUnavailable = errors.New("donation campaign or connection is unavailable")
	ErrDonationSecret      = errors.New("donation identity material is missing or invalid")
	ErrDonationReceipt     = errors.New("invalid donation receipt")
	ErrDonationAccount     = errors.New("donation account is disabled")
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
	ReceptionState  string `json:"reception_state" gorm:"size:24"`
	LastError       string `json:"last_error" gorm:"size:64"`
	CreatedAtMS     int64  `json:"created_at_ms" gorm:"index"`
	UpdatedAtMS     int64  `json:"updated_at_ms"`
	NeedsRecovery   bool   `json:"-" gorm:"index:idx_donation_recovery,priority:1"`
	NextPollAtMS    int64  `json:"-" gorm:"index:idx_donation_recovery,priority:2"`
	LeaseUntilMS    int64  `json:"-"`
	LeaseToken      string `json:"-" gorm:"size:36"`
	PollAttempts    int    `json:"-"`
	SendAttempts    int    `json:"-"`
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

func MigrateDonations(db *gorm.DB) error {
	return db.AutoMigrate(&DonationSecret{}, &DonationConnection{}, &DonationCampaign{}, &DonationCampaignRevision{},
		&DonationBatch{}, &DonationItem{}, &DonationResource{}, &DonationReward{}, &DonationRetry{}, &DonationEvent{})
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
