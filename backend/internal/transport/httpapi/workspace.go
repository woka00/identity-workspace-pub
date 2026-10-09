package httpapi

import (
	"net/http"

	"avatar-id/internal/domain"
)

func (s *Server) updateWorkspacePreferences(w http.ResponseWriter, r *http.Request) {
	var body domain.WorkspacePreferences
	if err := decodeJSON(w, r, 2_000, &body); err != nil {
		http.Error(w, "bad workspace preferences json", http.StatusBadRequest)
		return
	}
	if err := s.service.UpdateWorkspacePreferences(r.Context(), body); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
