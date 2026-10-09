package httpapi

import "net/http"

func (s *Server) getTimeStatistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	value, err := s.service.TimeStatistics(r.Context(), r.URL.Query().Get("period"))
	respond(w, value, err, http.StatusOK)
}

func (s *Server) getTimeActivityStatistics(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "time activity")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	value, err := s.service.TimeActivityStatistics(r.Context(), id)
	respond(w, value, err, http.StatusOK)
}
