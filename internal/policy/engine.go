package policy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
)

const (
	evaluationTimeout  = 250 * time.Millisecond
	maxPreparedQueries = 128
)

var forbiddenBuiltins = map[string]struct{}{
	"http.send":          {},
	"net.lookup_ip_addr": {},
	"opa.runtime":        {},
	"rand.intn":          {},
	"time.now_ns":        {},
	"uuid.rfc4122":       {},
}

var preparedQueries = preparedCache{entries: make(map[[sha256.Size]byte]rego.PreparedEvalQuery)}

type preparedCache struct {
	sync.Mutex
	entries map[[sha256.Size]byte]rego.PreparedEvalQuery
	order   [][sha256.Size]byte
}

func (c *preparedCache) load(key [sha256.Size]byte) (rego.PreparedEvalQuery, bool) {
	c.Lock()
	defer c.Unlock()
	query, ok := c.entries[key]
	return query, ok
}

func (c *preparedCache) store(key [sha256.Size]byte, query rego.PreparedEvalQuery) rego.PreparedEvalQuery {
	c.Lock()
	defer c.Unlock()
	if existing, ok := c.entries[key]; ok {
		return existing
	}
	if len(c.order) == maxPreparedQueries {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
	c.entries[key] = query
	c.order = append(c.order, key)
	return query
}

type Engine struct{}

func (Engine) Compile(ctx context.Context, source string) error {
	_, err := prepare(ctx, source)
	return err
}

func (Engine) Evaluate(ctx context.Context, source string, input Input) (Decision, time.Duration, error) {
	started := time.Now()
	query, err := prepare(ctx, source)
	if err != nil {
		return Decision{}, time.Since(started), err
	}

	evalCtx, cancel := context.WithTimeout(ctx, evaluationTimeout)
	defer cancel()
	results, err := query.Eval(evalCtx, rego.EvalInput(input))
	if err != nil {
		return Decision{}, time.Since(started), fmt.Errorf("evaluate %s: %w", Entrypoint, err)
	}
	if len(results) != 1 || len(results[0].Expressions) != 1 {
		return Decision{}, time.Since(started), fmt.Errorf("%s must return exactly one decision object", Entrypoint)
	}

	raw, err := json.Marshal(results[0].Expressions[0].Value)
	if err != nil {
		return Decision{}, time.Since(started), fmt.Errorf("encode decision: %w", err)
	}
	if len(raw) > MaxDecisionBytes {
		return Decision{}, time.Since(started), fmt.Errorf("decision exceeds %d bytes", MaxDecisionBytes)
	}
	var decision Decision
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decision); err != nil {
		return Decision{}, time.Since(started), fmt.Errorf("decision does not match Planty's output contract: %w", err)
	}
	normalizeDecision(&decision)
	if err := decision.Valid(); err != nil {
		return Decision{}, time.Since(started), err
	}
	return decision, time.Since(started), nil
}

func prepare(ctx context.Context, source string) (rego.PreparedEvalQuery, error) {
	if len(source) > MaxSourceBytes {
		return rego.PreparedEvalQuery{}, fmt.Errorf("policy source exceeds %d bytes", MaxSourceBytes)
	}
	fingerprint := sha256.Sum256([]byte(source))
	if cached, ok := preparedQueries.load(fingerprint); ok {
		return cached, nil
	}
	module, err := ast.ParseModuleWithOpts("planty.rego", source, ast.ParserOptions{RegoVersion: ast.RegoV1})
	if err != nil {
		return rego.PreparedEvalQuery{}, fmt.Errorf("compile policy: %w", err)
	}
	if module == nil || module.Package.Path.String() != "data.planty" || !hasDecisionRule(module) {
		return rego.PreparedEvalQuery{}, fmt.Errorf("compile policy: %s must define a decision rule", Entrypoint)
	}
	query, err := rego.New(
		rego.Query(Entrypoint),
		rego.ParsedModule(module),
		rego.SetRegoVersion(ast.RegoV1),
		rego.Strict(true),
		rego.StrictBuiltinErrors(true),
		rego.UnsafeBuiltins(forbiddenBuiltins),
	).PrepareForEval(ctx)
	if err != nil {
		return rego.PreparedEvalQuery{}, fmt.Errorf("compile policy: %w", err)
	}
	return preparedQueries.store(fingerprint, query), nil
}

func hasDecisionRule(module *ast.Module) bool {
	for _, rule := range module.Rules {
		if rule.Head.Name.String() == "decision" && len(rule.Head.Args) == 0 {
			return true
		}
	}
	return false
}

func normalizeDecision(decision *Decision) {
	if decision.Signals == nil {
		decision.Signals = []Signal{}
	}
	if decision.Notifications == nil {
		decision.Notifications = []Notification{}
	}
	if decision.FanRuns == nil {
		decision.FanRuns = []FanRun{}
	}
	if decision.Agent.Facts == nil {
		decision.Agent.Facts = []string{}
	}
	if decision.Agent.Guidance == nil {
		decision.Agent.Guidance = []string{}
	}
	if decision.Agent.DenyActions == nil {
		decision.Agent.DenyActions = []string{}
	}
}

func (d Decision) Valid() error {
	if strings.TrimSpace(d.Summary) == "" {
		return fmt.Errorf("decision summary is required")
	}
	if len(d.Summary) > MaxDecisionTextBytes {
		return fmt.Errorf("decision summary exceeds %d bytes", MaxDecisionTextBytes)
	}
	if len(d.Signals) > MaxDecisionItems || len(d.Notifications) > MaxDecisionItems ||
		len(d.FanRuns) > MaxDecisionItems || len(d.Agent.Facts) > MaxDecisionItems ||
		len(d.Agent.Guidance) > MaxDecisionItems || len(d.Agent.DenyActions) > MaxDecisionItems {
		return fmt.Errorf("decision arrays may contain at most %d items each", MaxDecisionItems)
	}
	for i, signal := range d.Signals {
		switch signal.Kind {
		case SignalNeedsWatered, SignalNeedsMisted, SignalMoveInside, SignalMoveOutside,
			SignalIncident, SignalHealth, SignalAirflow:
		default:
			return fmt.Errorf("signals[%d].kind %q is not supported", i, signal.Kind)
		}
		if err := validSeverity(signal.Severity); err != nil {
			return fmt.Errorf("signals[%d]: %w", i, err)
		}
		if strings.TrimSpace(signal.Reason) == "" {
			return fmt.Errorf("signals[%d].reason is required", i)
		}
		if len(signal.Reason) > MaxDecisionTextBytes {
			return fmt.Errorf("signals[%d].reason exceeds %d bytes", i, MaxDecisionTextBytes)
		}
		if signal.Confidence < 0 || signal.Confidence > 1 {
			return fmt.Errorf("signals[%d].confidence must be between 0 and 1", i)
		}
	}
	if d.Health != nil {
		if d.Health.Delta == 0 || d.Health.Delta < -20 || d.Health.Delta > 20 {
			return fmt.Errorf("health.delta must be non-zero and between -20 and 20")
		}
		if strings.TrimSpace(d.Health.Reason) == "" {
			return fmt.Errorf("health.reason is required")
		}
		if len(d.Health.Reason) > MaxDecisionTextBytes {
			return fmt.Errorf("health.reason exceeds %d bytes", MaxDecisionTextBytes)
		}
	}
	for i, notification := range d.Notifications {
		if strings.TrimSpace(notification.Title) == "" || strings.TrimSpace(notification.Body) == "" {
			return fmt.Errorf("notifications[%d] needs a title and body", i)
		}
		if len(notification.Title) > MaxDecisionTextBytes || len(notification.Body) > MaxDecisionTextBytes {
			return fmt.Errorf("notifications[%d] text exceeds %d bytes", i, MaxDecisionTextBytes)
		}
		if err := validSeverity(notification.Priority); err != nil {
			return fmt.Errorf("notifications[%d]: %w", i, err)
		}
	}
	for i, run := range d.FanRuns {
		if run.ActuatorID == uuid.Nil {
			return fmt.Errorf("fan_runs[%d].actuator_id is required", i)
		}
		if run.DurationSeconds < 1 || run.DurationSeconds > 3600 {
			return fmt.Errorf("fan_runs[%d].duration_seconds must be between 1 and 3600", i)
		}
		if strings.TrimSpace(run.Reason) == "" {
			return fmt.Errorf("fan_runs[%d].reason is required", i)
		}
		if len(run.Reason) > MaxDecisionTextBytes {
			return fmt.Errorf("fan_runs[%d].reason exceeds %d bytes", i, MaxDecisionTextBytes)
		}
	}
	for kind, values := range map[string][]string{
		"agent.facts": d.Agent.Facts, "agent.guidance": d.Agent.Guidance,
		"agent.deny_actions": d.Agent.DenyActions,
	} {
		for i, value := range values {
			if strings.TrimSpace(value) == "" || len(value) > MaxDecisionTextBytes {
				return fmt.Errorf("%s[%d] must be non-empty and no more than %d bytes", kind, i, MaxDecisionTextBytes)
			}
		}
	}
	return nil
}

func validSeverity(severity Severity) error {
	switch severity {
	case SeverityInfo, SeverityWarning, SeverityCritical:
		return nil
	default:
		return fmt.Errorf("severity %q is not supported", severity)
	}
}
