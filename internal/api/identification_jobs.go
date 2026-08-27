package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func (s *Server) enqueueIdentification(w http.ResponseWriter, r *http.Request) {
	if s.judge == nil {
		s.fail(w, http.StatusServiceUnavailable,
			errors.New("identification needs a judge, and none is configured"))
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	image, _, err := readImage(w, r, MaxIdentifyBytes)
	if err != nil {
		s.fail(w, statusForUpload(err), err)
		return
	}
	media := http.DetectContentType(image)
	ext, ok := photoTypes[media]
	if !ok {
		s.fail(w, http.StatusUnsupportedMediaType, ErrPhotoType)
		return
	}
	photo, err := s.keepUnownedPhoto(r.Context(), "identify/"+id.String(), image, media, ext)
	if err != nil {
		s.fail(w, statusForPhoto(err), err)
		return
	}
	queued, err := s.store.QueueIdentification(r.Context(), store.IdentificationJob{
		ID: id, PhotoID: photo.ID, Sighting: sighting(r),
	})
	if errors.Is(err, store.ErrTurnConflict) {
		s.fail(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusAccepted, identificationResponse(queued))
}

func (s *Server) getIdentification(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	job, err := s.store.Identification(r.Context(), id)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	s.ok(w, http.StatusOK, identificationResponse(job))
}

func identificationResponse(job store.IdentificationJob) map[string]any {
	response := map[string]any{"id": job.ID, "status": job.Status}
	if job.Status == store.ConsultComplete {
		response["candidates"] = job.Candidates
	}
	if job.Failure != "" {
		response["failure"] = job.Failure
	}
	return response
}
