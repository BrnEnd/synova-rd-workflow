package domain

// IntentName represents a recognized CRM intent.
type IntentName string

const (
	IntentGetContacts            IntentName = "get_contacts"
	IntentGetDeals               IntentName = "get_deals"
	IntentGetDeal                IntentName = "get_deal"
	IntentGetDealContacts        IntentName = "get_deal_contacts"
	IntentCreateContact          IntentName = "create_contact"
	IntentCreateDeal             IntentName = "create_deal"
	IntentUpdateDeal             IntentName = "update_deal"
	IntentMoveDealStage          IntentName = "move_deal_stage"
	IntentDeleteDeal             IntentName = "delete_deal"
	IntentUpdateContact          IntentName = "update_contact"
	IntentAssociateContactToDeal IntentName = "associate_contact_to_deal"
	IntentGetDealActivities      IntentName = "get_deal_activities"
	IntentCreateDealActivity     IntentName = "create_deal_activity"
	IntentUnknown                IntentName = "unknown"
)

// Intent holds the structured output from the NLP layer.
type Intent struct {
	Name       IntentName
	Parameters map[string]string
	RawText    string
}
