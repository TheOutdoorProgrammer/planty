package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

type homeAssistantDiscoverer interface {
	Entities(context.Context) ([]ha.Entity, error)
}

func (s *Server) WithHomeAssistant(client homeAssistantDiscoverer) *Server {
	s.homeAssistant = client
	return s
}

func (s *Server) discoverHomeAssistantEntities(w http.ResponseWriter, r *http.Request) {
	if s.homeAssistant == nil {
		s.fail(w, http.StatusServiceUnavailable, errors.New("Home Assistant discovery is not configured"))
		return
	}
	role, err := discoveryRole(r.URL.Query().Get("role"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	entities, err := s.homeAssistant.Entities(r.Context())
	if err != nil {
		s.log.Warn("Home Assistant discovery failed", "error", err)
		s.fail(w, http.StatusBadGateway, errors.New("Home Assistant discovery failed"))
		return
	}
	entities = filterDiscoveredEntities(entities, role, r.URL.Query().Get("q"))
	w.Header().Set("Cache-Control", "no-store")
	s.ok(w, http.StatusOK, map[string]any{"entities": entities, "count": len(entities)})
}

func discoveryRole(raw string) (plant.SensorRole, error) {
	role := plant.SensorRole(strings.TrimSpace(raw))
	switch role {
	case "", plant.RoleSoilMoisture, plant.RoleAmbientTemp, plant.RoleAmbientHumidity, plant.RoleIlluminance:
		return role, nil
	default:
		return "", errors.New("role must be soil_moisture, ambient_temp, ambient_humidity, or illuminance")
	}
}

func filterDiscoveredEntities(entities []ha.Entity, role plant.SensorRole, query string) []ha.Entity {
	query = strings.ToLower(strings.TrimSpace(query))
	filtered := make([]ha.Entity, 0, len(entities))
	for _, entity := range entities {
		if entity.Domain != "sensor" { continue }
		if role != "" && !entityLikelyForRole(entity, role) { continue }
		if query != "" && !entityMatchesQuery(entity, query) { continue }
		filtered = append(filtered, entity)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Available != filtered[j].Available { return filtered[i].Available }
		left, right := strings.ToLower(filtered[i].FriendlyName), strings.ToLower(filtered[j].FriendlyName)
		if left == right { return filtered[i].EntityID < filtered[j].EntityID }
		return left < right
	})
	return filtered
}

func entityMatchesQuery(entity ha.Entity, query string) bool {
	for _, value := range []string{entity.EntityID, entity.FriendlyName, entity.Domain, entity.DeviceClass, entity.Area} {
		if strings.Contains(strings.ToLower(value), query) { return true }
	}
	return false
}

func entityLikelyForRole(entity ha.Entity, role plant.SensorRole) bool {
	deviceClass := strings.ToLower(entity.DeviceClass)
	tokens := entityDiscoveryTokens(entity)
	switch role {
	case plant.RoleSoilMoisture:
		return deviceClass == "moisture" || hasAnyToken(tokens, "soil", "moisture", "wetness", "substrate", "miflora")
	case plant.RoleAmbientTemp:
		return deviceClass == "temperature" || hasAnyToken(tokens, "temperature", "temp")
	case plant.RoleAmbientHumidity:
		return deviceClass == "humidity" || hasAnyToken(tokens, "humidity", "humid")
	case plant.RoleIlluminance:
		return deviceClass == "illuminance" || hasAnyToken(tokens, "illuminance", "lux", "light", "brightness")
	default:
		return false
	}
}

func entityDiscoveryTokens(entity ha.Entity) map[string]struct{} {
	text := strings.Join([]string{entity.EntityID, entity.FriendlyName, entity.DeviceClass}, " ")
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	tokens := make(map[string]struct{}, len(fields))
	for _, field := range fields { tokens[field] = struct{}{} }
	return tokens
}

func hasAnyToken(tokens map[string]struct{}, wanted ...string) bool {
	for _, token := range wanted { if _, ok := tokens[token]; ok { return true } }
	return false
}
