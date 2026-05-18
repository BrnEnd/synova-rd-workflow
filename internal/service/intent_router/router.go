package intent_router

import (
	"context"
	"fmt"

	"synova-rd-workflow/internal/domain"
	rdSvc "synova-rd-workflow/internal/service/rdstation"
)

// Router maps intents to RD Station service calls.
type Router struct {
	rdstation *rdSvc.Service
}

// New returns a new intent Router.
func New(rdstation *rdSvc.Service) *Router {
	return &Router{rdstation: rdstation}
}

// Route executes the appropriate CRM operation for the given intent
// and returns the result as a plain interface{} for NLP formatting.
func (r *Router) Route(ctx context.Context, intent domain.Intent) (interface{}, error) {
	p := intent.Parameters

	switch intent.Name {
	case domain.IntentGetContacts:
		return r.rdstation.GetContacts(ctx, rdSvc.GetContactsParams{
			Name:  p["name"],
			Email: p["email"],
			Phone: p["phone"],
		})

	case domain.IntentGetDeals:
		return r.rdstation.GetDeals(ctx, rdSvc.GetDealsParams{
			Name:   p["name"],
			Stage:  p["stage"],
			Status: p["status"],
		})

	case domain.IntentGetDeal:
		return r.rdstation.GetDeal(ctx, p["deal_name"])

	case domain.IntentGetDealContacts:
		return r.rdstation.GetDealContacts(ctx, p["deal_name"])

	case domain.IntentCreateContact:
		return r.rdstation.CreateContact(ctx, rdSvc.CreateContactParams{
			Name:    p["name"],
			Email:   p["email"],
			Phone:   p["phone"],
			Company: p["company"],
		})

	case domain.IntentCreateDeal:
		return r.rdstation.CreateDeal(ctx, rdSvc.CreateDealParams{
			Name:        p["name"],
			ContactName: p["contact_name"],
			Stage:       p["stage"],
		})

	case domain.IntentUpdateDeal:
		return r.rdstation.UpdateDeal(ctx, rdSvc.UpdateDealParams{
			DealName: p["deal_name"],
			Field:    p["field"],
			Value:    p["value"],
		})

	case domain.IntentMoveDealStage:
		return r.rdstation.MoveDealStage(ctx, p["deal_name"], p["target_stage"])

	case domain.IntentUpdateContact:
		return r.rdstation.UpdateContact(ctx, rdSvc.UpdateContactParams{
			ContactName: p["contact_name"],
			Field:       p["field"],
			Value:       p["value"],
		})

	case domain.IntentAssociateContactToDeal:
		return r.rdstation.AssociateContactToDeal(ctx, p["deal_name"], p["contact_name"])

	case domain.IntentUnknown:
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown intent: %s", intent.Name)
	}
}

// ResolveDealSelection executes the original intent against a specific deal chosen by the user.
func (r *Router) ResolveDealSelection(ctx context.Context, intent domain.Intent, deal domain.Deal) (interface{}, error) {
	switch intent.Name {
	case domain.IntentGetDeal:
		return r.rdstation.GetDealByID(ctx, deal.ID)

	case domain.IntentGetDealContacts:
		return r.rdstation.GetDealContactsByID(ctx, deal.ID)

	default:
		return r.rdstation.GetDealByID(ctx, deal.ID)
	}
}
