package scheduledjob

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	serviceAccountDirectory = "/var/run/secrets/kubernetes.io/serviceaccount"
	managedByLabel          = "app.kubernetes.io/managed-by"
	managedByValue          = "planty-manual-run"
	jobIDLabel              = "planty.stout.zone/scheduled-job"
	manualRunTTLSeconds     = 7 * 24 * 60 * 60
	maximumResponseBytes    = 2 << 20
)

type KubernetesConfig struct {
	BaseURL   string
	Namespace string
	Token     string
	Client    *http.Client
}

type Kubernetes struct {
	baseURL   string
	namespace string
	token     string
	client    *http.Client
	startMu   sync.Mutex
}

func NewKubernetes(config KubernetesConfig) (*Kubernetes, error) {
	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.Namespace) == "" ||
		strings.TrimSpace(config.Token) == "" || config.Client == nil {
		return nil, fmt.Errorf("%w: incomplete Kubernetes configuration", ErrUnavailable)
	}
	return &Kubernetes{
		baseURL: strings.TrimRight(config.BaseURL, "/"), namespace: config.Namespace,
		token: strings.TrimSpace(config.Token), client: config.Client,
	}, nil
}

// NewInCluster uses only the namespace-scoped service-account credential that
// Kubernetes mounts into the API pod. The caller grants that account the small
// Role in deploy/deployment.yaml; no kubeconfig or cluster credential exists in
// the container.
func NewInCluster() (*Kubernetes, error) {
	host := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST"))
	port := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS"))
	if port == "" {
		port = strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_PORT"))
	}
	if host == "" || port == "" {
		return nil, fmt.Errorf("%w: Kubernetes service environment is absent", ErrUnavailable)
	}

	token, err := os.ReadFile(path.Join(serviceAccountDirectory, "token"))
	if err != nil {
		return nil, fmt.Errorf("%w: read service-account token: %v", ErrUnavailable, err)
	}
	namespace, err := os.ReadFile(path.Join(serviceAccountDirectory, "namespace"))
	if err != nil {
		return nil, fmt.Errorf("%w: read service-account namespace: %v", ErrUnavailable, err)
	}
	certificate, err := os.ReadFile(path.Join(serviceAccountDirectory, "ca.crt"))
	if err != nil {
		return nil, fmt.Errorf("%w: read cluster CA: %v", ErrUnavailable, err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificate) {
		return nil, fmt.Errorf("%w: cluster CA is invalid", ErrUnavailable)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	return NewKubernetes(KubernetesConfig{
		BaseURL:   "https://" + net.JoinHostPort(host, port),
		Namespace: strings.TrimSpace(string(namespace)),
		Token:     string(token),
		Client:    &http.Client{Transport: transport, Timeout: 20 * time.Second},
	})
}

func (k *Kubernetes) List(ctx context.Context) ([]Scheduled, error) {
	cronJobs, err := k.cronJobs(ctx)
	if err != nil {
		return nil, err
	}
	runs, err := k.runs(ctx)
	if err != nil {
		return nil, err
	}

	latest := make(map[ID]Run)
	for _, run := range runs {
		current, exists := latest[run.Job]
		if !exists || runTime(run).After(runTime(current)) {
			latest[run.Job] = run
		}
	}

	byName := make(map[string]cronJob, len(cronJobs.Items))
	for _, item := range cronJobs.Items {
		byName[item.Metadata.Name] = item
	}

	definitions := Definitions()
	out := make([]Scheduled, 0, len(definitions))
	for _, definition := range definitions {
		cron, found := byName[definition.CronJob]
		if !found {
			return nil, fmt.Errorf("scheduled job template %s is missing", definition.CronJob)
		}
		scheduled := Scheduled{
			ID: definition.ID, Name: definition.Name, Purpose: definition.Purpose,
			Category: definition.Category, Cadence: definition.Cadence, Schedule: cron.Spec.Schedule,
			TimeZone: cron.Spec.TimeZone, Suspended: cron.Spec.Suspend,
		}
		if run, found := latest[definition.ID]; found {
			scheduled.LatestRun = &run
		}
		out = append(out, scheduled)
	}
	return out, nil
}

func (k *Kubernetes) Start(ctx context.Context, id ID) (Run, bool, error) {
	definition, found := Lookup(id)
	if !found {
		return Run{}, false, fmt.Errorf("unknown scheduled job %q", id)
	}

	// The API currently has one replica, and this also closes the double-tap
	// race inside that replica. Kubernetes remains the durable source of truth.
	k.startMu.Lock()
	defer k.startMu.Unlock()

	cron, err := k.cronJob(ctx, definition.CronJob)
	if err != nil {
		return Run{}, false, err
	}
	if len(cron.Status.Active) > 0 {
		run, err := k.runForReference(ctx, id, cron.Status.Active[0])
		return run, false, err
	}

	runs, err := k.runs(ctx)
	if err != nil {
		return Run{}, false, err
	}
	for _, run := range runs {
		if run.Job == id && run.Active() {
			return run, false, nil
		}
	}

	request, err := jobFromCronJob(definition, cron)
	if err != nil {
		return Run{}, false, err
	}
	var created job
	if err := k.request(ctx, http.MethodPost, k.resourceURL("jobs", ""), request, &created); err != nil {
		return Run{}, false, err
	}
	run, ok := runFromJob(created)
	if !ok {
		return Run{}, false, errors.New("Kubernetes created a manual job without its job identity")
	}
	return run, true, nil
}

func (k *Kubernetes) cronJobs(ctx context.Context) (cronJobList, error) {
	var list cronJobList
	err := k.request(ctx, http.MethodGet, k.resourceURL("cronjobs", ""), nil, &list)
	return list, err
}

func (k *Kubernetes) cronJob(ctx context.Context, name string) (cronJob, error) {
	var result cronJob
	err := k.request(ctx, http.MethodGet, k.resourceURL("cronjobs", name), nil, &result)
	return result, err
}

func (k *Kubernetes) runs(ctx context.Context) ([]Run, error) {
	var list jobList
	if err := k.request(ctx, http.MethodGet, k.resourceURL("jobs", ""), nil, &list); err != nil {
		return nil, err
	}
	runs := make([]Run, 0, len(list.Items))
	for _, item := range list.Items {
		if run, ok := runFromJob(item); ok {
			runs = append(runs, run)
		}
	}
	SortRunsNewestFirst(runs)
	return runs, nil
}

func (k *Kubernetes) runForReference(ctx context.Context, id ID, reference objectReference) (Run, error) {
	var item job
	if err := k.request(ctx, http.MethodGet, k.resourceURL("jobs", reference.Name), nil, &item); err != nil {
		return Run{}, err
	}
	return runWithID(item, id), nil
}

func (k *Kubernetes) resourceURL(resource, name string) string {
	endpoint := k.baseURL + "/apis/batch/v1/namespaces/" + url.PathEscape(k.namespace) + "/" + resource
	if name != "" {
		endpoint += "/" + url.PathEscape(name)
	}
	return endpoint
}

func (k *Kubernetes) request(ctx context.Context, method, endpoint string, body, output any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode Kubernetes request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("build Kubernetes request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+k.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "planty-scheduled-jobs")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	response, err := k.client.Do(req)
	if err != nil {
		return fmt.Errorf("call Kubernetes: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maximumResponseBytes)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var status statusResponse
		_ = json.NewDecoder(limited).Decode(&status)
		if status.Message == "" {
			status.Message = response.Status
		}
		return fmt.Errorf("Kubernetes %s %s: %s", method, endpoint, status.Message)
	}
	if output == nil {
		return nil
	}
	if err := json.NewDecoder(limited).Decode(output); err != nil {
		return fmt.Errorf("decode Kubernetes response: %w", err)
	}
	return nil
}

func jobFromCronJob(definition Definition, cron cronJob) (jobRequest, error) {
	if len(cron.Spec.JobTemplate.Spec) == 0 {
		return jobRequest{}, fmt.Errorf("CronJob %s has no job template", cron.Metadata.Name)
	}
	var spec map[string]any
	if err := json.Unmarshal(cron.Spec.JobTemplate.Spec, &spec); err != nil {
		return jobRequest{}, fmt.Errorf("decode CronJob %s template: %w", cron.Metadata.Name, err)
	}
	spec["ttlSecondsAfterFinished"] = manualRunTTLSeconds

	labels := make(map[string]string, len(cron.Spec.JobTemplate.Metadata.Labels)+2)
	for key, value := range cron.Spec.JobTemplate.Metadata.Labels {
		labels[key] = value
	}
	labels[managedByLabel] = managedByValue
	labels[jobIDLabel] = string(definition.ID)

	return jobRequest{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Metadata: objectMetadata{
			GenerateName: "planty-manual-" + string(definition.ID) + "-",
			Namespace:    cron.Metadata.Namespace,
			Labels:       labels,
			Annotations:  cron.Spec.JobTemplate.Metadata.Annotations,
		},
		Spec: spec,
	}, nil
}

func runFromJob(item job) (Run, bool) {
	if id, err := ValidateID(item.Metadata.Labels[jobIDLabel]); err == nil {
		return runWithID(item, id), true
	}
	for _, owner := range item.Metadata.OwnerReferences {
		if owner.Kind != "CronJob" {
			continue
		}
		if definition, found := DefinitionForCronJob(owner.Name); found {
			return runWithID(item, definition.ID), true
		}
	}
	return Run{}, false
}

func runWithID(item job, id ID) Run {
	startedAt := item.Status.StartTime
	if startedAt == nil {
		startedAt = item.Metadata.CreationTimestamp
	}
	run := Run{ID: item.Metadata.Name, Job: id, State: Queued, StartedAt: startedAt}
	for _, condition := range item.Status.Conditions {
		if condition.Status != "True" {
			continue
		}
		switch condition.Type {
		case "Complete":
			run.State, run.CompletedAt = Succeeded, conditionTime(condition, item.Status.CompletionTime)
			return run
		case "Failed":
			run.State, run.Detail = Failed, strings.TrimSpace(condition.Message)
			run.CompletedAt = conditionTime(condition, item.Status.CompletionTime)
			return run
		}
	}
	if item.Status.Active > 0 || item.Status.StartTime != nil {
		run.State = Running
	} else if item.Status.Failed > 0 {
		run.State = Failed
	}
	return run
}

func conditionTime(condition jobCondition, fallback *time.Time) *time.Time {
	if condition.LastTransitionTime != nil {
		return condition.LastTransitionTime
	}
	return fallback
}

type objectMetadata struct {
	Name              string            `json:"name,omitempty"`
	GenerateName      string            `json:"generateName,omitempty"`
	Namespace         string            `json:"namespace,omitempty"`
	CreationTimestamp *time.Time        `json:"creationTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	OwnerReferences   []ownerReference  `json:"ownerReferences,omitempty"`
}

type ownerReference struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type objectReference struct {
	Name string `json:"name"`
}

type jobTemplate struct {
	Metadata objectMetadata  `json:"metadata"`
	Spec     json.RawMessage `json:"spec"`
}

type cronJob struct {
	Metadata objectMetadata `json:"metadata"`
	Spec     struct {
		Schedule    string      `json:"schedule"`
		TimeZone    string      `json:"timeZone"`
		Suspend     bool        `json:"suspend"`
		JobTemplate jobTemplate `json:"jobTemplate"`
	} `json:"spec"`
	Status struct {
		Active []objectReference `json:"active"`
	} `json:"status"`
}

type cronJobList struct {
	Items []cronJob `json:"items"`
}

type jobCondition struct {
	Type               string     `json:"type"`
	Status             string     `json:"status"`
	Message            string     `json:"message"`
	LastTransitionTime *time.Time `json:"lastTransitionTime"`
}

type job struct {
	Metadata objectMetadata `json:"metadata"`
	Status   struct {
		StartTime      *time.Time     `json:"startTime"`
		CompletionTime *time.Time     `json:"completionTime"`
		Active         int            `json:"active"`
		Succeeded      int            `json:"succeeded"`
		Failed         int            `json:"failed"`
		Conditions     []jobCondition `json:"conditions"`
	} `json:"status"`
}

type jobList struct {
	Items []job `json:"items"`
}

type jobRequest struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   objectMetadata `json:"metadata"`
	Spec       map[string]any `json:"spec"`
}

type statusResponse struct {
	Message string `json:"message"`
}
