package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/prometheus/common/log"
)

const (
	serviceNowBaseURL = "https://%s.service-now.com"
	tableAPI          = "%s/api/now/v2/table/%s"
	groupKeySnField   = "u_other_reference_1"
)

// Incident is a model of the created ServiceNow incident
type Incident struct {
	AssignmentGroup  LinkedValue `json:"assignment_group"`
	ContactType      string      `json:"contact_type,omitempty"`
	CallerID         LinkedValue `json:"caller_id,omitempty"`
	Comments         string      `json:"comments"`
	Description      string      `json:"description"`
	GroupKey         string      `json:"u_other_reference_1"`
	Impact           json.Number `json:"impact,omitempty"`
	Number           string      `json:"number,omitempty"`
	Priority         string      `json:"priority,omitempty"`
	ShortDescription string      `json:"short_description"`
	State            json.Number `json:"state,omitempty"`
	SysID            string      `json:"sys_id,omitempty"`
	Urgency          json.Number `json:"urgency,omitempty"`
}

// LinkedValue is a basic ServiceNow element containing a link field and a value field (or could be a string)
type LinkedValue interface{}

// IncidentsResponse is a model of a response with multiple incidents
type IncidentsResponse struct {
	Result []Incident `json:"result"`
}

// IncidentResponse is a model of a response with only one incident
type IncidentResponse struct {
	Result Incident `json:"result"`
}

// ServiceNow interface
type ServiceNow interface {
	CreateIncident(incident Incident) (*Incident, error)
	GetIncidents(params map[string]string) ([]Incident, error)
	UpdateIncident(incident Incident) (*Incident, error)
}

// ServiceNowClient is the interface to a ServiceNow instance
type ServiceNowClient struct {
	baseURL    string
	authHeader string
	client     *http.Client
}

// NewServiceNowClient will create a new ServiceNow client
func NewServiceNowClient(instanceName string, userName string, password string) (*ServiceNowClient, error) {
	if instanceName == "" {
		return nil, errors.New("Missing instanceName")
	}

	if userName == "" {
		return nil, errors.New("Missing userName")
	}

	if password == "" {
		return nil, errors.New("Missing password")
	}

	return &ServiceNowClient{
		baseURL:    fmt.Sprintf(serviceNowBaseURL, instanceName),
		authHeader: fmt.Sprintf("Basic %s", base64.URLEncoding.EncodeToString([]byte(userName+":"+password))),
		client:     http.DefaultClient,
	}, nil
}

// Create a table item in ServiceNow from a post body
func (snClient *ServiceNowClient) create(table string, body []byte) ([]byte, error) {
	url := fmt.Sprintf(tableAPI, snClient.baseURL, table)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		log.Errorf("Error creating the request. %s", err)
		return nil, err
	}

	return snClient.doRequest(req)
}

// get a table item from ServiceNow using a map of arguments
func (snClient *ServiceNowClient) get(table string, params map[string]string) ([]byte, error) {
	url := fmt.Sprintf(tableAPI, snClient.baseURL, table)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Errorf("Error creating the request. %s", err)
		return nil, err
	}

	q := req.URL.Query()
	for key, val := range params {
		q.Add(key, val)
	}
	req.URL.RawQuery = q.Encode()

	return snClient.doRequest(req)
}

// update a table item in ServiceNow from a post body and a sys_id
func (snClient *ServiceNowClient) update(table string, body []byte, sysID string) ([]byte, error) {
	url := fmt.Sprintf(tableAPI+"/%s", snClient.baseURL, table, sysID)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(body))
	if err != nil {
		log.Errorf("Error creating the request. %s", err)
		return nil, err
	}

	return snClient.doRequest(req)
}

// doRequest will do the given ServiceNow request and return response as byte array
func (snClient *ServiceNowClient) doRequest(req *http.Request) ([]byte, error) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", snClient.authHeader)
	resp, err := snClient.client.Do(req)
	if err != nil {
		log.Errorf("Error sending the request. %s", err)
		return nil, err
	}
	if resp.StatusCode >= 400 {
		errorMsg := fmt.Sprintf("ServiceNow returned the HTTP error code: %v", resp.StatusCode)
		log.Error(errorMsg)
		return nil, errors.New(errorMsg)
	}

	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Error reading the body. %s", err)
		return nil, err
	}

	return responseBody, nil
}

// CreateIncident will create an incident in ServiceNow from a given Incident, and return the created incident
func (snClient *ServiceNowClient) CreateIncident(incident Incident) (*Incident, error) {
	log.Info("Create a ServiceNow incident")

	postBody, err := json.Marshal(incident)
	if err != nil {
		log.Errorf("Error while marshalling the incident. %s", err)
		return nil, err
	}

	response, err := snClient.create("incident", postBody)
	if err != nil {
		log.Errorf("Error while creating the incident. %s", err)
		return nil, err
	}

	incidentResponse := IncidentResponse{}
	err = json.Unmarshal(response, &incidentResponse)
	if err != nil {
		log.Errorf("Error while unmarshalling the incident. %s", err)
		return nil, err
	}

	log.Infof("Incident %s created", incidentResponse.Result.Number)

	return &incidentResponse.Result, nil
}

// GetIncidents will retrieve an incident from ServiceNow
func (snClient *ServiceNowClient) GetIncidents(params map[string]string) ([]Incident, error) {
	log.Infof("Get ServiceNow incidents with params: %v", params)
	response, err := snClient.get("incident", params)

	if err != nil {
		log.Errorf("Error while getting the incident. %s", err)
		return nil, err
	}

	incidentsResponse := IncidentsResponse{}
	err = json.Unmarshal(response, &incidentsResponse)
	if err != nil {
		log.Errorf("Error while unmarshalling the incident. %s", err)
		return nil, err
	}

	return incidentsResponse.Result, nil
}

// UpdateIncident will update an incident in ServiceNow from a given Incident, and return the updated incident
func (snClient *ServiceNowClient) UpdateIncident(incident Incident) (*Incident, error) {
	log.Infof("Update ServiceNow incident %s", incident.Number)

	postBody, err := json.Marshal(incident)
	if err != nil {
		log.Errorf("Error while marshalling the incident. %s", err)
		return nil, err
	}

	response, err := snClient.update("incident", postBody, incident.SysID)
	if err != nil {
		log.Errorf("Error while updating the incident. %s", err)
		return nil, err
	}

	incidentResponse := IncidentResponse{}
	err = json.Unmarshal(response, &incidentResponse)
	if err != nil {
		log.Errorf("Error while unmarshalling the incident. %s", err)
		return nil, err
	}

	log.Infof("Incident %s updated", incidentResponse.Result.Number)

	return &incidentResponse.Result, nil
}
