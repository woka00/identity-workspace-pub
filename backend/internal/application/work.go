package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"avatar-id/internal/domain"
)

type workRepository interface {
	WorkProjects(context.Context) ([]domain.WorkProject, error)
	WorkProject(context.Context, int64) (domain.WorkProjectDetail, error)
	WorkProjectCore(context.Context, int64) (domain.WorkProjectDetail, error)
	WorkProjectRevision(context.Context, int64) (domain.WorkProjectRevision, error)
	WorkProjectResources(context.Context, int64) (domain.WorkProjectResources, error)
	WorkProjectResourceSummary(context.Context, int64) (domain.WorkProjectResourceSummary, error)
	CreateWorkProject(context.Context, domain.WorkProjectInput) (domain.WorkProjectDetail, error)
	UpdateWorkProject(context.Context, int64, domain.WorkProjectInput) (domain.WorkProjectDetail, error)
	DeleteWorkProject(context.Context, int64) error
	CreateWorkSection(context.Context, int64, domain.WorkSectionInput) (domain.WorkSection, error)
	UpdateWorkSection(context.Context, int64, string) (domain.WorkSection, error)
	UpdateWorkSectionCompletion(context.Context, int64, *int64, bool, bool) (domain.WorkSection, error)
	DeleteWorkSection(context.Context, int64) error
	CreateWorkTask(context.Context, int64, domain.WorkTaskInput) (domain.WorkTask, error)
	UpdateWorkTask(context.Context, int64, domain.WorkTaskInput) (domain.WorkTask, error)
	ClaimWorkTask(context.Context, int64) (domain.WorkTask, error)
	DeleteWorkTask(context.Context, int64) error
	CreateWorkComment(context.Context, int64, string) (domain.WorkComment, error)
	WorkTaskComments(context.Context, int64) ([]domain.WorkComment, error)
	CreateWorkInvite(context.Context, int64, string, time.Time) error
	RevokeWorkInvites(context.Context, int64) error
	RemoveWorkMember(context.Context, int64, int64) error
	LeaveWorkProject(context.Context, int64) error
	AcceptWorkInvite(context.Context, string, time.Time) (domain.WorkProjectDetail, error)
	CreateWorkAttachment(context.Context, int64, *int64, string, string, []byte) (domain.WorkAttachment, error)
	WorkAttachment(context.Context, int64) (domain.WorkAttachmentData, error)
	DeleteWorkAttachment(context.Context, int64) error
	CreateWorkLink(context.Context, int64, domain.WorkLinkInput) (domain.WorkLink, error)
	DeleteWorkLink(context.Context, int64) error
	CreateWorkNote(context.Context, int64, domain.WorkNoteInput) (domain.WorkNote, error)
	WorkNote(context.Context, int64) (domain.WorkNote, error)
	UpdateWorkNote(context.Context, int64, domain.WorkNoteInput) (domain.WorkNote, error)
	DeleteWorkNote(context.Context, int64) error
}

const (
	MaxWorkAttachmentBytes    = 50 << 20
	MaxWorkProjectAttachments = 500
	MaxWorkProjectLinks       = 1000
	MaxWorkProjectNotes       = 1000
	MaxWorkTaskComments       = 2000
)

func (s *Service) workRepo() (workRepository, error) {
	repo, ok := s.repo.(workRepository)
	if !ok {
		return nil, fmt.Errorf("work projects are unavailable")
	}
	return repo, nil
}

func (s *Service) WorkProjects(ctx context.Context) ([]domain.WorkProject, error) {
	r, err := s.workRepo()
	if err != nil {
		return nil, err
	}
	return r.WorkProjects(ctx)
}
func (s *Service) WorkProject(ctx context.Context, id int64) (domain.WorkProjectDetail, error) {
	if id <= 0 {
		return domain.WorkProjectDetail{}, invalidf("invalid project id")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	return r.WorkProject(ctx, id)
}
func (s *Service) WorkProjectCore(ctx context.Context, id int64) (domain.WorkProjectDetail, error) {
	if id <= 0 {
		return domain.WorkProjectDetail{}, invalidf("invalid project id")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	return r.WorkProjectCore(ctx, id)
}
func (s *Service) WorkProjectRevision(ctx context.Context, id int64) (domain.WorkProjectRevision, error) {
	if id <= 0 {
		return domain.WorkProjectRevision{}, invalidf("invalid project id")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkProjectRevision{}, err
	}
	return r.WorkProjectRevision(ctx, id)
}
func (s *Service) WorkProjectResources(ctx context.Context, id int64) (domain.WorkProjectResources, error) {
	if id <= 0 {
		return domain.WorkProjectResources{}, invalidf("invalid project id")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkProjectResources{}, err
	}
	return r.WorkProjectResources(ctx, id)
}
func (s *Service) WorkProjectResourceSummary(ctx context.Context, id int64) (domain.WorkProjectResourceSummary, error) {
	if id <= 0 {
		return domain.WorkProjectResourceSummary{}, invalidf("invalid project id")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkProjectResourceSummary{}, err
	}
	return r.WorkProjectResourceSummary(ctx, id)
}
func normalizeWorkProject(in domain.WorkProjectInput) (domain.WorkProjectInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Summary = strings.TrimSpace(in.Summary)
	if in.Title == "" || len([]rune(in.Title)) > 160 {
		return in, invalidf("название проекта должно содержать 1–160 символов")
	}
	if len([]rune(in.Description)) > 20000 || len([]rune(in.Summary)) > 2000 {
		return in, invalidf("текст проекта слишком длинный")
	}
	if in.Status == "" {
		in.Status = "on_track"
	}
	if in.Status != "on_track" && in.Status != "at_risk" && in.Status != "off_track" {
		return in, invalidf("неизвестный статус проекта")
	}
	return in, nil
}
func (s *Service) CreateWorkProject(ctx context.Context, in domain.WorkProjectInput) (domain.WorkProjectDetail, error) {
	in, err := normalizeWorkProject(in)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	return r.CreateWorkProject(ctx, in)
}
func (s *Service) UpdateWorkProject(ctx context.Context, id int64, in domain.WorkProjectInput) (domain.WorkProjectDetail, error) {
	if id <= 0 || in.Version <= 0 {
		return domain.WorkProjectDetail{}, invalidf("invalid project version")
	}
	in, err := normalizeWorkProject(in)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	return r.UpdateWorkProject(ctx, id, in)
}
func (s *Service) DeleteWorkProject(ctx context.Context, id int64) error {
	r, err := s.workRepo()
	if err != nil {
		return err
	}
	return r.DeleteWorkProject(ctx, id)
}
func normalizeWorkSectionTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 120 {
		return "", invalidf("название раздела должно содержать 1–120 символов")
	}
	return title, nil
}

func (s *Service) CreateWorkSection(ctx context.Context, projectID int64, in domain.WorkSectionInput) (domain.WorkSection, error) {
	var err error
	in.Title, err = normalizeWorkSectionTitle(in.Title)
	if err != nil {
		return domain.WorkSection{}, err
	}
	if in.RelativeTo == nil && in.Placement != "" || in.RelativeTo != nil && in.Placement != "above" && in.Placement != "below" {
		return domain.WorkSection{}, invalidf("некорректное положение раздела")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkSection{}, err
	}
	return r.CreateWorkSection(ctx, projectID, in)
}

func (s *Service) UpdateWorkSection(ctx context.Context, id int64, title string) (domain.WorkSection, error) {
	if id <= 0 {
		return domain.WorkSection{}, invalidf("invalid section id")
	}
	title, err := normalizeWorkSectionTitle(title)
	if err != nil {
		return domain.WorkSection{}, err
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkSection{}, err
	}
	return r.UpdateWorkSection(ctx, id, title)
}

func (s *Service) UpdateWorkSectionCompletion(ctx context.Context, id int64, sectionID *int64, resetCompletedOnMove, moveCompletedToEnd bool) (domain.WorkSection, error) {
	if id <= 0 || sectionID != nil && (*sectionID <= 0 || *sectionID == id) {
		return domain.WorkSection{}, invalidf("некорректный раздел назначения")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkSection{}, err
	}
	return r.UpdateWorkSectionCompletion(ctx, id, sectionID, resetCompletedOnMove, moveCompletedToEnd)
}

func (s *Service) DeleteWorkSection(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalidf("invalid section id")
	}
	r, err := s.workRepo()
	if err != nil {
		return err
	}
	return r.DeleteWorkSection(ctx, id)
}
func normalizeWorkTask(in domain.WorkTaskInput) (domain.WorkTaskInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if in.Title == "" || len([]rune(in.Title)) > 300 {
		return in, invalidf("название задачи должно содержать 1–300 символов")
	}
	if len([]rune(in.Description)) > 30000 {
		return in, invalidf("описание задачи слишком длинное")
	}
	return in, nil
}
func (s *Service) CreateWorkTask(ctx context.Context, projectID int64, in domain.WorkTaskInput) (domain.WorkTask, error) {
	in, err := normalizeWorkTask(in)
	if err != nil {
		return domain.WorkTask{}, err
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkTask{}, err
	}
	return r.CreateWorkTask(ctx, projectID, in)
}
func (s *Service) UpdateWorkTask(ctx context.Context, id int64, in domain.WorkTaskInput) (domain.WorkTask, error) {
	if in.Version <= 0 {
		return domain.WorkTask{}, invalidf("invalid task version")
	}
	in, err := normalizeWorkTask(in)
	if err != nil {
		return domain.WorkTask{}, err
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkTask{}, err
	}
	return r.UpdateWorkTask(ctx, id, in)
}
func (s *Service) ClaimWorkTask(ctx context.Context, id int64) (domain.WorkTask, error) {
	if id <= 0 {
		return domain.WorkTask{}, invalidf("invalid task id")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkTask{}, err
	}
	return r.ClaimWorkTask(ctx, id)
}
func (s *Service) DeleteWorkTask(ctx context.Context, id int64) error {
	r, err := s.workRepo()
	if err != nil {
		return err
	}
	return r.DeleteWorkTask(ctx, id)
}
func (s *Service) CreateWorkComment(ctx context.Context, taskID int64, body string) (domain.WorkComment, error) {
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 10000 {
		return domain.WorkComment{}, invalidf("комментарий должен содержать 1–10000 символов")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkComment{}, err
	}
	return r.CreateWorkComment(ctx, taskID, body)
}
func (s *Service) WorkTaskComments(ctx context.Context, taskID int64) ([]domain.WorkComment, error) {
	if taskID <= 0 {
		return nil, invalidf("invalid task id")
	}
	r, err := s.workRepo()
	if err != nil {
		return nil, err
	}
	return r.WorkTaskComments(ctx, taskID)
}
func (s *Service) CreateWorkInvite(ctx context.Context, projectID int64) (domain.WorkInvite, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return domain.WorkInvite{}, err
	}
	token := hex.EncodeToString(raw)
	expires := s.now().Add(7 * 24 * time.Hour)
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkInvite{}, err
	}
	if err := r.CreateWorkInvite(ctx, projectID, token, expires); err != nil {
		return domain.WorkInvite{}, err
	}
	return domain.WorkInvite{Token: token, ExpiresAt: expires.Format(time.RFC3339)}, nil
}
func (s *Service) AcceptWorkInvite(ctx context.Context, token string) (domain.WorkProjectDetail, error) {
	token = strings.TrimSpace(token)
	if len(token) != 64 {
		return domain.WorkProjectDetail{}, invalidf("invalid invite token")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	return r.AcceptWorkInvite(ctx, token, s.now())
}
func (s *Service) RevokeWorkInvites(ctx context.Context, projectID int64) error {
	r, err := s.workRepo()
	if err != nil {
		return err
	}
	return r.RevokeWorkInvites(ctx, projectID)
}
func (s *Service) RemoveWorkMember(ctx context.Context, projectID, userID int64) error {
	if projectID <= 0 || userID <= 0 {
		return invalidf("invalid member")
	}
	r, err := s.workRepo()
	if err != nil {
		return err
	}
	return r.RemoveWorkMember(ctx, projectID, userID)
}
func (s *Service) LeaveWorkProject(ctx context.Context, projectID int64) error {
	r, err := s.workRepo()
	if err != nil {
		return err
	}
	return r.LeaveWorkProject(ctx, projectID)
}
func (s *Service) CreateWorkAttachment(ctx context.Context, projectID int64, taskID *int64, name, mimeType string, content []byte) (domain.WorkAttachment, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 255 {
		return domain.WorkAttachment{}, invalidf("invalid attachment name")
	}
	if len(content) == 0 || len(content) > MaxWorkAttachmentBytes {
		return domain.WorkAttachment{}, invalidf("размер файла должен быть от 1 байта до 50 МБ")
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkAttachment{}, err
	}
	return r.CreateWorkAttachment(ctx, projectID, taskID, name, mimeType, content)
}
func (s *Service) WorkAttachment(ctx context.Context, id int64) (domain.WorkAttachmentData, error) {
	if id <= 0 {
		return domain.WorkAttachmentData{}, invalidf("invalid attachment id")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkAttachmentData{}, err
	}
	return r.WorkAttachment(ctx, id)
}

func (s *Service) DeleteWorkAttachment(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalidf("invalid attachment id")
	}
	r, err := s.workRepo()
	if err != nil {
		return err
	}
	return r.DeleteWorkAttachment(ctx, id)
}

func normalizeWorkLink(in domain.WorkLinkInput) (domain.WorkLinkInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.URL = strings.TrimSpace(in.URL)
	parsed, err := url.ParseRequestURI(in.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return in, invalidf("ссылка должна начинаться с http:// или https://")
	}
	if in.Title == "" {
		in.Title = parsed.Host
	}
	if len([]rune(in.Title)) > 200 || len([]rune(in.URL)) > 2048 {
		return in, invalidf("ссылка слишком длинная")
	}
	return in, nil
}

func (s *Service) CreateWorkLink(ctx context.Context, projectID int64, in domain.WorkLinkInput) (domain.WorkLink, error) {
	if projectID <= 0 {
		return domain.WorkLink{}, invalidf("invalid project id")
	}
	in, err := normalizeWorkLink(in)
	if err != nil {
		return domain.WorkLink{}, err
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkLink{}, err
	}
	return r.CreateWorkLink(ctx, projectID, in)
}

func (s *Service) DeleteWorkLink(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalidf("invalid link id")
	}
	r, err := s.workRepo()
	if err != nil {
		return err
	}
	return r.DeleteWorkLink(ctx, id)
}

func normalizeWorkNote(in domain.WorkNoteInput) (domain.WorkNoteInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	if in.Title == "" || len([]rune(in.Title)) > 160 {
		return in, invalidf("название заметки должно содержать 1–160 символов")
	}
	if len([]rune(in.Body)) > 100000 {
		return in, invalidf("текст заметки не должен превышать 100 000 символов")
	}
	return in, nil
}

func (s *Service) CreateWorkNote(ctx context.Context, projectID int64, in domain.WorkNoteInput) (domain.WorkNote, error) {
	if projectID <= 0 {
		return domain.WorkNote{}, invalidf("invalid project id")
	}
	in, err := normalizeWorkNote(in)
	if err != nil {
		return domain.WorkNote{}, err
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkNote{}, err
	}
	return r.CreateWorkNote(ctx, projectID, in)
}

func (s *Service) WorkNote(ctx context.Context, id int64) (domain.WorkNote, error) {
	if id <= 0 {
		return domain.WorkNote{}, invalidf("invalid note id")
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkNote{}, err
	}
	return r.WorkNote(ctx, id)
}

func (s *Service) UpdateWorkNote(ctx context.Context, id int64, in domain.WorkNoteInput) (domain.WorkNote, error) {
	if id <= 0 || in.Version <= 0 {
		return domain.WorkNote{}, invalidf("invalid note version")
	}
	in, err := normalizeWorkNote(in)
	if err != nil {
		return domain.WorkNote{}, err
	}
	r, err := s.workRepo()
	if err != nil {
		return domain.WorkNote{}, err
	}
	return r.UpdateWorkNote(ctx, id, in)
}

func (s *Service) DeleteWorkNote(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalidf("invalid note id")
	}
	r, err := s.workRepo()
	if err != nil {
		return err
	}
	return r.DeleteWorkNote(ctx, id)
}
