package intent_router

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"synova-rd-workflow/internal/domain"
	rdSvc "synova-rd-workflow/internal/service/rdstation"
)

var (
	ErrPermissionDenied         = errors.New("permission denied")
	ErrDeletionRequiresApproval = errors.New("deal deletion requires RD Station approval")
)

// Router maps intents to RD Station service calls.
type Router struct {
	rdstation *rdSvc.Service
}

type Actor struct {
	Role          string
	RDStationID   string
	TeamRDUserIDs []string
}

// New returns a new intent Router.
func New(rdstation *rdSvc.Service) *Router {
	return &Router{rdstation: rdstation}
}

// Route executes the appropriate CRM operation for the given intent
// and returns the result as a plain interface{} for NLP formatting.
func (r *Router) Route(ctx context.Context, intent domain.Intent) (interface{}, error) {
	return r.RouteForActor(ctx, intent, Actor{Role: "director"})
}

func (r *Router) RouteForActor(ctx context.Context, intent domain.Intent, actor Actor) (interface{}, error) {
	p := intent.Parameters
	owners := allowedOwnerIDs(actor)

	switch intent.Name {
	case domain.IntentGetContacts:
		return r.rdstation.GetContacts(ctx, rdSvc.GetContactsParams{
			Name:  p["name"],
			Email: p["email"],
			Phone: p["phone"],
		})

	case domain.IntentGetDeals:
		return r.rdstation.GetDeals(ctx, rdSvc.GetDealsParams{
			Name:           p["name"],
			Stage:          p["stage"],
			Status:         p["status"],
			OwnerName:      p["owner_name"],
			AllowedOwnerID: owners,
		})

	case domain.IntentGetDeal:
		return r.rdstation.GetDealForOwners(ctx, p["deal_name"], owners)

	case domain.IntentGetDealContacts:
		return r.rdstation.GetDealContactsForOwners(ctx, p["deal_name"], owners)

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
			OwnerName:   p["owner_name"],
			ProductName: p["product"],
			Notes:       p["notes"],
			UserID:      actor.RDStationID,
		})

	case domain.IntentUpdateDeal:
		return r.rdstation.UpdateDeal(ctx, rdSvc.UpdateDealParams{
			DealName:       p["deal_name"],
			Field:          p["field"],
			Value:          p["value"],
			AllowedOwnerID: owners,
		})

	case domain.IntentMoveDealStage:
		return r.rdstation.MoveDealStageForOwners(ctx, p["deal_name"], p["target_stage"], owners)

	case domain.IntentDeleteDeal:
		if normalizeRole(actor.Role) != "director" {
			return nil, ErrPermissionDenied
		}
		return nil, ErrDeletionRequiresApproval

	case domain.IntentUpdateContact:
		return r.rdstation.UpdateContact(ctx, rdSvc.UpdateContactParams{
			ContactName: p["contact_name"],
			Field:       p["field"],
			Value:       p["value"],
		})

	case domain.IntentAssociateContactToDeal:
		return r.rdstation.AssociateContactToDeal(ctx, p["deal_name"], p["contact_name"])

	case domain.IntentGetDealActivities:
		return r.rdstation.GetDealActivities(ctx, p["deal_name"], owners)

	case domain.IntentCreateDealActivity:
		return r.rdstation.CreateDealActivity(ctx, rdSvc.CreateDealActivityParams{
			DealName:       p["deal_name"],
			UserID:         actor.RDStationID,
			Text:           p["text"],
			AllowedOwnerID: owners,
		})

	case domain.IntentCreateScheduledTask:
		return r.rdstation.CreateScheduledTask(ctx, rdSvc.CreateScheduledTaskParams{
			DealName:       p["deal_name"],
			Subject:        p["subject"],
			Type:           p["type"],
			Date:           p["date"],
			Hour:           p["hour"],
			Notes:          p["notes"],
			UserID:         actor.RDStationID,
			OwnerName:      p["owner_name"],
			AllowedOwnerID: owners,
		})

	case domain.IntentUnknown:
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown intent: %s", intent.Name)
	}
}

// ResolveDealSelection executes the original intent against a specific deal chosen by the user.
func (r *Router) ResolveDealSelection(ctx context.Context, intent domain.Intent, deal domain.Deal, actor Actor) (interface{}, error) {
	switch intent.Name {
	case domain.IntentGetDeal:
		return r.rdstation.GetDealByID(ctx, deal.ID)

	case domain.IntentGetDealContacts:
		return r.rdstation.GetDealContactsByID(ctx, deal.ID)

	case domain.IntentGetDealActivities:
		return r.rdstation.GetDealActivitiesByID(ctx, deal.ID)

	case domain.IntentCreateScheduledTask:
		p := intent.Parameters
		return r.rdstation.CreateScheduledTaskForDeal(ctx, deal, rdSvc.CreateScheduledTaskParams{
			Subject:   p["subject"],
			Type:      p["type"],
			Date:      p["date"],
			Hour:      p["hour"],
			Notes:     p["notes"],
			UserID:    actor.RDStationID,
			OwnerName: p["owner_name"],
		})

	default:
		return r.rdstation.GetDealByID(ctx, deal.ID)
	}
}

func allowedOwnerIDs(actor Actor) map[string]struct{} {
	role := normalizeRole(actor.Role)
	if role == "director" || role == "supervisor" {
		return nil
	}
	ids := append([]string{actor.RDStationID}, actor.TeamRDUserIDs...)
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out[id] = struct{}{}
		}
	}
	if len(out) == 0 {
		return map[string]struct{}{"__no_owner_configured__": {}}
	}
	return out
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "director", "diretoria", "diretor", "the god", "god":
		return "director"
	case "supervisor":
		return "supervisor"
	default:
		return "seller"
	}
}
