package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func setDonationRoutes(api *gin.RouterGroup) {
	base := api.Group("/donations", middleware.DisableCache())
	self := base.Group("", middleware.UserAuth())
	self.GET("/campaigns", controller.GetDonationCampaigns)
	self.POST("/batches", controller.SubmitDonationBatch)
	self.GET("/batches", controller.GetDonationBatches)
	self.GET("/batches/:id", controller.GetDonationBatch)
	self.POST("/batches/:id/retry", controller.RetryDonationBatch)
	admin := base.Group("/admin", middleware.AdminAuth())
	handlePermissionRoute(admin, http.MethodGet, "/connection", authz.DonationConfigRead, controller.GetDonationConnection)
	handlePermissionRoute(admin, http.MethodPut, "/connection", authz.DonationConfigWrite, controller.PutDonationConnection)
	handlePermissionRoute(admin, http.MethodGet, "/group-options", authz.DonationConfigRead, controller.GetDonationGroupOptions)
	handlePermissionRoute(admin, http.MethodGet, "/campaigns", authz.DonationConfigRead, controller.AdminGetDonationCampaigns)
	handlePermissionRoute(admin, http.MethodPost, "/campaigns", authz.DonationConfigWrite, controller.SaveDonationCampaign)
	handlePermissionRoute(admin, http.MethodPatch, "/campaigns/:id", authz.DonationConfigWrite, controller.SaveDonationCampaign)
	handlePermissionRoute(admin, http.MethodGet, "/records", authz.DonationRecordsRead, controller.GetDonationRecords)
	handlePermissionRoute(admin, http.MethodGet, "/records/:item_id", authz.DonationRecordsRead, controller.GetDonationRecord)
	handlePermissionRoute(admin, http.MethodGet, "/records/:item_id/review-context", authz.DonationRecordsRead, controller.GetDonationReviewContext)
	handlePermissionRoute(admin, http.MethodPost, "/records/:item_id/review-actions", authz.DonationRecordsReview, middleware.RequirePermission(authz.DonationRecordsRead), controller.PostDonationReviewAction)
	handlePermissionRoute(admin, http.MethodGet, "/records/:item_id/review-actions/:action_id", authz.DonationRecordsRead, controller.GetDonationReviewAction)
	handlePermissionRoute(admin, http.MethodPost, "/records/:item_id/tests", authz.DonationRecordsTest, middleware.RequirePermission(authz.DonationRecordsRead), controller.PostDonationTest)
	handlePermissionRoute(admin, http.MethodGet, "/records/:item_id/tests/:test_id", authz.DonationRecordsRead, controller.GetDonationTest)
}
