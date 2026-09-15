package authz

const (
	ResourceDonationConfig  = "donation_config"
	ResourceDonationRecords = "donation_records"

	// Reviewing a decision and running a real call against a staging key are
	// separate write capabilities; record read access implies neither.
	ActionDonationReview = "review"
	ActionDonationTest   = "test"
)

var (
	DonationConfigRead    = Permission{Resource: ResourceDonationConfig, Action: ActionRead}
	DonationConfigWrite   = Permission{Resource: ResourceDonationConfig, Action: ActionWrite}
	DonationRecordsRead   = Permission{Resource: ResourceDonationRecords, Action: ActionRead}
	DonationRecordsReview = Permission{Resource: ResourceDonationRecords, Action: ActionDonationReview}
	DonationRecordsTest   = Permission{Resource: ResourceDonationRecords, Action: ActionDonationTest}
)

func init() {
	RegisterResource(ResourceDefinition{Resource: ResourceDonationConfig, LabelKey: "Donation Settings", Actions: []ActionDefinition{
		{Action: ActionRead, LabelKey: "View donation settings", DescriptionKey: "View campaigns and target groups without secrets.", DefaultRoles: []string{BuiltInRoleAdmin}},
		{Action: ActionWrite, LabelKey: "Manage donation settings", DescriptionKey: "Configure the donation connection, campaigns, and permanent rewards.", DefaultRoles: []string{BuiltInRoleAdmin}},
	}})
	RegisterResource(ResourceDefinition{Resource: ResourceDonationRecords, LabelKey: "Donation Records", Actions: []ActionDefinition{
		{Action: ActionRead, LabelKey: "View all donation records", DescriptionKey: "View donation ownership, masked keys, and reward records.", DefaultRoles: []string{BuiltInRoleAdmin}},
		{Action: ActionDonationReview, LabelKey: "Review donation records", DescriptionKey: "Enter a record into manual review and approve or reject it.", DefaultRoles: []string{BuiltInRoleAdmin}},
		{Action: ActionDonationTest, LabelKey: "Run donation tests", DescriptionKey: "Run one controlled model call with the staging key of a pending donation.", DefaultRoles: []string{BuiltInRoleAdmin}},
	}})
}
