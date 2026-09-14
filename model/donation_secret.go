package model

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DonationSecret struct {
	Slot     string `json:"-" gorm:"primaryKey;size:32"`
	Material string `json:"-" gorm:"type:text"`
	Checksum string `json:"-" gorm:"size:64"`
}

type DonationConnection struct {
	ID          int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	BaseURL     string `json:"base_url" gorm:"size:512"`
	TokenCipher string `json:"-" gorm:"type:text"`
	InstanceID  string `json:"instance_id" gorm:"size:36"`
	SourceID    string `json:"source_id" gorm:"size:36"`
	SecretID    string `json:"-" gorm:"size:64"`
	Version     int64  `json:"version"`
	UpdatedAtMS int64  `json:"updated_at_ms"`
}

type DonationStore struct {
	DB             *gorm.DB
	secretID       string
	fingerprintKey []byte
	tokenKey       []byte
}

func OpenDonationStore(db *gorm.DB) (*DonationStore, error) {
	// Debug SQL also must not contain the encryption key or encrypted token.
	quiet := db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	var saved DonationSecret
	err := quiet.Where("slot = ?", "v1").First(&saved).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		for _, table := range []any{&DonationConnection{}, &DonationCampaign{}, &DonationCampaignRevision{}, &DonationBatch{}, &DonationItem{}, &DonationResource{}, &DonationReward{}, &DonationRetry{}, &DonationEvent{}} {
			var count int64
			if err := quiet.Model(table).Count(&count).Error; err != nil || count != 0 {
				return nil, ErrDonationSecret
			}
		}
		material := make([]byte, 64)
		if _, err := rand.Read(material); err != nil {
			return nil, ErrDonationSecret
		}
		sum := sha256.Sum256(material)
		candidate := DonationSecret{Slot: "v1", Material: base64.StdEncoding.EncodeToString(material), Checksum: hex.EncodeToString(sum[:])}
		if err := quiet.Create(&candidate).Error; err != nil && !donationUniqueError(err) {
			return nil, ErrDonationSecret
		}
		// Read the actual winner, never infer first insert from RowsAffected.
		err = quiet.Where("slot = ?", "v1").First(&saved).Error
	}
	if err != nil {
		return nil, ErrDonationSecret
	}
	material, err := base64.StdEncoding.DecodeString(saved.Material)
	if err != nil || len(material) != 64 {
		return nil, ErrDonationSecret
	}
	sum := sha256.Sum256(material)
	if hex.EncodeToString(sum[:]) != saved.Checksum {
		return nil, ErrDonationSecret
	}
	for _, table := range []any{&DonationConnection{}, &DonationBatch{}, &DonationResource{}} {
		var count int64
		if err := quiet.Model(table).Where("secret_id <> ? OR secret_id IS NULL", saved.Checksum).Count(&count).Error; err != nil || count != 0 {
			return nil, ErrDonationSecret
		}
	}
	return &DonationStore{DB: db, secretID: saved.Checksum, fingerprintKey: material[:32], tokenKey: material[32:]}, nil
}

func (s *DonationStore) verifySecret(tx *gorm.DB) error {
	var secret DonationSecret
	err := tx.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Where("slot = ?", "v1").First(&secret).Error
	if err != nil || secret.Checksum != s.secretID {
		return ErrDonationSecret
	}
	material, err := base64.StdEncoding.DecodeString(secret.Material)
	if err != nil {
		return ErrDonationSecret
	}
	sum := sha256.Sum256(material)
	if len(material) != 64 || hex.EncodeToString(sum[:]) != s.secretID {
		return ErrDonationSecret
	}
	return nil
}

func (s *DonationStore) Fingerprint(key string) string {
	return "v1:" + common.GenerateHMACWithKey(s.fingerprintKey, "new-api/donation-resource/v1\x00"+key)
}

func (s *DonationStore) Connection(ctx context.Context) (DonationConnection, string, error) {
	quiet := s.DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	if err := s.verifySecret(quiet); err != nil {
		return DonationConnection{}, "", err
	}
	var saved DonationConnection
	err := quiet.First(&saved, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return saved, "", nil
	}
	if err != nil {
		return saved, "", ErrDonationUnavailable
	}
	if saved.SecretID != s.secretID {
		return saved, "", ErrDonationSecret
	}
	block, err := aes.NewCipher(s.tokenKey)
	if err != nil {
		return saved, "", ErrDonationSecret
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return saved, "", ErrDonationSecret
	}
	data, err := base64.StdEncoding.DecodeString(saved.TokenCipher)
	if err != nil || len(data) < gcm.NonceSize() {
		return saved, "", ErrDonationSecret
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], []byte("donation/v1/"+saved.InstanceID+"/"+saved.SourceID))
	if err != nil {
		return saved, "", ErrDonationSecret
	}
	return saved, string(plain), nil
}

func (s *DonationStore) SaveConnection(ctx context.Context, value DonationConnection, token string, expectedVersion int64) error {
	if !ValidDonationID(value.InstanceID) || !ValidDonationID(value.SourceID) {
		return ErrDonationInput
	}
	block, err := aes.NewCipher(s.tokenKey)
	if err != nil {
		return ErrDonationSecret
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ErrDonationSecret
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return ErrDonationSecret
	}
	value.TokenCipher = base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(token), []byte("donation/v1/"+value.InstanceID+"/"+value.SourceID)))
	value.ID, value.SecretID, value.Version, value.UpdatedAtMS = 1, s.secretID, expectedVersion+1, time.Now().UnixMilli()
	return s.transaction(ctx, func(tx *gorm.DB) error {
		tx = tx.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		if err := s.verifySecret(tx); err != nil {
			return err
		}
		var old DonationConnection
		err := lockForUpdate(tx).First(&old, 1).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if expectedVersion != 0 {
				return ErrDonationConflict
			}
			if err := tx.Create(&value).Error; donationUniqueError(err) {
				return errDonationRace
			} else {
				return err
			}
		}
		if err != nil {
			return ErrDonationUnavailable
		}
		if old.Version != expectedVersion || old.InstanceID != value.InstanceID || old.SourceID != value.SourceID {
			return ErrDonationConflict
		}
		return tx.Save(&value).Error
	})
}
