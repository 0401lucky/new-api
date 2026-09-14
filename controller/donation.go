package controller

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func donationError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "DONATION_ERROR", "Unable to process the donation request."
	var remote *service.DonationRemoteError
	switch {
	case errors.Is(err, model.ErrDonationInput):
		status, code, message = http.StatusBadRequest, "DONATION_INVALID_REQUEST", "Invalid donation request."
	case errors.Is(err, gorm.ErrRecordNotFound):
		status, code, message = http.StatusNotFound, "DONATION_NOT_FOUND", "Donation record not found."
	case errors.Is(err, model.ErrDonationConflict):
		status, code, message = http.StatusConflict, "DONATION_CONFLICT", "The request conflicts with its saved identity."
	case errors.Is(err, model.ErrDonationAccount):
		status, code, message = http.StatusForbidden, "DONATION_ACCOUNT_DISABLED", "This account cannot receive donation rewards."
	case errors.Is(err, model.ErrDonationUnavailable):
		status, code, message = http.StatusConflict, "DONATION_UNAVAILABLE", "The donation campaign or confirmed receipt is unavailable."
	case errors.Is(err, model.ErrDonationSecret):
		status, code, message = http.StatusServiceUnavailable, "DONATION_IDENTITY_UNAVAILABLE", "Donation identity material needs administrator attention."
	case errors.Is(err, model.ErrDonationReceipt):
		status, code, message = http.StatusBadGateway, "DONATION_INVALID_RECEIPT", "The integration receipt could not be verified."
	case errors.As(err, &remote):
		status, code, message = http.StatusBadGateway, "DONATION_"+strings.ToUpper(remote.Reason), "The donation integration is unavailable."
	}
	c.JSON(status, gin.H{"success": false, "code": code, "message": message})
}

func donationService(c *gin.Context) *service.DonationService {
	svc, err := service.NewDonationService()
	if err != nil {
		donationError(c, err)
		return nil
	}
	return svc
}

// Explicit field allowlists make user-controlled recipient/target/quota fields
// an error. JSON is required; the existing Authorization-only auth handles CSRF.
func readDonationJSON(c *gin.Context, target any, fields ...string) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		donationError(c, model.ErrDonationInput)
		return false
	}
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, model.DonationMaxBodyBytes))
	if err != nil {
		donationError(c, model.ErrDonationInput)
		return false
	}
	var raw map[string]json.RawMessage
	if common.Unmarshal(data, &raw) != nil || raw == nil {
		donationError(c, model.ErrDonationInput)
		return false
	}
	allowed := make(map[string]bool, len(fields))
	for _, field := range fields {
		allowed[field] = true
	}
	for field, value := range raw {
		if !allowed[field] || string(value) == "null" {
			donationError(c, model.ErrDonationInput)
			return false
		}
	}
	if common.Unmarshal(data, target) != nil {
		donationError(c, model.ErrDonationInput)
		return false
	}
	return true
}

func donationPage(c *gin.Context) *common.PageInfo {
	page := common.GetPageQuery(c)
	page.Page, page.PageSize = min(1000000, max(1, page.Page)), min(100, max(1, page.PageSize))
	return page
}

func GetDonationCampaigns(c *gin.Context) {
	svc := donationService(c)
	if svc == nil {
		return
	}
	items, err := svc.Campaigns(c.Request.Context())
	if err != nil {
		donationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": items})
}

func SubmitDonationBatch(c *gin.Context) {
	var input struct {
		CampaignID int    `json:"campaign_id"`
		KeysText   string `json:"keys_text"`
	}
	if !readDonationJSON(c, &input, "campaign_id", "keys_text") {
		return
	}
	svc := donationService(c)
	if svc == nil {
		return
	}
	batch, err := svc.Submit(c.Request.Context(), c.GetInt("id"), input.CampaignID, c.GetHeader("Idempotency-Key"), input.KeysText)
	if err != nil {
		donationError(c, err)
		return
	}
	message := ""
	if batch.ReceptionState == "unconfirmed" {
		message = "Receipt is not confirmed. Keep the original keys and try this submission again."
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": message, "data": batch})
}

func GetDonationBatches(c *gin.Context) {
	for key := range c.Request.URL.Query() {
		if key != "p" && key != "page_size" && key != "ps" && key != "size" {
			donationError(c, model.ErrDonationInput)
			return
		}
	}
	svc := donationService(c)
	if svc == nil {
		return
	}
	page := donationPage(c)
	items, total, err := svc.Store.Batches(c.Request.Context(), c.GetInt("id"), page.GetStartIdx(), page.PageSize)
	if err != nil {
		donationError(c, err)
		return
	}
	page.SetTotal(int(total))
	page.SetItems(items)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": page})
}

func GetDonationBatch(c *gin.Context) {
	if !model.ValidDonationID(c.Param("id")) {
		donationError(c, model.ErrDonationInput)
		return
	}
	svc := donationService(c)
	if svc == nil {
		return
	}
	batch, err := svc.Store.Batch(c.Request.Context(), c.Param("id"), c.GetInt("id"))
	if err != nil {
		donationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": batch})
}

func RetryDonationBatch(c *gin.Context) {
	var input struct {
		ItemIDs []string `json:"item_ids"`
	}
	if !readDonationJSON(c, &input, "item_ids") {
		return
	}
	if !model.ValidDonationID(c.Param("id")) || !model.ValidDonationID(c.GetHeader("Idempotency-Key")) {
		donationError(c, model.ErrDonationInput)
		return
	}
	svc := donationService(c)
	if svc == nil {
		return
	}
	batch, err := svc.Retry(c.Request.Context(), c.GetInt("id"), c.Param("id"), c.GetHeader("Idempotency-Key"), input.ItemIDs)
	if err != nil {
		donationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": batch})
}

func GetDonationConnection(c *gin.Context) {
	svc := donationService(c)
	if svc == nil {
		return
	}
	connection, err := svc.Connection(c.Request.Context())
	if err != nil {
		donationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": connection})
}

func PutDonationConnection(c *gin.Context) {
	var input struct {
		BaseURL string  `json:"base_url"`
		Token   *string `json:"token"`
	}
	if !readDonationJSON(c, &input, "base_url", "token") {
		return
	}
	svc := donationService(c)
	if svc == nil {
		return
	}
	connection, err := svc.SaveConnection(c.Request.Context(), input.BaseURL, input.Token)
	if err != nil {
		donationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": connection})
}

func GetDonationGroupOptions(c *gin.Context) {
	svc := donationService(c)
	if svc == nil {
		return
	}
	groups, err := svc.Groups(c.Request.Context())
	if err != nil {
		donationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": groups})
}

func AdminGetDonationCampaigns(c *gin.Context) {
	svc := donationService(c)
	if svc == nil {
		return
	}
	items, err := svc.Store.Campaigns(c.Request.Context())
	if err != nil {
		donationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": items})
}

type donationCampaignInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	GroupID     *uint64 `json:"group_id"`
	RewardQuota *int    `json:"reward_quota"`
	Enabled     *bool   `json:"enabled"`
}

func SaveDonationCampaign(c *gin.Context) {
	var input donationCampaignInput
	if !readDonationJSON(c, &input, "name", "description", "group_id", "reward_quota", "enabled") {
		return
	}
	svc := donationService(c)
	if svc == nil {
		return
	}
	value := model.DonationCampaign{}
	var previous *model.DonationCampaign
	if c.Request.Method == http.MethodPatch {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil || id <= 0 {
			donationError(c, model.ErrDonationInput)
			return
		}
		old, err := svc.Store.Campaign(c.Request.Context(), id)
		if err != nil {
			donationError(c, err)
			return
		}
		previous, value = &old, old
	} else if input.Name == nil || input.GroupID == nil || input.RewardQuota == nil {
		donationError(c, model.ErrDonationInput)
		return
	}
	if input.Name != nil {
		value.Name = *input.Name
	}
	if input.Description != nil {
		value.Description = *input.Description
	}
	if input.GroupID != nil {
		value.GroupID = *input.GroupID
	}
	if input.RewardQuota != nil {
		value.RewardQuota = *input.RewardQuota
	}
	if input.Enabled != nil {
		value.Enabled = *input.Enabled
	}
	saved, err := svc.SaveCampaign(c.Request.Context(), value, previous)
	if err != nil {
		donationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": saved})
}

func GetDonationRecords(c *gin.Context) {
	var query struct {
		UserID       int    `form:"user_id"`
		CampaignID   int    `form:"campaign_id"`
		GroupID      uint64 `form:"group_id"`
		State        string `form:"state"`
		RewardState  string `form:"reward_state"`
		FromMS       int64  `form:"from_at_ms"`
		ToMS         int64  `form:"to_at_ms"`
		ItemID       string `form:"item_id"`
		CredentialID uint64 `form:"credential_id"`
	}
	if c.ShouldBindQuery(&query) != nil || query.UserID < 0 || query.CampaignID < 0 || query.FromMS < 0 || query.ToMS < 0 || (query.ToMS > 0 && query.ToMS < query.FromMS) || len(query.State) > 24 || len(query.RewardState) > 24 || (query.ItemID != "" && !model.ValidDonationID(query.ItemID)) {
		donationError(c, model.ErrDonationInput)
		return
	}
	svc := donationService(c)
	if svc == nil {
		return
	}
	page := donationPage(c)
	items, total, err := svc.Store.Records(c.Request.Context(), model.DonationRecordFilter{UserID: query.UserID, CampaignID: query.CampaignID, GroupID: query.GroupID, State: query.State, RewardState: query.RewardState, FromMS: query.FromMS, ToMS: query.ToMS, ItemID: query.ItemID, CredentialID: query.CredentialID}, page.GetStartIdx(), page.PageSize)
	if err != nil {
		donationError(c, err)
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": page})
}

func GetDonationRecord(c *gin.Context) {
	if !model.ValidDonationID(c.Param("item_id")) {
		donationError(c, model.ErrDonationInput)
		return
	}
	svc := donationService(c)
	if svc == nil {
		return
	}
	record, err := svc.Store.Record(c.Request.Context(), c.Param("item_id"))
	if err != nil {
		donationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": record})
}
