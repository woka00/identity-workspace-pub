package httpapi

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

func (s *Server) getWorkProjects(w http.ResponseWriter, r *http.Request) {
	v, err := s.service.WorkProjects(r.Context())
	respond(w, v, err, http.StatusOK)
}
func (s *Server) getWorkProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	v, err := s.service.WorkProject(r.Context(), id)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) getWorkProjectCore(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	v, err := s.service.WorkProjectCore(r.Context(), id)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) getWorkProjectRevision(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	v, err := s.service.WorkProjectRevision(r.Context(), id)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) getWorkProjectResources(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	v, err := s.service.WorkProjectResources(r.Context(), id)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) getWorkProjectResourceSummary(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	v, err := s.service.WorkProjectResourceSummary(r.Context(), id)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) createWorkProject(w http.ResponseWriter, r *http.Request) {
	var in domain.WorkProjectInput
	if err := decodeJSON(w, r, 1<<20, &in); err != nil {
		writeError(w, domain.InvalidInputError{Message: "некорректные данные проекта"})
		return
	}
	v, err := s.service.CreateWorkProject(r.Context(), in)
	respond(w, v, err, http.StatusCreated)
}
func (s *Server) updateWorkProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	var in domain.WorkProjectInput
	if err := decodeJSON(w, r, 1<<20, &in); err != nil {
		writeError(w, domain.InvalidInputError{Message: "некорректные данные проекта"})
		return
	}
	v, err := s.service.UpdateWorkProject(r.Context(), id, in)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) deleteWorkProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err == nil {
		err = s.service.DeleteWorkProject(r.Context(), id)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) createWorkSection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	var in domain.WorkSectionInput
	if err := decodeJSON(w, r, 32<<10, &in); err != nil {
		writeError(w, domain.InvalidInputError{Message: "некорректный раздел"})
		return
	}
	v, err := s.service.CreateWorkSection(r.Context(), id, in)
	respond(w, v, err, http.StatusCreated)
}
func (s *Server) updateWorkSection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "section")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	var in domain.WorkSectionInput
	if err := decodeJSON(w, r, 32<<10, &in); err != nil {
		writeError(w, domain.InvalidInputError{Message: "некорректный раздел"})
		return
	}
	v, err := s.service.UpdateWorkSection(r.Context(), id, in.Title)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) updateWorkSectionCompletion(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "section")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	var in domain.WorkSectionCompletionInput
	if err := decodeJSON(w, r, 32<<10, &in); err != nil {
		writeError(w, domain.InvalidInputError{Message: "некорректный раздел назначения"})
		return
	}
	v, err := s.service.UpdateWorkSectionCompletion(r.Context(), id, in.SectionID, in.ResetCompletedOnMove, in.MoveCompletedToEnd)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) deleteWorkSection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "section")
	if err == nil {
		err = s.service.DeleteWorkSection(r.Context(), id)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) createWorkTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	var in domain.WorkTaskInput
	if err := decodeJSON(w, r, 1<<20, &in); err != nil {
		writeError(w, domain.InvalidInputError{Message: "некорректная задача"})
		return
	}
	v, err := s.service.CreateWorkTask(r.Context(), id, in)
	respond(w, v, err, http.StatusCreated)
}
func (s *Server) updateWorkTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "task")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	var in domain.WorkTaskInput
	if err := decodeJSON(w, r, 1<<20, &in); err != nil {
		writeError(w, domain.InvalidInputError{Message: "некорректная задача"})
		return
	}
	v, err := s.service.UpdateWorkTask(r.Context(), id, in)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) claimWorkTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "task")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	v, err := s.service.ClaimWorkTask(r.Context(), id)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) deleteWorkTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "task")
	if err == nil {
		err = s.service.DeleteWorkTask(r.Context(), id)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) createWorkComment(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "task")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(w, r, 64<<10, &in); err != nil {
		writeError(w, domain.InvalidInputError{Message: "некорректный комментарий"})
		return
	}
	v, err := s.service.CreateWorkComment(r.Context(), id, in.Body)
	respond(w, v, err, http.StatusCreated)
}
func (s *Server) getWorkTaskComments(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "task")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	v, err := s.service.WorkTaskComments(r.Context(), id)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) createWorkInvite(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	v, err := s.service.CreateWorkInvite(r.Context(), id)
	if err == nil {
		v.Token = workInviteURL(s.config.PublicURL, v.Token)
	}
	respond(w, v, err, http.StatusCreated)
}

func workInviteURL(publicURL, token string) string {
	return strings.TrimRight(publicURL, "/") + "/#work_invite=" + token
}
func (s *Server) revokeWorkInvites(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err == nil {
		err = s.service.RevokeWorkInvites(r.Context(), id)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) removeWorkMember(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	userID, e := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if e != nil || userID <= 0 {
		writeError(w, domain.InvalidInputError{Message: "invalid member id"})
		return
	}
	err = s.service.RemoveWorkMember(r.Context(), id, userID)
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) leaveWorkProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err == nil {
		err = s.service.LeaveWorkProject(r.Context(), id)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) acceptWorkInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		var input struct {
			Token string `json:"token"`
		}
		if err := decodeJSON(w, r, 1024, &input); err != nil {
			writeError(w, domain.InvalidInputError{Message: "некорректный токен приглашения"})
			return
		}
		token = input.Token
	}
	v, err := s.service.AcceptWorkInvite(r.Context(), token)
	respond(w, v, err, http.StatusOK)
}
func (s *Server) createWorkAttachment(w http.ResponseWriter, r *http.Request) {
	select {
	case s.attachmentUploadSlots <- struct{}{}:
		defer func() { <-s.attachmentUploadSlots }()
	case <-r.Context().Done():
		return
	default:
		w.Header().Set("Retry-After", "30")
		http.Error(w, "сервер уже обрабатывает несколько файлов; повторите загрузку через минуту", http.StatusServiceUnavailable)
		return
	}
	projectID, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, application.MaxWorkAttachmentBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, domain.InvalidInputError{Message: "Файл больше 50 МБ. Добавьте ссылку на Яндекс Диск или Google Диск."})
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: "файл не выбран"})
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, application.MaxWorkAttachmentBytes+1))
	if err != nil || len(content) > application.MaxWorkAttachmentBytes {
		writeError(w, domain.InvalidInputError{Message: "Файл больше 50 МБ. Добавьте ссылку на Яндекс Диск или Google Диск."})
		return
	}
	var taskID *int64
	if raw := r.FormValue("taskId"); raw != "" {
		v, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || v <= 0 {
			writeError(w, domain.InvalidInputError{Message: "invalid task id"})
			return
		}
		taskID = &v
	}
	mimeType := header.Header.Get("Content-Type")
	v, err := s.service.CreateWorkAttachment(r.Context(), projectID, taskID, header.Filename, mimeType, content)
	respond(w, v, err, http.StatusCreated)
}

func (s *Server) deleteWorkAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "attachment")
	if err == nil {
		err = s.service.DeleteWorkAttachment(r.Context(), id)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) downloadWorkAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "attachment")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	v, err := s.service.WorkAttachment(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", v.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(v.Size, 10))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", strings.ReplaceAll(mime.QEncoding.Encode("UTF-8", v.Name), " ", "%20")))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(v.Content)
}

func (s *Server) createWorkLink(w http.ResponseWriter, r *http.Request) {
	projectID, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	var in domain.WorkLinkInput
	if err := decodeJSON(w, r, 16<<10, &in); err != nil {
		writeError(w, domain.InvalidInputError{Message: "некорректная ссылка"})
		return
	}
	v, err := s.service.CreateWorkLink(r.Context(), projectID, in)
	respond(w, v, err, http.StatusCreated)
}

func (s *Server) deleteWorkLink(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "link")
	if err == nil {
		err = s.service.DeleteWorkLink(r.Context(), id)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createWorkNote(w http.ResponseWriter, r *http.Request) {
	projectID, err := pathID(r, "project")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	var in domain.WorkNoteInput
	if err := decodeJSON(w, r, 1<<20, &in); err != nil {
		writeError(w, domain.InvalidInputError{Message: "некорректная заметка"})
		return
	}
	v, err := s.service.CreateWorkNote(r.Context(), projectID, in)
	respond(w, v, err, http.StatusCreated)
}

func (s *Server) getWorkNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "note")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	v, err := s.service.WorkNote(r.Context(), id)
	respond(w, v, err, http.StatusOK)
}

func (s *Server) updateWorkNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "note")
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: err.Error()})
		return
	}
	var in domain.WorkNoteInput
	if err := decodeJSON(w, r, 1<<20, &in); err != nil {
		writeError(w, domain.InvalidInputError{Message: "некорректная заметка"})
		return
	}
	v, err := s.service.UpdateWorkNote(r.Context(), id, in)
	respond(w, v, err, http.StatusOK)
}

func (s *Server) deleteWorkNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "note")
	if err == nil {
		err = s.service.DeleteWorkNote(r.Context(), id)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
