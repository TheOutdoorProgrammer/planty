package policy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
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

var supportedRules = map[string]struct{}{
	"agent_fact": {}, "agent_facts": {}, "agent_guidance": {},
	"deny_action": {}, "deny_actions": {}, "fan_run": {}, "fan_runs": {},
	"health": {}, "health_adjustment": {}, "incident": {},
	"move_inside": {}, "move_outside": {}, "needs_airflow": {},
	"needs_fertilized": {}, "needs_light": {}, "needs_misted": {},
	"needs_pruned": {}, "needs_repotted": {}, "needs_shade": {}, "needs_water": {},
	"notification": {}, "notifications": {},
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

func (Engine) Evaluate(ctx context.Context, source string, input Input) (Result, time.Duration, error) {
	started := time.Now()
	query, err := prepare(ctx, source)
	if err != nil {
		return Result{}, time.Since(started), err
	}

	evalCtx, cancel := context.WithTimeout(ctx, evaluationTimeout)
	defer cancel()
	results, err := query.Eval(evalCtx, rego.EvalInput(input))
	if err != nil {
		return Result{}, time.Since(started), fmt.Errorf("evaluate %s: %w", Entrypoint, err)
	}
	if len(results) != 1 || len(results[0].Expressions) != 1 {
		return Result{}, time.Since(started), fmt.Errorf("%s must return an object of rules", Entrypoint)
	}

	raw, err := json.Marshal(results[0].Expressions[0].Value)
	if err != nil {
		return Result{}, time.Since(started), fmt.Errorf("encode policy rules: %w", err)
	}
	if len(raw) > MaxOutputBytes {
		return Result{}, time.Since(started), fmt.Errorf("policy output exceeds %d bytes", MaxOutputBytes)
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return Result{}, time.Since(started), fmt.Errorf("%s must return an object of rules: %w", Entrypoint, err)
	}
	result := Result{
		Rules: []Rule{}, Notifications: []Notification{}, FanRuns: []FanRun{},
		Agent: AgentGuidance{Facts: []string{}, Guidance: []string{}, DenyActions: []string{}},
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := append(json.RawMessage(nil), values[name]...)
		active := true
		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			return Result{}, time.Since(started), fmt.Errorf("decode rule %q: %w", name, err)
		}
		if boolean, ok := decoded.(bool); ok {
			active = boolean
		}
		result.Rules = append(result.Rules, Rule{Name: name, Active: active, Value: value})
	}
	if err := normalizeKnownRules(&result); err != nil {
		return Result{}, time.Since(started), err
	}
	if err := result.Valid(); err != nil {
		return Result{}, time.Since(started), err
	}
	return result, time.Since(started), nil
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
	if module == nil || module.Package.Path.String() != Entrypoint {
		return rego.PreparedEvalQuery{}, fmt.Errorf("compile policy: package must be planty.v1")
	}
	for _, rule := range module.Rules {
		if len(rule.Head.Args) > 0 {
			continue
		}
		name := rule.Head.Name.String()
		if !isSupportedRule(name) {
			return rego.PreparedEvalQuery{}, fmt.Errorf("compile policy: rule %q is not supported", name)
		}
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

func isSupportedRule(name string) bool {
	if _, ok := supportedRules[name]; ok {
		return true
	}
	return strings.HasPrefix(name, "needs_") && len(name) > len("needs_")
}

func normalizeKnownRules(result *Result) error {
	for _, rule := range result.Rules {
		if !rule.Active {
			continue
		}
		switch rule.Name {
		case "health_adjustment":
			var health HealthAdjustment
			if err := decodeRuleValue(rule, &health); err != nil {
				return err
			}
			result.Health = &health
		case "notification":
			var notification Notification
			if err := decodeRuleValue(rule, &notification); err != nil {
				return err
			}
			result.Notifications = append(result.Notifications, notification)
		case "notifications":
			var notifications []Notification
			if err := decodeRuleValue(rule, &notifications); err != nil {
				return err
			}
			result.Notifications = append(result.Notifications, notifications...)
		case "fan_run":
			var run FanRun
			if err := decodeRuleValue(rule, &run); err != nil {
				return err
			}
			result.FanRuns = append(result.FanRuns, run)
		case "fan_runs":
			var runs []FanRun
			if err := decodeRuleValue(rule, &runs); err != nil {
				return err
			}
			result.FanRuns = append(result.FanRuns, runs...)
		case "agent_fact", "agent_facts":
			values, err := decodeStringValues(rule)
			if err != nil {
				return err
			}
			result.Agent.Facts = append(result.Agent.Facts, values...)
		case "agent_guidance":
			values, err := decodeStringValues(rule)
			if err != nil {
				return err
			}
			result.Agent.Guidance = append(result.Agent.Guidance, values...)
		case "deny_action", "deny_actions":
			values, err := decodeStringValues(rule)
			if err != nil {
				return err
			}
			result.Agent.DenyActions = append(result.Agent.DenyActions, values...)
		}
	}
	return nil
}

func decodeRuleValue[T any](rule Rule, target *T) error {
	decoder := json.NewDecoder(bytes.NewReader(rule.Value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("rule %q does not match its typed contract: %w", rule.Name, err)
	}
	return nil
}

func decodeStringValues(rule Rule) ([]string, error) {
	var single string
	if err := json.Unmarshal(rule.Value, &single); err == nil {
		return []string{single}, nil
	}
	var values []string
	if err := decodeRuleValue(rule, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (r Result) Valid() error {
	if len(r.Rules) > MaxOutputItems || len(r.Notifications) > MaxOutputItems ||
		len(r.FanRuns) > MaxOutputItems || len(r.Agent.Facts) > MaxOutputItems ||
		len(r.Agent.Guidance) > MaxOutputItems || len(r.Agent.DenyActions) > MaxOutputItems {
		return fmt.Errorf("policy output arrays may contain at most %d items each", MaxOutputItems)
	}
	for i, rule := range r.Rules {
		if strings.TrimSpace(rule.Name) == "" || len(rule.Name) > MaxOutputTextBytes {
			return fmt.Errorf("rules[%d].name must be non-empty and no more than %d bytes", i, MaxOutputTextBytes)
		}
		if len(rule.Value) == 0 || len(rule.Value) > MaxOutputBytes {
			return fmt.Errorf("rules[%d].value must be valid JSON no larger than %d bytes", i, MaxOutputBytes)
		}
	}
	if r.Health != nil {
		if r.Health.Delta == 0 || r.Health.Delta < -20 || r.Health.Delta > 20 {
			return fmt.Errorf("health.delta must be non-zero and between -20 and 20")
		}
		if strings.TrimSpace(r.Health.Reason) == "" {
			return fmt.Errorf("health.reason is required")
		}
		if len(r.Health.Reason) > MaxOutputTextBytes {
			return fmt.Errorf("health.reason exceeds %d bytes", MaxOutputTextBytes)
		}
	}
	for i, notification := range r.Notifications {
		if strings.TrimSpace(notification.Title) == "" || strings.TrimSpace(notification.Body) == "" {
			return fmt.Errorf("notifications[%d] needs a title and body", i)
		}
		if len(notification.Title) > MaxOutputTextBytes || len(notification.Body) > MaxOutputTextBytes {
			return fmt.Errorf("notifications[%d] text exceeds %d bytes", i, MaxOutputTextBytes)
		}
		if err := validSeverity(notification.Priority); err != nil {
			return fmt.Errorf("notifications[%d]: %w", i, err)
		}
	}
	for i, run := range r.FanRuns {
		if run.ActuatorID == uuid.Nil {
			return fmt.Errorf("fan_runs[%d].actuator_id is required", i)
		}
		if run.DurationSeconds < 1 || run.DurationSeconds > 3600 {
			return fmt.Errorf("fan_runs[%d].duration_seconds must be between 1 and 3600", i)
		}
		if strings.TrimSpace(run.Reason) == "" {
			return fmt.Errorf("fan_runs[%d].reason is required", i)
		}
		if len(run.Reason) > MaxOutputTextBytes {
			return fmt.Errorf("fan_runs[%d].reason exceeds %d bytes", i, MaxOutputTextBytes)
		}
	}
	for kind, values := range map[string][]string{
		"agent.facts": r.Agent.Facts, "agent.guidance": r.Agent.Guidance,
		"agent.deny_actions": r.Agent.DenyActions,
	} {
		for i, value := range values {
			if strings.TrimSpace(value) == "" || len(value) > MaxOutputTextBytes {
				return fmt.Errorf("%s[%d] must be non-empty and no more than %d bytes", kind, i, MaxOutputTextBytes)
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
