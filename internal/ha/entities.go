package ha

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type Entity struct {
	EntityID     string `json:"entity_id"`
	FriendlyName string `json:"friendly_name"`
	Domain       string `json:"domain"`
	DeviceClass  string `json:"device_class,omitempty"`
	State        string `json:"state,omitempty"`
	Available    bool   `json:"available"`
	Area         string `json:"area,omitempty"`
}

const entityAreasTemplate = `{% set ns = namespace(items=[]) -%}
{%- for entity in states -%}
  {%- set area = area_name(entity.entity_id) -%}
  {%- if area -%}
    {%- set ns.items = ns.items + [{"entity_id": entity.entity_id, "area": area}] -%}
  {%- endif -%}
{%- endfor -%}
{{ ns.items | tojson }}`

func (c *Client) Entities(ctx context.Context) ([]Entity, error) {
	states, err := c.States(ctx)
	if err != nil {
		return nil, err
	}
	if len(states) == 0 {
		return []Entity{}, nil
	}
	areas, err := c.entityAreas(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover Home Assistant areas: %w", err)
	}
	entities := make([]Entity, 0, len(states))
	for _, state := range states {
		domain, objectID, ok := strings.Cut(state.EntityID, ".")
		if !ok || domain == "" || objectID == "" {
			continue
		}
		friendlyName := stateAttributeString(state.Attributes, "friendly_name")
		if friendlyName == "" {
			friendlyName = state.EntityID
		}
		area := areas[state.EntityID]
		if area == "" {
			area = firstStateAttributeString(state.Attributes, "area_name", "area")
		}
		entities = append(entities, Entity{
			EntityID: state.EntityID, FriendlyName: friendlyName, Domain: domain,
			DeviceClass: stateAttributeString(state.Attributes, "device_class"),
			State:       state.State, Available: stateIsAvailable(state.State), Area: area,
		})
	}
	sort.Slice(entities, func(i, j int) bool {
		left, right := strings.ToLower(entities[i].FriendlyName), strings.ToLower(entities[j].FriendlyName)
		if left == right {
			return entities[i].EntityID < entities[j].EntityID
		}
		return left < right
	})
	return entities, nil
}

func (c *Client) entityAreas(ctx context.Context) (map[string]string, error) {
	var rows []struct {
		EntityID string `json:"entity_id"`
		Area     string `json:"area"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/template", map[string]string{"template": entityAreasTemplate}, &rows); err != nil {
		return nil, err
	}
	areas := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.EntityID != "" && row.Area != "" {
			areas[row.EntityID] = row.Area
		}
	}
	return areas, nil
}

func stateAttributeString(attributes map[string]any, key string) string {
	value, _ := attributes[key].(string)
	return strings.TrimSpace(value)
}

func firstStateAttributeString(attributes map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stateAttributeString(attributes, key); value != "" {
			return value
		}
	}
	return ""
}

func stateIsAvailable(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "unknown", "unavailable":
		return false
	default:
		return true
	}
}
