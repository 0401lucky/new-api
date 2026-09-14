package authz

const (
	ResourceDonationConfig  = "donation_config"
	ResourceDonationRecords = "donation_records"
)

var (
	DonationConfigRead  = Permission{Resource: ResourceDonationConfig, Action: ActionRead}
	DonationConfigWrite = Permission{Resource: ResourceDonationConfig, Action: ActionWrite}
	DonationRecordsRead = Permission{Resource: ResourceDonationRecords, Action: ActionRead}
)

func init() {
	RegisterResource(ResourceDefinition{Resource: ResourceDonationConfig, LabelKey: "Donation Settings", Actions: []ActionDefinition{
		{Action: ActionRead, LabelKey: "View donation settings", DescriptionKey: "View campaigns and target groups without secrets.", DefaultRoles: []string{BuiltInRoleAdmin}},
		{Action: ActionWrite, LabelKey: "Manage donation settings", DescriptionKey: "Configure the donation connection, campaigns, and permanent rewards.", DefaultRoles: []string{BuiltInRoleAdmin}},
	}})
	RegisterResource(ResourceDefinition{Resource: ResourceDonationRecords, LabelKey: "Donation Records", Actions: []ActionDefinition{
		{Action: ActionRead, LabelKey: "View all donation records", DescriptionKey: "View donation ownership, masked keys, and reward records.", DefaultRoles: []string{BuiltInRoleAdmin}},
	}})
}
