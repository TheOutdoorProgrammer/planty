package api

import "github.com/TheOutdoorProgrammer/planty/internal/plant"

func sourceOrApp(source plant.Source) plant.Source {
	if source == "" {
		return plant.SourceApp
	}
	return source
}
