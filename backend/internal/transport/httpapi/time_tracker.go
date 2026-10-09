package httpapi

import (
	"net/http"

	"avatar-id/internal/domain"
)

func timeTrackerPeriod(r *http.Request) string {
	return r.URL.Query().Get("period")
}

func (s *Server) getTimeTracker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	value, err := s.service.TimeTracker(r.Context(), timeTrackerPeriod(r))
	respond(w, value, err, http.StatusOK)
}

func (s *Server) createTimeActivity(w http.ResponseWriter, r *http.Request) {
	var input domain.TimeActivityInput
	if err := decodeJSON(w, r, 4_000, &input); err != nil {
		http.Error(w, "bad time activity json", http.StatusBadRequest)
		return
	}
	value, err := s.service.CreateTimeActivity(r.Context(), input)
	respond(w, value, err, http.StatusCreated)
}

func (s *Server) updateTimeActivity(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "time activity")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var input domain.TimeActivityInput
	if err := decodeJSON(w, r, 4_000, &input); err != nil {
		http.Error(w, "bad time activity json", http.StatusBadRequest)
		return
	}
	value, err := s.service.UpdateTimeActivity(r.Context(), id, input)
	respond(w, value, err, http.StatusOK)
}

func (s *Server) deleteTimeActivity(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "time activity")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.service.DeleteTimeActivity(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startTimeActivity(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "time activity")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := s.service.StartTimeActivity(r.Context(), id, timeTrackerPeriod(r))
	respond(w, value, err, http.StatusOK)
}

func (s *Server) stopTimeActivity(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.StopTimeActivity(r.Context(), timeTrackerPeriod(r))
	respond(w, value, err, http.StatusOK)
}

func (s *Server) pauseTimeActivity(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.PauseTimeActivity(r.Context(), timeTrackerPeriod(r))
	respond(w, value, err, http.StatusOK)
}
