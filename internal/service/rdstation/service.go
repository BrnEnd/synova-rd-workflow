package rdstation

import (
	"context"
	"fmt"
	"strings"
	"time"

	rdClient "synova-rd-workflow/internal/client/rdstation"
	"synova-rd-workflow/internal/domain"
)

// GetContactsParams maps intent parameters to the contacts query.
type GetContactsParams struct {
	Name  string
	Email string
	Phone string
}

// CreateContactParams holds required fields to create a contact.
type CreateContactParams struct {
	Name    string
	Email   string
	Phone   string
	Company string
}

// GetDealsParams maps intent parameters to the deals query.
type GetDealsParams struct {
	Name           string
	Stage          string
	Status         string // "open", "won", "lost"
	OwnerName      string
	AllowedOwnerID map[string]struct{}
}

// CreateDealParams holds required fields to create a deal.
type CreateDealParams struct {
	Name        string
	ContactName string
	Stage       string
	UserID      string
}

// UpdateDealParams holds fields to update a deal.
type UpdateDealParams struct {
	DealName       string
	Field          string
	Value          string
	AllowedOwnerID map[string]struct{}
}

// MultipleDealsError is returned when a search finds more than one deal with the same name.
type MultipleDealsError struct {
	Deals []domain.Deal
}

func (e *MultipleDealsError) Error() string {
	return fmt.Sprintf("multiple deals found: %d results", len(e.Deals))
}

// StageNotFoundError is returned when the target stage does not exist.
type StageNotFoundError struct {
	Available []string
}

func (e *StageNotFoundError) Error() string {
	return fmt.Sprintf("stage not found; available: %s", strings.Join(e.Available, ", "))
}

// DealNotFoundError is returned when no deal matches the given name.
type DealNotFoundError struct {
	Name string
}

func (e *DealNotFoundError) Error() string {
	return fmt.Sprintf("no deal found with name '%s'", e.Name)
}

// MissingDealNameError is returned when a deal name is required but not provided.
type MissingDealNameError struct{}

func (e *MissingDealNameError) Error() string {
	return "deal name is required"
}

// ContactNotFoundError is returned when no contact matches the given name.
type ContactNotFoundError struct {
	Name string
}

func (e *ContactNotFoundError) Error() string {
	return fmt.Sprintf("no contact found with name '%s'", e.Name)
}

// UpdateContactParams holds fields to update a contact.
type UpdateContactParams struct {
	ContactName string
	Field       string // "email", "phone", "name"
	Value       string
}

// Service orchestrates CRM operations via the RD Station client.
type Service struct {
	client *rdClient.Client
}

// New returns a new RD Station service.
func New(client *rdClient.Client) *Service {
	return &Service{client: client}
}

// GetContacts fetches contacts from RD Station.
func (s *Service) GetContacts(ctx context.Context, params GetContactsParams) ([]domain.Contact, error) {
	resp, err := s.client.GetContacts(ctx, rdClient.GetContactsParams{
		Name:  params.Name,
		Email: params.Email,
		Phone: params.Phone,
	})
	if err != nil {
		return nil, err
	}

	contacts := make([]domain.Contact, 0, len(resp))
	for _, c := range resp {
		contacts = append(contacts, domain.Contact{
			ID:    c.ID,
			Name:  c.Name,
			Email: c.Email,
			Phone: c.Phone,
		})
	}

	return contacts, nil
}

// CreateContact creates a new contact in RD Station.
func (s *Service) CreateContact(ctx context.Context, params CreateContactParams) (domain.Contact, error) {
	resp, err := s.client.CreateContact(ctx, rdClient.CreateContactParams{
		Name:  params.Name,
		Email: params.Email,
		Phone: params.Phone,
	})
	if err != nil {
		return domain.Contact{}, err
	}

	return domain.Contact{
		ID:    resp.ID,
		Name:  resp.Name,
		Email: resp.Email,
		Phone: resp.Phone,
	}, nil
}

// GetDeals fetches deals from RD Station.
func (s *Service) GetDeals(ctx context.Context, params GetDealsParams) ([]domain.Deal, error) {
	clientParams := rdClient.GetDealsParams{Name: params.Name}

	if params.Status == "won" {
		t := true
		clientParams.Win = &t
	} else if params.Status == "lost" {
		f := false
		clientParams.Win = &f
	}

	if params.Stage != "" {
		stageID, err := s.findStageID(ctx, params.Stage)
		if err != nil {
			return nil, err
		}
		clientParams.DealStageID = stageID
	}

	resp, err := s.client.GetDeals(ctx, clientParams)
	if err != nil {
		return nil, err
	}

	deals := filterDealsByOwner(mapDeals(resp), params.AllowedOwnerID)
	deals = filterDealsByOwnerName(deals, params.OwnerName)
	return deals, nil
}

// CreateDeal creates a new deal in RD Station.
func (s *Service) CreateDeal(ctx context.Context, params CreateDealParams) (domain.Deal, error) {
	clientParams := rdClient.CreateDealParams{
		Name:   params.Name,
		UserID: params.UserID,
	}

	if params.Stage != "" {
		stageID, err := s.findStageID(ctx, params.Stage)
		if err != nil {
			return domain.Deal{}, err
		}
		clientParams.DealStageID = stageID
	}

	if params.ContactName != "" {
		contacts, err := s.client.GetContacts(ctx, rdClient.GetContactsParams{Name: params.ContactName})
		if err != nil {
			return domain.Deal{}, fmt.Errorf("rdstation.CreateDeal search contact: %w", err)
		}
		if len(contacts) > 0 {
			clientParams.ContactIDs = []string{contacts[0].ID}
		}
	}

	resp, err := s.client.CreateDeal(ctx, clientParams)
	if err != nil {
		return domain.Deal{}, err
	}

	return domain.Deal{
		ID:   resp.ID,
		Name: resp.Name,
		Stage: domain.Stage{
			ID:   resp.DealStage.ID,
			Name: resp.DealStage.Name,
		},
		Owner: mapOwner(resp),
	}, nil
}

// UpdateDeal updates a field on a deal identified by name.
func (s *Service) UpdateDeal(ctx context.Context, params UpdateDealParams) (domain.Deal, error) {
	deal, err := s.findSingleDeal(ctx, params.DealName, params.AllowedOwnerID)
	if err != nil {
		return domain.Deal{}, err
	}

	updateParams := rdClient.UpdateDealParams{}
	switch strings.ToLower(params.Field) {
	case "name", "nome":
		updateParams.Name = params.Value
	case "stage", "estágio", "estagio":
		stageID, stageErr := s.findStageID(ctx, params.Value)
		if stageErr != nil {
			return domain.Deal{}, stageErr
		}
		updateParams.DealStageID = stageID
	default:
		return domain.Deal{}, fmt.Errorf("campo '%s' não suportado para atualização", params.Field)
	}

	resp, err := s.client.UpdateDeal(ctx, deal.ID, updateParams)
	if err != nil {
		return domain.Deal{}, err
	}

	return domain.Deal{
		ID:   resp.ID,
		Name: resp.Name,
		Stage: domain.Stage{
			ID:   resp.DealStage.ID,
			Name: resp.DealStage.Name,
		},
		Owner: mapOwner(resp),
	}, nil
}

// MoveDealStage moves a deal to a target stage identified by name.
func (s *Service) MoveDealStage(ctx context.Context, dealName, targetStageName string) (domain.Deal, error) {
	return s.MoveDealStageForOwners(ctx, dealName, targetStageName, nil)
}

func (s *Service) MoveDealStageForOwners(ctx context.Context, dealName, targetStageName string, allowedOwnerID map[string]struct{}) (domain.Deal, error) {
	deal, err := s.findSingleDeal(ctx, dealName, allowedOwnerID)
	if err != nil {
		return domain.Deal{}, err
	}

	stageID, err := s.findStageID(ctx, targetStageName)
	if err != nil {
		return domain.Deal{}, err
	}

	resp, err := s.client.UpdateDeal(ctx, deal.ID, rdClient.UpdateDealParams{DealStageID: stageID})
	if err != nil {
		return domain.Deal{}, err
	}

	return domain.Deal{
		ID:   resp.ID,
		Name: resp.Name,
		Stage: domain.Stage{
			ID:   resp.DealStage.ID,
			Name: resp.DealStage.Name,
		},
		Owner: mapOwner(resp),
	}, nil
}

// GetDeal retrieves a single deal by name (finds ID first, then fetches details).
func (s *Service) GetDeal(ctx context.Context, dealName string) (domain.Deal, error) {
	return s.GetDealForOwners(ctx, dealName, nil)
}

func (s *Service) GetDealForOwners(ctx context.Context, dealName string, allowedOwnerID map[string]struct{}) (domain.Deal, error) {
	deal, err := s.findSingleDeal(ctx, dealName, allowedOwnerID)
	if err != nil {
		return domain.Deal{}, err
	}

	return s.GetDealByID(ctx, deal.ID)
}

// GetDealByID retrieves a single deal by its RD Station ID.
func (s *Service) GetDealByID(ctx context.Context, dealID string) (domain.Deal, error) {
	resp, err := s.client.GetDealByID(ctx, dealID)
	if err != nil {
		return domain.Deal{}, err
	}

	contacts := make([]domain.Contact, 0, len(resp.Contacts))
	for _, c := range resp.Contacts {
		contacts = append(contacts, domain.Contact{ID: c.ID, Name: c.Name})
	}

	return domain.Deal{
		ID:   resp.ID,
		Name: resp.Name,
		Stage: domain.Stage{
			ID:   resp.DealStage.ID,
			Name: resp.DealStage.Name,
		},
		Owner:     mapOwner(resp),
		Contacts:  contacts,
		CreatedAt: parseRDTime(resp.CreatedAt),
		UpdatedAt: parseRDTime(resp.UpdatedAt),
	}, nil
}

// GetDealContacts retrieves all contacts linked to a deal identified by name.
func (s *Service) GetDealContacts(ctx context.Context, dealName string) ([]domain.Contact, error) {
	return s.GetDealContactsForOwners(ctx, dealName, nil)
}

func (s *Service) GetDealContactsForOwners(ctx context.Context, dealName string, allowedOwnerID map[string]struct{}) ([]domain.Contact, error) {
	deal, err := s.findSingleDeal(ctx, dealName, allowedOwnerID)
	if err != nil {
		return nil, err
	}

	return s.GetDealContactsByID(ctx, deal.ID)
}

// GetDealContactsByID retrieves all contacts linked to a deal by its ID.
func (s *Service) GetDealContactsByID(ctx context.Context, dealID string) ([]domain.Contact, error) {
	resp, err := s.client.GetDealContacts(ctx, dealID)
	if err != nil {
		return nil, err
	}

	contacts := make([]domain.Contact, 0, len(resp))
	for _, c := range resp {
		contacts = append(contacts, domain.Contact{ID: c.ID, Name: c.Name})
	}

	return contacts, nil
}

// UpdateContact finds a contact by name and updates a single field.
func (s *Service) UpdateContact(ctx context.Context, params UpdateContactParams) (domain.Contact, error) {
	if params.ContactName == "" {
		return domain.Contact{}, fmt.Errorf("contact name is required")
	}

	contacts, err := s.client.GetContacts(ctx, rdClient.GetContactsParams{Name: params.ContactName})
	if err != nil {
		return domain.Contact{}, err
	}
	if len(contacts) == 0 {
		return domain.Contact{}, &ContactNotFoundError{Name: params.ContactName}
	}

	contact := contacts[0]
	updateParams := rdClient.UpdateContactParams{}
	switch strings.ToLower(params.Field) {
	case "email", "e-mail":
		updateParams.Email = params.Value
	case "phone", "telefone", "fone":
		updateParams.Phone = params.Value
	case "name", "nome":
		updateParams.Name = params.Value
	default:
		return domain.Contact{}, fmt.Errorf("campo '%s' não suportado para atualização de contato", params.Field)
	}

	resp, err := s.client.UpdateContact(ctx, contact.ID, updateParams)
	if err != nil {
		return domain.Contact{}, err
	}

	return domain.Contact{
		ID:    resp.ID,
		Name:  resp.Name,
		Email: resp.Email,
		Phone: resp.Phone,
	}, nil
}

// AssociateContactToDeal links an existing contact to a deal by name.
func (s *Service) AssociateContactToDeal(ctx context.Context, dealName, contactName string) (domain.Deal, error) {
	deal, err := s.findSingleDeal(ctx, dealName, nil)
	if err != nil {
		return domain.Deal{}, err
	}

	contacts, err := s.client.GetContacts(ctx, rdClient.GetContactsParams{Name: contactName})
	if err != nil {
		return domain.Deal{}, err
	}
	if len(contacts) == 0 {
		return domain.Deal{}, &ContactNotFoundError{Name: contactName}
	}
	newContactID := contacts[0].ID

	// Preserve existing contacts and avoid duplicates.
	ids := make([]string, 0, len(deal.Contacts)+1)
	for _, c := range deal.Contacts {
		if c.ID == newContactID {
			return deal, nil // already associated
		}
		ids = append(ids, c.ID)
	}
	ids = append(ids, newContactID)

	resp, err := s.client.UpdateDeal(ctx, deal.ID, rdClient.UpdateDealParams{ContactIDs: ids})
	if err != nil {
		return domain.Deal{}, err
	}

	dealContacts := make([]domain.Contact, 0, len(resp.Contacts))
	for _, c := range resp.Contacts {
		dealContacts = append(dealContacts, domain.Contact{ID: c.ID, Name: c.Name})
	}

	return domain.Deal{
		ID:        resp.ID,
		Name:      resp.Name,
		Stage:     domain.Stage{ID: resp.DealStage.ID, Name: resp.DealStage.Name},
		Owner:     mapOwner(resp),
		Contacts:  dealContacts,
		CreatedAt: parseRDTime(resp.CreatedAt),
		UpdatedAt: parseRDTime(resp.UpdatedAt),
	}, nil
}

// --- helpers ---

func (s *Service) findSingleDeal(ctx context.Context, name string, allowedOwnerID map[string]struct{}) (domain.Deal, error) {
	if name == "" {
		return domain.Deal{}, &MissingDealNameError{}
	}

	resp, err := s.client.GetDeals(ctx, rdClient.GetDealsParams{Name: name})
	if err != nil {
		return domain.Deal{}, err
	}

	deals := filterDealsByOwner(mapDeals(resp), allowedOwnerID)

	switch len(deals) {
	case 0:
		return domain.Deal{}, &DealNotFoundError{Name: name}
	case 1:
		return deals[0], nil
	default:
		// Cap to 5 for display
		if len(deals) > 5 {
			deals = deals[:5]
		}
		return domain.Deal{}, &MultipleDealsError{Deals: deals}
	}
}

func (s *Service) findStageID(ctx context.Context, stageName string) (string, error) {
	stages, err := s.client.GetDealStages(ctx)
	if err != nil {
		return "", fmt.Errorf("rdstation.findStageID: %w", err)
	}

	lowerTarget := strings.ToLower(stageName)
	available := make([]string, 0, len(stages))
	var partialID, partialName string
	var partialCount int

	for _, st := range stages {
		available = append(available, st.Name)
		lowerStage := strings.ToLower(st.Name)

		// Exact match wins immediately.
		if lowerStage == lowerTarget {
			return st.ID, nil
		}

		// Track substring matches for fallback.
		if strings.Contains(lowerStage, lowerTarget) || strings.Contains(lowerTarget, lowerStage) {
			partialID = st.ID
			partialName = st.Name
			partialCount++
		}
	}

	// Single unambiguous partial match — use it.
	if partialCount == 1 {
		_ = partialName
		return partialID, nil
	}

	return "", &StageNotFoundError{Available: available}
}

func mapDeals(resp []rdClient.DealResponse) []domain.Deal {
	deals := make([]domain.Deal, 0, len(resp))
	for _, d := range resp {
		contacts := make([]domain.Contact, 0, len(d.Contacts))
		for _, c := range d.Contacts {
			contacts = append(contacts, domain.Contact{ID: c.ID, Name: c.Name})
		}
		deals = append(deals, domain.Deal{
			ID:        d.ID,
			Name:      d.Name,
			Stage:     domain.Stage{ID: d.DealStage.ID, Name: d.DealStage.Name},
			Owner:     mapOwner(d),
			Contacts:  contacts,
			CreatedAt: parseRDTime(d.CreatedAt),
			UpdatedAt: parseRDTime(d.UpdatedAt),
		})
	}
	return deals
}

func mapOwner(d rdClient.DealResponse) domain.DealOwner {
	for _, owner := range []rdClient.DealUserResponse{d.DealOwner, d.Owner, d.User} {
		if owner.ID != "" || owner.Email != "" || owner.Name != "" {
			return domain.DealOwner{ID: owner.ID, Name: owner.Name, Email: owner.Email}
		}
	}
	return domain.DealOwner{}
}

func filterDealsByOwner(deals []domain.Deal, allowedOwnerID map[string]struct{}) []domain.Deal {
	if len(allowedOwnerID) == 0 {
		return deals
	}
	out := make([]domain.Deal, 0, len(deals))
	for _, deal := range deals {
		if _, ok := allowedOwnerID[deal.Owner.ID]; ok {
			out = append(out, deal)
		}
	}
	return out
}

func filterDealsByOwnerName(deals []domain.Deal, ownerName string) []domain.Deal {
	ownerName = normalizeSearchText(ownerName)
	if ownerName == "" {
		return deals
	}
	out := make([]domain.Deal, 0, len(deals))
	for _, deal := range deals {
		name := normalizeSearchText(deal.Owner.Name)
		email := normalizeSearchText(deal.Owner.Email)
		if strings.Contains(name, ownerName) || strings.Contains(ownerName, name) || strings.Contains(email, ownerName) {
			out = append(out, deal)
		}
	}
	return out
}

func normalizeSearchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "õ", "o", "ô", "o",
		"ú", "u",
		"ç", "c",
	).Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func parseRDTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z07:00", "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// CreateDealActivityParams holds the parameters to create a deal annotation.
type CreateDealActivityParams struct {
	DealName       string
	UserID         string
	Text           string
	AllowedOwnerID map[string]struct{}
}

// GetDealActivities returns the manual annotations of a deal identified by name.
func (s *Service) GetDealActivities(ctx context.Context, dealName string, allowedOwnerID map[string]struct{}) ([]domain.Activity, error) {
	deal, err := s.findSingleDeal(ctx, dealName, allowedOwnerID)
	if err != nil {
		return nil, err
	}

	return s.GetDealActivitiesByID(ctx, deal.ID)
}

// GetDealActivitiesByID returns the manual annotations of a deal by its ID.
func (s *Service) GetDealActivitiesByID(ctx context.Context, dealID string) ([]domain.Activity, error) {
	resp, err := s.client.GetActivities(ctx, dealID)
	if err != nil {
		return nil, err
	}

	activities := make([]domain.Activity, len(resp))
	for i, a := range resp {
		activities[i] = domain.Activity{ID: a.ID, Text: a.Text, Date: a.Date}
	}
	return activities, nil
}

// CreateDealActivity registers a manual annotation in a deal identified by name.
func (s *Service) CreateDealActivity(ctx context.Context, params CreateDealActivityParams) (domain.Activity, error) {
	if params.UserID == "" {
		return domain.Activity{}, fmt.Errorf("seu perfil não possui RD Station ID configurado; contate o administrador")
	}

	deal, err := s.findSingleDeal(ctx, params.DealName, params.AllowedOwnerID)
	if err != nil {
		return domain.Activity{}, err
	}

	resp, err := s.client.CreateActivity(ctx, deal.ID, params.UserID, params.Text)
	if err != nil {
		return domain.Activity{}, err
	}

	return domain.Activity{ID: resp.ID, Text: resp.Text, Date: resp.Date}, nil
}
