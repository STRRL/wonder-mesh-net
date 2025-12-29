package controller

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// ServiceAccountController handles Keycloak service account management.
type ServiceAccountController struct {
	wonderNetService *service.WonderNetService
}

// NewServiceAccountController creates a new ServiceAccountController.
func NewServiceAccountController(wonderNetService *service.WonderNetService) *ServiceAccountController {
	return &ServiceAccountController{
		wonderNetService: wonderNetService,
	}
}

// ServiceAccountResponse represents a service account in JSON responses.
type ServiceAccountResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	Description  string `json:"description,omitempty"`
}

// ServiceAccountListResponse represents the response for listing service accounts.
type ServiceAccountListResponse struct {
	ServiceAccounts []ServiceAccountResponse `json:"service_accounts"`
}

// HandleCreate handles POST /api/v1/service-accounts requests.
// Creates a new Keycloak service account for the authenticated user's wonder net.
func (c *ServiceAccountController) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wonderNet := WonderNetFromContext(r)
	if wonderNet == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	details, err := c.wonderNetService.CreateServiceAccount(r.Context(), wonderNet, req.Name)
	if err != nil {
		slog.Error("create service account", "error", err)
		http.Error(w, "create service account", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(ServiceAccountResponse{
		ClientID:     details.ClientID,
		ClientSecret: details.ClientSecret,
	})
}

// HandleList handles GET /api/v1/service-accounts requests.
// Lists all service accounts for the authenticated user's wonder net.
func (c *ServiceAccountController) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wonderNet := WonderNetFromContext(r)
	if wonderNet == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	accounts, err := c.wonderNetService.ListServiceAccounts(r.Context(), wonderNet)
	if err != nil {
		slog.Error("list service accounts", "error", err)
		http.Error(w, "list service accounts", http.StatusInternalServerError)
		return
	}

	response := ServiceAccountListResponse{
		ServiceAccounts: make([]ServiceAccountResponse, len(accounts)),
	}
	for i, account := range accounts {
		response.ServiceAccounts[i] = ServiceAccountResponse{
			ClientID:    account.ClientID,
			Description: account.Description,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// HandleDelete handles DELETE /api/v1/service-accounts/{id} requests.
// Deletes a service account by its client ID.
func (c *ServiceAccountController) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wonderNet := WonderNetFromContext(r)
	if wonderNet == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	clientID := r.PathValue("id")
	if clientID == "" {
		http.Error(w, "client_id is required", http.StatusBadRequest)
		return
	}

	if err := c.wonderNetService.DeleteServiceAccount(r.Context(), clientID, wonderNet); err != nil {
		if errors.Is(err, service.ErrServiceAccountNotFound) {
			http.Error(w, "service account not found", http.StatusNotFound)
			return
		}
		slog.Error("delete service account", "error", err, "client_id", clientID)
		http.Error(w, "delete service account", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
