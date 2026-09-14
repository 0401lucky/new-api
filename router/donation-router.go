package router

import (
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
	admin.GET("/connection", middleware.RequirePermission(authz.DonationConfigRead), controller.GetDonationConnection)
	admin.PUT("/connection", middleware.RequirePermission(authz.DonationConfigWrite), controller.PutDonationConnection)
	admin.GET("/group-options", middleware.RequirePermission(authz.DonationConfigRead), controller.GetDonationGroupOptions)
	admin.GET("/campaigns", middleware.RequirePermission(authz.DonationConfigRead), controller.AdminGetDonationCampaigns)
	admin.POST("/campaigns", middleware.RequirePermission(authz.DonationConfigWrite), controller.SaveDonationCampaign)
	admin.PATCH("/campaigns/:id", middleware.RequirePermission(authz.DonationConfigWrite), controller.SaveDonationCampaign)
	admin.GET("/records", middleware.RequirePermission(authz.DonationRecordsRead), controller.GetDonationRecords)
	admin.GET("/records/:item_id", middleware.RequirePermission(authz.DonationRecordsRead), controller.GetDonationRecord)
}
