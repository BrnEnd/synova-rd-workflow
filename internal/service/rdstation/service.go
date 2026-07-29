package rdstation

import (
	"context"
	"fmt"
	"sort"
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
	CustomerName   string
	Stage          string
	Status         string // "open", "won", "lost"
	OwnerName      string
	UpdatedAfter   string // YYYY-MM-DD, inclusivo
	UpdatedBefore  string // YYYY-MM-DD, inclusivo
	AllowedOwnerID map[string]struct{}
}

// CreateDealParams holds required fields to create a deal.
type CreateDealParams struct {
	Name            string
	Company         string
	ContactName     string
	Pipeline        string
	Stage           string
	OwnerName       string
	ProductName     string
	Notes           string
	UserID          string
	FollowUpSubject string
	FollowUpType    string
	FollowUpDate    string
	FollowUpHour    string
	FollowUpNotes   string
	IdempotencyKey  string
}

// UpdateDealParams holds fields to update a deal.
type UpdateDealParams struct {
	DealName       string
	Field          string
	Value          string
	AllowedOwnerID map[string]struct{}
}

type CreateScheduledTaskParams struct {
	DealName       string
	Company        string
	ProductName    string
	Pipeline       string
	Stage          string
	Subject        string
	Type           string
	Date           string
	Hour           string
	Notes          string
	UserID         string
	OwnerName      string
	AllowedOwnerID map[string]struct{}
	IdempotencyKey string
}

type ResolveDealParams struct {
	DealName       string
	Company        string
	ProductName    string
	Stage          string
	Pipeline       string
	OwnerName      string
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

// AmbiguousStageError is returned when a stage name exists in multiple pipelines.
type AmbiguousStageError struct {
	Name   string
	Stages []domain.Stage
}

func (e *AmbiguousStageError) Error() string {
	return fmt.Sprintf("stage '%s' exists in multiple pipelines", e.Name)
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
	Name        string
	Suggestions []string
}

func (e *ContactNotFoundError) Error() string {
	return fmt.Sprintf("no contact found with name '%s'", e.Name)
}

type OwnerNotFoundError struct {
	Name        string
	Suggestions []string
}

func (e *OwnerNotFoundError) Error() string {
	return fmt.Sprintf("no owner found with name '%s'", e.Name)
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
	if strings.TrimSpace(params.CustomerName) != "" {
		clientParams.Name = ""
	}

	if params.Status == "won" {
		t := true
		clientParams.Win = &t
	} else if params.Status == "lost" {
		f := false
		clientParams.Win = &f
	}

	if params.Stage != "" {
		stageID, err := s.findStageIDInPipeline(ctx, params.Stage, "")
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
	if strings.TrimSpace(params.CustomerName) != "" {
		deals = filterDealsByCustomerName(deals, params.CustomerName)
	}
	deals = filterDealsByUpdatedRange(deals, params.UpdatedAfter, params.UpdatedBefore)
	sortDealsByUpdatedDesc(deals)
	if strings.TrimSpace(params.OwnerName) == "" {
		return deals, nil
	}
	filtered := filterDealsByOwnerName(deals, params.OwnerName)
	if len(filtered) == 0 && len(deals) > 0 {
		return nil, &OwnerNotFoundError{Name: params.OwnerName, Suggestions: closestOwnerNames(deals, params.OwnerName, 5)}
	}
	return filtered, nil
}

// CreateDeal creates a new deal in RD Station.
func (s *Service) CreateDeal(ctx context.Context, params CreateDealParams) (domain.DealCreationResult, error) {
	if existing, ok, err := s.findExistingDealForCreate(ctx, params); err != nil {
		return domain.DealCreationResult{}, err
	} else if ok {
		result := domain.DealCreationResult{Deal: existing, Idempotent: true}
		if strings.TrimSpace(params.FollowUpSubject) != "" {
			task, taskErr := s.CreateScheduledTaskForDeal(ctx, existing, CreateScheduledTaskParams{
				Subject:        params.FollowUpSubject,
				Type:           params.FollowUpType,
				Date:           params.FollowUpDate,
				Hour:           params.FollowUpHour,
				Notes:          params.FollowUpNotes,
				UserID:         params.UserID,
				OwnerName:      params.OwnerName,
				IdempotencyKey: params.IdempotencyKey,
			})
			if taskErr != nil {
				result.TaskError = taskErr.Error()
			} else {
				result.Task = task
			}
		}
		return result, nil
	}

	clientParams := rdClient.CreateDealParams{
		Name:   params.Name,
		UserID: params.UserID,
	}

	if params.Stage != "" {
		stageID, err := s.findStageIDInPipeline(ctx, params.Stage, params.Pipeline)
		if err != nil {
			return domain.DealCreationResult{}, err
		}
		clientParams.DealStageID = stageID
	}

	contactName := strings.TrimSpace(params.ContactName)
	if contactName == "" {
		contactName = strings.TrimSpace(params.Company)
	}
	if contactName != "" {
		contacts, err := s.client.GetContacts(ctx, rdClient.GetContactsParams{Name: contactName})
		if err != nil {
			return domain.DealCreationResult{}, fmt.Errorf("rdstation.CreateDeal search contact: %w", err)
		}
		contacts = exactContactsByName(contacts, contactName)
		if len(contacts) == 0 {
			return domain.DealCreationResult{}, &ContactNotFoundError{Name: contactName, Suggestions: s.suggestContactNames(ctx, contactName, 5)}
		}
		clientParams.ContactIDs = []string{contacts[0].ID}
	}

	if params.OwnerName != "" {
		owner, suggestions, err := s.findOwner(ctx, params.OwnerName)
		if err != nil {
			return domain.DealCreationResult{}, err
		}
		if owner.ID == "" {
			return domain.DealCreationResult{}, &OwnerNotFoundError{Name: params.OwnerName, Suggestions: suggestions}
		}
		clientParams.UserID = owner.ID
	}

	if params.ProductName != "" {
		clientParams.Products = []rdClient.DealProductParams{{
			Name:        params.ProductName,
			Description: params.Notes,
			Amount:      1,
			BasePrice:   0,
			Price:       0,
		}}
	}

	resp, err := s.client.CreateDeal(ctx, clientParams)
	if err != nil {
		return domain.DealCreationResult{}, err
	}

	deal := domain.Deal{
		ID:   resp.ID,
		Name: resp.Name,
		Stage: domain.Stage{
			ID:         resp.DealStage.ID,
			Name:       resp.DealStage.Name,
			PipelineID: resp.DealStage.DealPipelineID,
		},
		Owner:    mapOwner(resp),
		Contacts: mapDealContacts(resp.Contacts),
		Products: mapDealProducts(resp.Products),
	}
	result := domain.DealCreationResult{Deal: deal}
	if strings.TrimSpace(params.FollowUpSubject) != "" {
		task, taskErr := s.CreateScheduledTaskForDeal(ctx, deal, CreateScheduledTaskParams{
			Subject:        params.FollowUpSubject,
			Type:           params.FollowUpType,
			Date:           params.FollowUpDate,
			Hour:           params.FollowUpHour,
			Notes:          params.FollowUpNotes,
			UserID:         params.UserID,
			OwnerName:      params.OwnerName,
			IdempotencyKey: params.IdempotencyKey,
		})
		if taskErr != nil {
			result.TaskError = taskErr.Error()
		} else {
			result.Task = task
		}
	}
	return result, nil
}

func (s *Service) findExistingDealForCreate(ctx context.Context, params CreateDealParams) (domain.Deal, bool, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return domain.Deal{}, false, nil
	}
	resp, err := s.client.GetDeals(ctx, rdClient.GetDealsParams{Name: name})
	if err != nil {
		return domain.Deal{}, false, fmt.Errorf("rdstation.findExistingDealForCreate: %w", err)
	}
	deals := mapDeals(resp)
	exact := make([]domain.Deal, 0)
	normalizedName := normalizeSearchText(name)
	for _, deal := range deals {
		if normalizeSearchText(deal.Name) == normalizedName {
			exact = append(exact, deal)
		}
	}
	if len(exact) == 0 {
		return domain.Deal{}, false, nil
	}
	if strings.TrimSpace(params.Company) != "" {
		exact = filterDealsByCustomerName(exact, params.Company)
	}
	if strings.TrimSpace(params.ProductName) != "" {
		exact = filterDealsByProductName(exact, params.ProductName)
	}
	if strings.TrimSpace(params.Stage) != "" {
		stageID, err := s.findStageIDInPipeline(ctx, params.Stage, params.Pipeline)
		if err != nil {
			return domain.Deal{}, false, err
		}
		exact = filterDealsByStageID(exact, stageID)
	}
	if len(exact) == 0 {
		return domain.Deal{}, false, nil
	}
	sortDealsByUpdatedDesc(exact)
	return exact[0], true, nil
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
		stageID, stageErr := s.findStageIDInPipeline(ctx, params.Value, "")
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
			ID:         resp.DealStage.ID,
			Name:       resp.DealStage.Name,
			PipelineID: resp.DealStage.DealPipelineID,
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

	stageID, err := s.findStageIDInPipeline(ctx, targetStageName, "")
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
			ID:         resp.DealStage.ID,
			Name:       resp.DealStage.Name,
			PipelineID: resp.DealStage.DealPipelineID,
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

// GetDealSummaryForOwners gathers deal details, contacts, annotations and open tasks.
func (s *Service) GetDealSummaryForOwners(ctx context.Context, dealName string, allowedOwnerID map[string]struct{}) (domain.DealSummaryContext, error) {
	deal, err := s.findSingleDeal(ctx, dealName, allowedOwnerID)
	if err != nil {
		return domain.DealSummaryContext{}, err
	}

	return s.GetDealSummaryByID(ctx, deal.ID)
}

func (s *Service) GetDealSummaryResolved(ctx context.Context, params ResolveDealParams) (domain.DealSummaryContext, error) {
	deal, err := s.resolveDealForAction(ctx, params)
	if err != nil {
		return domain.DealSummaryContext{}, err
	}
	return s.GetDealSummaryByID(ctx, deal.ID)
}

// GetDealSummaryByID gathers all available CRM context for an executive summary.
func (s *Service) GetDealSummaryByID(ctx context.Context, dealID string) (domain.DealSummaryContext, error) {
	deal, err := s.GetDealByID(ctx, dealID)
	if err != nil {
		return domain.DealSummaryContext{}, err
	}

	contacts, err := s.GetDealContactsByID(ctx, deal.ID)
	if err != nil {
		return domain.DealSummaryContext{}, err
	}
	if len(contacts) == 0 {
		contacts = deal.Contacts
	}

	activities, err := s.GetDealActivitiesByID(ctx, deal.ID)
	if err != nil {
		return domain.DealSummaryContext{}, err
	}

	open := false
	openResp, err := s.client.GetTasks(ctx, rdClient.GetTasksParams{Done: &open, DealID: deal.ID, Limit: 20})
	if err != nil {
		return domain.DealSummaryContext{}, err
	}
	openTasks := make([]domain.Task, 0, len(openResp))
	for _, task := range openResp {
		openTasks = append(openTasks, mapTask(task))
	}

	done := true
	doneResp, err := s.client.GetTasks(ctx, rdClient.GetTasksParams{Done: &done, DealID: deal.ID, Limit: 20})
	if err != nil {
		return domain.DealSummaryContext{}, err
	}
	completedTasks := make([]domain.Task, 0, len(doneResp))
	for _, task := range doneResp {
		completedTasks = append(completedTasks, mapTask(task))
	}

	return domain.DealSummaryContext{
		Deal:           deal,
		Contacts:       contacts,
		Activities:     activities,
		OpenTasks:      openTasks,
		CompletedTasks: completedTasks,
	}, nil
}

// GetDealByID retrieves a single deal by its RD Station ID.
func (s *Service) GetDealByID(ctx context.Context, dealID string) (domain.Deal, error) {
	resp, err := s.client.GetDealByID(ctx, dealID)
	if err != nil {
		return domain.Deal{}, err
	}

	return domain.Deal{
		ID:   resp.ID,
		Name: resp.Name,
		Stage: domain.Stage{
			ID:         resp.DealStage.ID,
			Name:       resp.DealStage.Name,
			PipelineID: resp.DealStage.DealPipelineID,
		},
		Owner:     mapOwner(resp),
		Contacts:  mapDealContacts(resp.Contacts),
		Products:  mapDealProducts(resp.Products),
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
		return domain.Contact{}, &ContactNotFoundError{Name: params.ContactName, Suggestions: s.suggestContactNames(ctx, params.ContactName, 5)}
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
		return domain.Deal{}, &ContactNotFoundError{Name: contactName, Suggestions: s.suggestContactNames(ctx, contactName, 5)}
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

	return domain.Deal{
		ID:        resp.ID,
		Name:      resp.Name,
		Stage:     domain.Stage{ID: resp.DealStage.ID, Name: resp.DealStage.Name, PipelineID: resp.DealStage.DealPipelineID},
		Owner:     mapOwner(resp),
		Contacts:  mapDealContacts(resp.Contacts),
		Products:  mapDealProducts(resp.Products),
		CreatedAt: parseRDTime(resp.CreatedAt),
		UpdatedAt: parseRDTime(resp.UpdatedAt),
	}, nil
}

func (s *Service) CreateScheduledTask(ctx context.Context, params CreateScheduledTaskParams) (domain.Task, error) {
	deal, err := s.resolveDealForAction(ctx, ResolveDealParams{
		DealName:       params.DealName,
		Company:        params.Company,
		ProductName:    params.ProductName,
		Stage:          params.Stage,
		Pipeline:       params.Pipeline,
		OwnerName:      params.OwnerName,
		AllowedOwnerID: params.AllowedOwnerID,
	})
	if err != nil {
		return domain.Task{}, err
	}
	return s.CreateScheduledTaskForDeal(ctx, deal, params)
}

func (s *Service) CreateScheduledTaskForDeal(ctx context.Context, deal domain.Deal, params CreateScheduledTaskParams) (domain.Task, error) {
	taskType := strings.TrimSpace(params.Type)
	if taskType == "" {
		taskType = "task"
	}
	userIDs := []string{}
	if strings.TrimSpace(params.OwnerName) != "" {
		owner, suggestions, err := s.findOwner(ctx, params.OwnerName)
		if err != nil {
			return domain.Task{}, err
		}
		if owner.ID == "" {
			return domain.Task{}, &OwnerNotFoundError{Name: params.OwnerName, Suggestions: suggestions}
		}
		userIDs = []string{owner.ID}
	} else if strings.TrimSpace(params.UserID) != "" {
		userIDs = []string{strings.TrimSpace(params.UserID)}
	}
	if existing, ok, err := s.findExistingTask(ctx, deal.ID, params); err != nil {
		return domain.Task{}, err
	} else if ok {
		return existing, nil
	}
	resp, err := s.client.CreateTask(ctx, rdClient.CreateTaskParams{
		DealID:  deal.ID,
		Subject: params.Subject,
		Type:    taskType,
		Date:    params.Date,
		Hour:    params.Hour,
		Notes:   params.Notes,
		UserIDs: userIDs,
	})
	if err != nil {
		return domain.Task{}, err
	}
	return mapTask(resp), nil
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

func (s *Service) resolveDealForAction(ctx context.Context, params ResolveDealParams) (domain.Deal, error) {
	if strings.TrimSpace(params.DealName) != "" {
		return s.findSingleDeal(ctx, params.DealName, params.AllowedOwnerID)
	}
	if strings.TrimSpace(params.Company) == "" {
		return domain.Deal{}, &MissingDealNameError{}
	}
	stageID := ""
	if strings.TrimSpace(params.Stage) != "" {
		var err error
		stageID, err = s.findStageIDInPipeline(ctx, params.Stage, params.Pipeline)
		if err != nil {
			return domain.Deal{}, err
		}
	}
	resp, err := s.client.GetDeals(ctx, rdClient.GetDealsParams{})
	if err != nil {
		return domain.Deal{}, err
	}
	deals := filterDealsByOwner(mapDeals(resp), params.AllowedOwnerID)
	deals = filterDealsByCustomerName(deals, params.Company)
	deals = filterDealsByProductName(deals, params.ProductName)
	deals = filterDealsByStageID(deals, stageID)
	if strings.TrimSpace(params.OwnerName) != "" {
		deals = filterDealsByOwnerName(deals, params.OwnerName)
	}
	sortDealsByUpdatedDesc(deals)
	switch len(deals) {
	case 0:
		return domain.Deal{}, &DealNotFoundError{Name: params.Company}
	case 1:
		return deals[0], nil
	default:
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

func (s *Service) findStageIDInPipeline(ctx context.Context, stageName, pipelineName string) (string, error) {
	stages, err := s.client.GetDealStages(ctx)
	if err != nil {
		return "", fmt.Errorf("rdstation.findStageIDInPipeline: %w", err)
	}

	normalizedTarget := normalizeSearchText(stageName)
	normalizedPipeline := normalizeSearchText(pipelineName)
	available := make([]string, 0, len(stages))
	exactMatches := make([]domain.Stage, 0)
	partialMatches := make([]domain.Stage, 0)

	for _, st := range stages {
		available = append(available, st.Name)
		stage := domain.Stage{ID: st.ID, Name: st.Name, PipelineID: st.DealPipelineID}
		if normalizedPipeline != "" && normalizeSearchText(st.DealPipelineID) != normalizedPipeline {
			continue
		}

		normalizedStage := normalizeSearchText(st.Name)
		if normalizedStage == normalizedTarget {
			exactMatches = append(exactMatches, stage)
			continue
		}
		if strings.Contains(normalizedStage, normalizedTarget) || strings.Contains(normalizedTarget, normalizedStage) {
			partialMatches = append(partialMatches, stage)
		}
	}

	if len(exactMatches) == 1 {
		return exactMatches[0].ID, nil
	}
	if len(exactMatches) > 1 {
		return "", &AmbiguousStageError{Name: stageName, Stages: exactMatches}
	}
	if len(partialMatches) == 1 {
		return partialMatches[0].ID, nil
	}
	if len(partialMatches) > 1 {
		return "", &AmbiguousStageError{Name: stageName, Stages: partialMatches}
	}
	return "", &StageNotFoundError{Available: available}
}

func mapDeals(resp []rdClient.DealResponse) []domain.Deal {
	deals := make([]domain.Deal, 0, len(resp))
	for _, d := range resp {
		deals = append(deals, domain.Deal{
			ID:        d.ID,
			Name:      d.Name,
			Stage:     domain.Stage{ID: d.DealStage.ID, Name: d.DealStage.Name, PipelineID: d.DealStage.DealPipelineID},
			Owner:     mapOwner(d),
			Contacts:  mapDealContacts(d.Contacts),
			Products:  mapDealProducts(d.Products),
			CreatedAt: parseRDTime(d.CreatedAt),
			UpdatedAt: parseRDTime(d.UpdatedAt),
		})
	}
	return deals
}

func mapDealContacts(resp []rdClient.DealContactResponse) []domain.Contact {
	contacts := make([]domain.Contact, 0, len(resp))
	for _, c := range resp {
		contacts = append(contacts, domain.Contact{ID: c.ID, Name: c.Name})
	}
	return contacts
}

func mapDealProducts(resp []rdClient.DealProductResponse) []domain.DealProduct {
	products := make([]domain.DealProduct, 0, len(resp))
	for _, p := range resp {
		products = append(products, domain.DealProduct{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			Amount:      p.Amount,
			Price:       p.Price,
			Total:       p.Total,
		})
	}
	return products
}

func mapTask(resp rdClient.TaskResponse) domain.Task {
	names := make([]string, 0, len(resp.Users))
	for _, user := range resp.Users {
		name := strings.TrimSpace(user.Name)
		if name == "" {
			name = strings.TrimSpace(user.Nickname)
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return domain.Task{
		ID:               resp.ID,
		Subject:          resp.Subject,
		Type:             resp.Type,
		Date:             resp.Date,
		Hour:             resp.Hour,
		Notes:            resp.Notes,
		DealName:         resp.Deal.Name,
		ResponsibleNames: names,
	}
}

func (s *Service) findExistingTask(ctx context.Context, dealID string, params CreateScheduledTaskParams) (domain.Task, bool, error) {
	open := false
	resp, err := s.client.GetTasks(ctx, rdClient.GetTasksParams{Done: &open, DealID: dealID, Limit: 200})
	if err != nil {
		return domain.Task{}, false, fmt.Errorf("rdstation.findExistingTask: %w", err)
	}
	targetSubject := normalizeSearchText(params.Subject)
	targetType := strings.TrimSpace(params.Type)
	if targetType == "" {
		targetType = "task"
	}
	targetDate := strings.TrimSpace(params.Date)
	targetHour := strings.TrimSpace(params.Hour)
	for _, task := range resp {
		if normalizeSearchText(task.Subject) != targetSubject {
			continue
		}
		if strings.TrimSpace(task.Type) != targetType {
			continue
		}
		if strings.TrimSpace(task.Date) != targetDate || strings.TrimSpace(task.Hour) != targetHour {
			continue
		}
		return mapTask(task), true, nil
	}
	return domain.Task{}, false, nil
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

func filterDealsByCustomerName(deals []domain.Deal, customerName string) []domain.Deal {
	customerName = normalizeSearchText(customerName)
	if customerName == "" {
		return deals
	}
	exact := make([]domain.Deal, 0)
	for _, deal := range deals {
		for _, contact := range deal.Contacts {
			if normalizeSearchText(contact.Name) == customerName {
				exact = append(exact, deal)
				break
			}
		}
	}
	if len(exact) > 0 {
		return exact
	}

	partial := make([]domain.Deal, 0)
	for _, deal := range deals {
		for _, contact := range deal.Contacts {
			name := normalizeSearchText(contact.Name)
			if strings.Contains(name, customerName) || strings.Contains(customerName, name) {
				partial = append(partial, deal)
				break
			}
		}
	}
	return partial
}

func filterDealsByProductName(deals []domain.Deal, productName string) []domain.Deal {
	productName = normalizeSearchText(productName)
	if productName == "" {
		return deals
	}
	exact := make([]domain.Deal, 0)
	for _, deal := range deals {
		for _, product := range deal.Products {
			if normalizeSearchText(product.Name) == productName {
				exact = append(exact, deal)
				break
			}
		}
	}
	if len(exact) > 0 {
		return exact
	}
	partial := make([]domain.Deal, 0)
	for _, deal := range deals {
		for _, product := range deal.Products {
			name := normalizeSearchText(product.Name)
			if strings.Contains(name, productName) || strings.Contains(productName, name) {
				partial = append(partial, deal)
				break
			}
		}
	}
	return partial
}

func filterDealsByStageID(deals []domain.Deal, stageID string) []domain.Deal {
	stageID = strings.TrimSpace(stageID)
	if stageID == "" {
		return deals
	}
	out := make([]domain.Deal, 0, len(deals))
	for _, deal := range deals {
		if deal.Stage.ID == stageID {
			out = append(out, deal)
		}
	}
	return out
}

func filterDealsByUpdatedRange(deals []domain.Deal, updatedAfter, updatedBefore string) []domain.Deal {
	after, hasAfter := parseDateOnly(updatedAfter)
	before, hasBefore := parseDateOnly(updatedBefore)
	if !hasAfter && !hasBefore {
		return deals
	}

	out := make([]domain.Deal, 0, len(deals))
	for _, deal := range deals {
		if deal.UpdatedAt.IsZero() {
			continue
		}
		updated := dateOnly(deal.UpdatedAt)
		if hasAfter && updated.Before(after) {
			continue
		}
		if hasBefore && updated.After(before) {
			continue
		}
		out = append(out, deal)
	}
	return out
}

func exactContactsByName(contacts []rdClient.ContactResponse, target string) []rdClient.ContactResponse {
	normalizedTarget := normalizeSearchText(target)
	if normalizedTarget == "" {
		return contacts
	}
	exact := make([]rdClient.ContactResponse, 0)
	for _, contact := range contacts {
		if normalizeSearchText(contact.Name) == normalizedTarget {
			exact = append(exact, contact)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return contacts
}

func sortDealsByUpdatedDesc(deals []domain.Deal) {
	sort.SliceStable(deals, func(i, j int) bool {
		left := deals[i].UpdatedAt
		right := deals[j].UpdatedAt
		switch {
		case left.IsZero() && right.IsZero():
			return false
		case left.IsZero():
			return false
		case right.IsZero():
			return true
		default:
			return left.After(right)
		}
	})
}

func parseDateOnly(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func dateOnly(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func closestOwnerNames(deals []domain.Deal, target string, limit int) []string {
	seen := map[string]struct{}{}
	names := make([]string, 0)
	for _, deal := range deals {
		name := strings.TrimSpace(deal.Owner.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return closestStrings(names, target, limit)
}

func (s *Service) findOwner(ctx context.Context, target string) (domain.DealOwner, []string, error) {
	deals, err := s.client.GetDeals(ctx, rdClient.GetDealsParams{})
	if err != nil {
		return domain.DealOwner{}, nil, fmt.Errorf("rdstation.findOwner get deals: %w", err)
	}
	owners := make([]domain.DealOwner, 0)
	seen := map[string]struct{}{}
	for _, deal := range mapDeals(deals) {
		if deal.Owner.ID == "" {
			continue
		}
		key := deal.Owner.ID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		owners = append(owners, deal.Owner)
	}
	normalizedTarget := normalizeSearchText(target)
	for _, owner := range owners {
		name := normalizeSearchText(owner.Name)
		email := normalizeSearchText(owner.Email)
		if name == normalizedTarget || strings.Contains(name, normalizedTarget) || strings.Contains(normalizedTarget, name) || strings.Contains(email, normalizedTarget) {
			return owner, nil, nil
		}
	}
	ownerDeals := make([]domain.Deal, 0, len(owners))
	for _, owner := range owners {
		ownerDeals = append(ownerDeals, domain.Deal{Owner: owner})
	}
	return domain.DealOwner{}, closestOwnerNames(ownerDeals, target, 5), nil
}

func (s *Service) suggestContactNames(ctx context.Context, target string, limit int) []string {
	contacts, err := s.client.GetContacts(ctx, rdClient.GetContactsParams{})
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		if strings.TrimSpace(contact.Name) != "" {
			names = append(names, contact.Name)
		}
	}
	return closestStrings(names, target, limit)
}

func closestStrings(values []string, target string, limit int) []string {
	target = normalizeSearchText(target)
	type scored struct {
		value string
		score int
	}
	scoredValues := make([]scored, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := normalizeSearchText(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		score := levenshtein(key, target)
		if strings.Contains(key, target) || strings.Contains(target, key) {
			score = 0
		}
		scoredValues = append(scoredValues, scored{value: value, score: score})
	}
	for i := 0; i < len(scoredValues); i++ {
		for j := i + 1; j < len(scoredValues); j++ {
			if scoredValues[j].score < scoredValues[i].score {
				scoredValues[i], scoredValues[j] = scoredValues[j], scoredValues[i]
			}
		}
	}
	if limit <= 0 || limit > len(scoredValues) {
		limit = len(scoredValues)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, scoredValues[i].value)
	}
	return out
}

func levenshtein(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			curr[j] = minInt(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

func minInt(values ...int) int {
	min := values[0]
	for _, value := range values[1:] {
		if value < min {
			min = value
		}
	}
	return min
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
	Company        string
	ProductName    string
	Stage          string
	Pipeline       string
	UserID         string
	Text           string
	AllowedOwnerID map[string]struct{}
	IdempotencyKey string
}

// GetDealActivities returns the manual annotations of a deal identified by name.
func (s *Service) GetDealActivities(ctx context.Context, dealName string, allowedOwnerID map[string]struct{}) ([]domain.Activity, error) {
	deal, err := s.findSingleDeal(ctx, dealName, allowedOwnerID)
	if err != nil {
		return nil, err
	}

	return s.GetDealActivitiesByID(ctx, deal.ID)
}

func (s *Service) GetDealActivitiesResolved(ctx context.Context, params ResolveDealParams) ([]domain.Activity, error) {
	deal, err := s.resolveDealForAction(ctx, params)
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

func (s *Service) findExistingActivity(ctx context.Context, dealID, text string) (domain.Activity, bool, error) {
	activities, err := s.GetDealActivitiesByID(ctx, dealID)
	if err != nil {
		return domain.Activity{}, false, fmt.Errorf("rdstation.findExistingActivity: %w", err)
	}
	target := normalizeSearchText(text)
	for _, activity := range activities {
		if normalizeSearchText(activity.Text) == target {
			return activity, true, nil
		}
	}
	return domain.Activity{}, false, nil
}

// CreateDealActivity registers a manual annotation in a deal identified by name.
func (s *Service) CreateDealActivity(ctx context.Context, params CreateDealActivityParams) (domain.Activity, error) {
	if params.UserID == "" {
		return domain.Activity{}, fmt.Errorf("seu perfil não possui RD Station ID configurado; contate o administrador")
	}

	deal, err := s.resolveDealForAction(ctx, ResolveDealParams{
		DealName:       params.DealName,
		Company:        params.Company,
		ProductName:    params.ProductName,
		Stage:          params.Stage,
		AllowedOwnerID: params.AllowedOwnerID,
	})
	if err != nil {
		return domain.Activity{}, err
	}

	if existing, ok, err := s.findExistingActivity(ctx, deal.ID, params.Text); err != nil {
		return domain.Activity{}, err
	} else if ok {
		return existing, nil
	}

	resp, err := s.client.CreateActivity(ctx, deal.ID, params.UserID, params.Text)
	if err != nil {
		return domain.Activity{}, err
	}

	return domain.Activity{ID: resp.ID, Text: resp.Text, Date: resp.Date}, nil
}
