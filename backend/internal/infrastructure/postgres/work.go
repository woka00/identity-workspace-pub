package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

const workPublicNameSQL = `CASE WHEN u.id IS NULL THEN '' ELSE COALESCE(NULLIF(p.work_display_name,''),u.login) END`
const workPublicAvatarSQL = `CASE WHEN u.id IS NULL OR NOT COALESCE(p.work_show_avatar,TRUE) THEN '' ELSE COALESCE(NULLIF(p.work_avatar,''),NULLIF(p.photo,''),'') END`

const workProjectSelect = `
SELECT p.id,p.title,p.description,p.summary,p.status,
       COALESCE(to_char(p.deadline,'YYYY-MM-DD'),''),p.completed_at IS NOT NULL,
       COALESCE(to_char(p.completed_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),''),m.pinned,
       p.owner_id,m.role,
       COUNT(t.id) FILTER (WHERE t.parent_id IS NULL),
       COUNT(t.id) FILTER (WHERE t.parent_id IS NULL AND t.completed_at IS NOT NULL),
       CASE WHEN COUNT(t.id) FILTER (WHERE t.parent_id IS NULL)=0 THEN 0
            ELSE round(100.0*COUNT(t.id) FILTER (WHERE t.parent_id IS NULL AND t.completed_at IS NOT NULL)/COUNT(t.id) FILTER (WHERE t.parent_id IS NULL))::int END,
       p.content_revision,p.version,to_char(p.created_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),to_char(p.updated_at,'YYYY-MM-DD"T"HH24:MI:SSOF')
FROM work_projects p JOIN work_project_members m ON m.project_id=p.id
LEFT JOIN work_tasks t ON t.project_id=p.id`

func scanWorkProject(row interface{ Scan(...any) error }) (domain.WorkProject, error) {
	var p domain.WorkProject
	err := row.Scan(&p.ID, &p.Title, &p.Description, &p.Summary, &p.Status, &p.Deadline, &p.Completed, &p.CompletedAt, &p.Pinned, &p.OwnerID, &p.Role, &p.TaskCount, &p.CompletedCount, &p.CompletionPct, &p.ContentRevision, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (s *Repository) WorkProjects(ctx context.Context) ([]domain.WorkProject, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, workProjectSelect+` WHERE m.user_id=$1 GROUP BY p.id,m.pinned,m.role ORDER BY p.completed_at NULLS FIRST,p.updated_at DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WorkProject{}
	byID := make(map[int64]int)
	for rows.Next() {
		p, err := scanWorkProject(rows)
		if err != nil {
			return nil, err
		}
		p.Members = []domain.WorkMember{}
		byID[p.ID] = len(out)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	memberRows, err := s.db.QueryContext(ctx, `SELECT m.project_id,m.user_id,`+workPublicNameSQL+`,`+workPublicAvatarSQL+`,m.role FROM work_project_members m JOIN users u ON u.id=m.user_id LEFT JOIN user_profiles p ON p.user_id=u.id WHERE m.project_id IN (SELECT project_id FROM work_project_members WHERE user_id=$1) ORDER BY m.project_id,m.role DESC,m.joined_at`, uid)
	if err != nil {
		return nil, err
	}
	defer memberRows.Close()
	for memberRows.Next() {
		var projectID int64
		var member domain.WorkMember
		if err := memberRows.Scan(&projectID, &member.UserID, &member.Name, &member.Avatar, &member.Role); err != nil {
			return nil, err
		}
		if index, ok := byID[projectID]; ok {
			out[index].Members = append(out[index].Members, member)
		}
	}
	return out, memberRows.Err()
}
func (s *Repository) workMembers(ctx context.Context, projectID int64) ([]domain.WorkMember, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.user_id,`+workPublicNameSQL+`,`+workPublicAvatarSQL+`,m.role FROM work_project_members m JOIN users u ON u.id=m.user_id LEFT JOIN user_profiles p ON p.user_id=u.id WHERE m.project_id=$1 ORDER BY m.role DESC,m.joined_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WorkMember{}
	for rows.Next() {
		var m domain.WorkMember
		if err := rows.Scan(&m.UserID, &m.Name, &m.Avatar, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Repository) workProject(ctx context.Context, projectID, userID int64) (domain.WorkProject, error) {
	p, err := scanWorkProject(s.db.QueryRowContext(ctx, workProjectSelect+` WHERE p.id=$1 AND m.user_id=$2 GROUP BY p.id,m.pinned,m.role`, projectID, userID))
	if err == sql.ErrNoRows {
		return p, domain.ErrNotFound
	}
	if err != nil {
		return p, err
	}
	p.Members, err = s.workMembers(ctx, projectID)
	return p, err
}
func (s *Repository) WorkProject(ctx context.Context, id int64) (domain.WorkProjectDetail, error) {
	d, err := s.WorkProjectCore(ctx, id)
	if err != nil {
		return d, err
	}
	resources, err := s.workProjectResources(ctx, id, d.Project.ContentRevision)
	if err != nil {
		return d, err
	}
	d.Comments = resources.Comments
	d.Attachments = resources.Attachments
	d.Links = resources.Links
	d.Notes = resources.Notes
	d.Events = resources.Events
	return d, nil
}

func (s *Repository) WorkProjectCore(ctx context.Context, id int64) (domain.WorkProjectDetail, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	p, err := s.workProject(ctx, id, uid)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	d := domain.WorkProjectDetail{Project: p, Sections: []domain.WorkSection{}, Tasks: []domain.WorkTask{}, Comments: []domain.WorkComment{}, Attachments: []domain.WorkAttachment{}, Links: []domain.WorkLink{}, Notes: []domain.WorkNote{}, Events: []domain.WorkEvent{}}
	rows, err := s.db.QueryContext(ctx, `SELECT id,project_id,title,sort_order,completed_section_id,reset_completed_on_move,move_completed_to_end FROM work_sections WHERE project_id=$1 ORDER BY sort_order,id`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v domain.WorkSection
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Title, &v.SortOrder, &v.CompletedSectionID, &v.ResetCompletedOnMove, &v.MoveCompletedToEnd); err != nil {
			rows.Close()
			return d, err
		}
		d.Sections = append(d.Sections, v)
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT t.id,t.project_id,t.section_id,t.parent_id,t.title,t.description,t.created_by,t.assignee_id,`+workPublicNameSQL+`,COALESCE(to_char(t.due_date,'YYYY-MM-DD'),''),t.completed_at IS NOT NULL,t.sort_order,t.version,to_char(t.created_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),to_char(t.updated_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_tasks t LEFT JOIN users u ON u.id=t.assignee_id LEFT JOIN user_profiles p ON p.user_id=u.id WHERE t.project_id=$1 ORDER BY t.parent_id NULLS FIRST,t.sort_order,t.id`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v domain.WorkTask
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.SectionID, &v.ParentID, &v.Title, &v.Description, &v.CreatedByID, &v.AssigneeID, &v.Assignee, &v.DueDate, &v.Completed, &v.SortOrder, &v.Version, &v.CreatedAt, &v.UpdatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Tasks = append(d.Tasks, v)
	}
	rows.Close()
	return d, nil
}

func (s *Repository) WorkProjectRevision(ctx context.Context, id int64) (domain.WorkProjectRevision, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkProjectRevision{}, err
	}
	var revision domain.WorkProjectRevision
	err = s.db.QueryRowContext(ctx, `SELECT p.content_revision FROM work_projects p JOIN work_project_members m ON m.project_id=p.id WHERE p.id=$1 AND m.user_id=$2`, id, uid).Scan(&revision.Revision)
	if err == sql.ErrNoRows {
		return revision, domain.ErrNotFound
	}
	return revision, err
}

func (s *Repository) WorkProjectResources(ctx context.Context, id int64) (domain.WorkProjectResources, error) {
	revision, err := s.WorkProjectRevision(ctx, id)
	if err != nil {
		return domain.WorkProjectResources{}, err
	}
	return s.workProjectResources(ctx, id, revision.Revision)
}

func (s *Repository) WorkProjectResourceSummary(ctx context.Context, id int64) (domain.WorkProjectResourceSummary, error) {
	revision, err := s.WorkProjectRevision(ctx, id)
	if err != nil {
		return domain.WorkProjectResourceSummary{}, err
	}
	d := domain.WorkProjectResourceSummary{Revision: revision.Revision, Attachments: []domain.WorkAttachment{}, Links: []domain.WorkLink{}, Notes: []domain.WorkNoteSummary{}, Events: []domain.WorkEvent{}}
	rows, err := s.db.QueryContext(ctx, `SELECT id,task_id,project_id,name,mime_type,size_bytes,to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_attachments WHERE project_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v domain.WorkAttachment
		if err := rows.Scan(&v.ID, &v.TaskID, &v.ProjectID, &v.Name, &v.MimeType, &v.Size, &v.CreatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Attachments = append(d.Attachments, v)
	}
	if err := rows.Close(); err != nil {
		return d, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT id,project_id,title,url,to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_links WHERE project_id=$1 ORDER BY created_at,id`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v domain.WorkLink
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Title, &v.URL, &v.CreatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Links = append(d.Links, v)
	}
	if err := rows.Close(); err != nil {
		return d, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT id,project_id,title,preview,version,to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_notes WHERE project_id=$1 ORDER BY updated_at DESC,id DESC`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v domain.WorkNoteSummary
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Title, &v.Preview, &v.Version, &v.CreatedAt, &v.UpdatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Notes = append(d.Notes, v)
	}
	if err := rows.Close(); err != nil {
		return d, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT e.id,e.actor_id,COALESCE(NULLIF(`+workPublicNameSQL+`,''),'Система'),`+workPublicAvatarSQL+`,e.kind,e.message,to_char(e.created_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_events e LEFT JOIN users u ON u.id=e.actor_id LEFT JOIN user_profiles p ON p.user_id=u.id WHERE e.project_id=$1 ORDER BY e.created_at DESC,e.id DESC LIMIT 100`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v domain.WorkEvent
		if err := rows.Scan(&v.ID, &v.ActorID, &v.Actor, &v.ActorAvatar, &v.Kind, &v.Message, &v.CreatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Events = append(d.Events, v)
	}
	if err := rows.Close(); err != nil {
		return d, err
	}
	return d, nil
}

func (s *Repository) workProjectResources(ctx context.Context, id, revision int64) (domain.WorkProjectResources, error) {
	d := domain.WorkProjectResources{Revision: revision, Comments: []domain.WorkComment{}, Attachments: []domain.WorkAttachment{}, Links: []domain.WorkLink{}, Notes: []domain.WorkNote{}, Events: []domain.WorkEvent{}}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.task_id,c.author_id,`+workPublicNameSQL+`,`+workPublicAvatarSQL+`,c.body,to_char(c.created_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_comments c JOIN users u ON u.id=c.author_id LEFT JOIN user_profiles p ON p.user_id=u.id JOIN work_tasks t ON t.id=c.task_id WHERE t.project_id=$1 ORDER BY c.created_at`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v domain.WorkComment
		if err := rows.Scan(&v.ID, &v.TaskID, &v.AuthorID, &v.Author, &v.Avatar, &v.Body, &v.CreatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Comments = append(d.Comments, v)
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT id,task_id,project_id,name,mime_type,size_bytes,to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_attachments WHERE project_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v domain.WorkAttachment
		if err := rows.Scan(&v.ID, &v.TaskID, &v.ProjectID, &v.Name, &v.MimeType, &v.Size, &v.CreatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Attachments = append(d.Attachments, v)
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT id,project_id,title,url,to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_links WHERE project_id=$1 ORDER BY created_at,id`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v domain.WorkLink
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Title, &v.URL, &v.CreatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Links = append(d.Links, v)
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT id,project_id,title,body,version,to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_notes WHERE project_id=$1 ORDER BY updated_at DESC,id DESC`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v domain.WorkNote
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Title, &v.Body, &v.Version, &v.CreatedAt, &v.UpdatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Notes = append(d.Notes, v)
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT e.id,e.actor_id,COALESCE(NULLIF(`+workPublicNameSQL+`,''),'Система'),`+workPublicAvatarSQL+`,e.kind,e.message,to_char(e.created_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_events e LEFT JOIN users u ON u.id=e.actor_id LEFT JOIN user_profiles p ON p.user_id=u.id WHERE e.project_id=$1 ORDER BY e.created_at DESC,e.id DESC LIMIT 100`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var v domain.WorkEvent
		if err := rows.Scan(&v.ID, &v.ActorID, &v.Actor, &v.ActorAvatar, &v.Kind, &v.Message, &v.CreatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Events = append(d.Events, v)
	}
	rows.Close()
	return d, nil
}
func (s *Repository) CreateWorkProject(ctx context.Context, in domain.WorkProjectInput) (domain.WorkProjectDetail, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO work_projects(owner_id,title,description,summary,status,deadline,completed_at) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::date,CASE WHEN $7 THEN now() END) RETURNING id`, uid, in.Title, in.Description, in.Summary, in.Status, in.Deadline, in.Completed).Scan(&id)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_project_members(project_id,user_id,role,pinned) VALUES($1,$2,'owner',$3)`, id, uid, in.Pinned); err != nil {
		return domain.WorkProjectDetail{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_sections(project_id,title,sort_order) VALUES($1,'Без раздела',0)`, id); err != nil {
		return domain.WorkProjectDetail{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO work_events(project_id,actor_id,kind,message) VALUES($1,$2,'project_created','Проект создан')`, id, uid)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.WorkProjectDetail{}, err
	}
	return s.WorkProjectCore(ctx, id)
}
func (s *Repository) UpdateWorkProject(ctx context.Context, id int64, in domain.WorkProjectInput) (domain.WorkProjectDetail, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE work_projects p SET title=$3,description=$4,summary=$5,status=$6,deadline=NULLIF($7,'')::date,completed_at=CASE WHEN $8 THEN COALESCE(completed_at,now()) ELSE NULL END,content_revision=content_revision+1,version=version+1,updated_at=now() FROM work_project_members m WHERE p.id=$1 AND m.project_id=p.id AND m.user_id=$2 AND p.version=$9 AND ($8=(p.completed_at IS NOT NULL) OR p.owner_id=$2)`, id, uid, in.Title, in.Description, in.Summary, in.Status, in.Deadline, in.Completed, in.Version)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.WorkProjectDetail{}, fmt.Errorf("проект изменён другим участником; обновите страницу: %w", domain.ErrConflict)
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE work_project_members SET pinned=$3 WHERE project_id=$1 AND user_id=$2 AND pinned IS DISTINCT FROM $3`, id, uid, in.Pinned)
	_, _ = s.db.ExecContext(ctx, `INSERT INTO work_events(project_id,actor_id,kind,message) VALUES($1,$2,'project_updated','Проект обновлён')`, id, uid)
	return s.WorkProjectCore(ctx, id)
}
func (s *Repository) DeleteWorkProject(ctx context.Context, id int64) error {
	uid, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM work_projects WHERE id=$1 AND owner_id=$2`, id, uid)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrForbidden
	}
	return nil
}
func (s *Repository) CreateWorkSection(ctx context.Context, projectID int64, in domain.WorkSectionInput) (domain.WorkSection, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkSection{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.WorkSection{}, err
	}
	defer tx.Rollback()
	var allowed bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM work_projects p JOIN work_project_members m ON m.project_id=p.id WHERE p.id=$1 AND m.user_id=$2 FOR UPDATE OF p)`, projectID, uid).Scan(&allowed); err != nil {
		return domain.WorkSection{}, err
	}
	if !allowed {
		return domain.WorkSection{}, domain.ErrNotFound
	}
	var sortOrder int64
	if in.RelativeTo == nil {
		err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order)+1,1) FROM work_sections WHERE project_id=$1`, projectID).Scan(&sortOrder)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT sort_order FROM work_sections WHERE id=$1 AND project_id=$2`, *in.RelativeTo, projectID).Scan(&sortOrder)
		if err == sql.ErrNoRows {
			return domain.WorkSection{}, domain.ErrNotFound
		}
		if err == nil && in.Placement == "below" {
			sortOrder++
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE work_sections SET sort_order=sort_order+1 WHERE project_id=$1 AND sort_order >= $2`, projectID, sortOrder)
		}
	}
	if err != nil {
		return domain.WorkSection{}, err
	}
	var v domain.WorkSection
	err = tx.QueryRowContext(ctx, `INSERT INTO work_sections(project_id,title,sort_order) VALUES($1,$2,$3) RETURNING id,project_id,title,sort_order,completed_section_id,reset_completed_on_move,move_completed_to_end`, projectID, in.Title, sortOrder).Scan(&v.ID, &v.ProjectID, &v.Title, &v.SortOrder, &v.CompletedSectionID, &v.ResetCompletedOnMove, &v.MoveCompletedToEnd)
	if err != nil {
		return v, err
	}
	if err = tx.Commit(); err != nil {
		return domain.WorkSection{}, err
	}
	return v, nil
}

func (s *Repository) UpdateWorkSection(ctx context.Context, id int64, title string) (domain.WorkSection, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkSection{}, err
	}
	var v domain.WorkSection
	err = s.db.QueryRowContext(ctx, `UPDATE work_sections section SET title=$3 FROM work_project_members member WHERE section.id=$1 AND member.project_id=section.project_id AND member.user_id=$2 RETURNING section.id,section.project_id,section.title,section.sort_order,section.completed_section_id,section.reset_completed_on_move,section.move_completed_to_end`, id, uid, title).Scan(&v.ID, &v.ProjectID, &v.Title, &v.SortOrder, &v.CompletedSectionID, &v.ResetCompletedOnMove, &v.MoveCompletedToEnd)
	if err == sql.ErrNoRows {
		return v, domain.ErrNotFound
	}
	return v, err
}

func (s *Repository) UpdateWorkSectionCompletion(ctx context.Context, id int64, sectionID *int64, resetCompletedOnMove, moveCompletedToEnd bool) (domain.WorkSection, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkSection{}, err
	}
	var v domain.WorkSection
	err = s.db.QueryRowContext(ctx, `UPDATE work_sections source SET completed_section_id=$3::BIGINT,reset_completed_on_move=$4,move_completed_to_end=$5 FROM work_project_members member WHERE source.id=$1 AND member.project_id=source.project_id AND member.user_id=$2 AND ($3::BIGINT IS NULL OR EXISTS(SELECT 1 FROM work_sections target WHERE target.id=$3::BIGINT AND target.project_id=source.project_id AND target.id<>source.id)) RETURNING source.id,source.project_id,source.title,source.sort_order,source.completed_section_id,source.reset_completed_on_move,source.move_completed_to_end`, id, uid, sectionID, resetCompletedOnMove, moveCompletedToEnd).Scan(&v.ID, &v.ProjectID, &v.Title, &v.SortOrder, &v.CompletedSectionID, &v.ResetCompletedOnMove, &v.MoveCompletedToEnd)
	if err == sql.ErrNoRows {
		return v, domain.ErrNotFound
	}
	return v, err
}

func (s *Repository) DeleteWorkSection(ctx context.Context, id int64) error {
	uid, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM work_sections section USING work_project_members member WHERE section.id=$1 AND member.project_id=section.project_id AND member.user_id=$2`, id, uid)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return domain.ErrNotFound
	}
	return nil
}
func (s *Repository) CreateWorkTask(ctx context.Context, projectID int64, in domain.WorkTaskInput) (domain.WorkTask, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkTask{}, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `INSERT INTO work_tasks(project_id,section_id,parent_id,title,description,created_by,assignee_id,due_date,completed_at,sort_order) SELECT $1,$3::BIGINT,$4::BIGINT,$5,$6,$2,$7::BIGINT,NULLIF($8,'')::date,CASE WHEN $9 THEN now() END,COALESCE((SELECT MIN(sort_order)-1 FROM work_tasks WHERE project_id=$1 AND section_id IS NOT DISTINCT FROM $3::BIGINT AND parent_id IS NOT DISTINCT FROM $4::BIGINT),0) FROM work_project_members WHERE project_id=$1 AND user_id=$2 AND ($3::BIGINT IS NULL OR EXISTS(SELECT 1 FROM work_sections WHERE id=$3::BIGINT AND project_id=$1)) AND ($4::BIGINT IS NULL OR EXISTS(SELECT 1 FROM work_tasks WHERE id=$4::BIGINT AND project_id=$1 AND parent_id IS NULL)) AND ($7::BIGINT IS NULL OR EXISTS(SELECT 1 FROM work_project_members WHERE project_id=$1 AND user_id=$7::BIGINT)) RETURNING id`, projectID, uid, in.SectionID, in.ParentID, in.Title, in.Description, in.AssigneeID, in.DueDate, in.Completed).Scan(&id)
	if err == sql.ErrNoRows {
		return domain.WorkTask{}, domain.ErrForbidden
	}
	if err != nil {
		return domain.WorkTask{}, err
	}
	_, _ = s.db.ExecContext(ctx, `INSERT INTO work_events(project_id,actor_id,kind,message) VALUES($1,$2,'task_created',$3)`, projectID, uid, "Создана задача «"+in.Title+"»")
	return s.workTask(ctx, id, uid)
}
func (s *Repository) workTask(ctx context.Context, id, uid int64) (domain.WorkTask, error) {
	var v domain.WorkTask
	err := s.db.QueryRowContext(ctx, `SELECT t.id,t.project_id,t.section_id,t.parent_id,t.title,t.description,t.created_by,t.assignee_id,`+workPublicNameSQL+`,COALESCE(to_char(t.due_date,'YYYY-MM-DD'),''),t.completed_at IS NOT NULL,t.sort_order,t.version,to_char(t.created_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),to_char(t.updated_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_tasks t JOIN work_project_members m ON m.project_id=t.project_id LEFT JOIN users u ON u.id=t.assignee_id LEFT JOIN user_profiles p ON p.user_id=u.id WHERE t.id=$1 AND m.user_id=$2`, id, uid).Scan(&v.ID, &v.ProjectID, &v.SectionID, &v.ParentID, &v.Title, &v.Description, &v.CreatedByID, &v.AssigneeID, &v.Assignee, &v.DueDate, &v.Completed, &v.SortOrder, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return v, domain.ErrNotFound
	}
	return v, err
}
func (s *Repository) UpdateWorkTask(ctx context.Context, id int64, in domain.WorkTaskInput) (domain.WorkTask, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkTask{}, err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE work_tasks t
		SET section_id=CASE
				WHEN $9 AND t.completed_at IS NULL AND t.parent_id IS NULL THEN COALESCE(
					(SELECT target.id
					 FROM work_sections source
					 JOIN work_sections target ON target.id=source.completed_section_id AND target.project_id=source.project_id
					 WHERE source.id=t.section_id AND source.project_id=t.project_id),
					$3::BIGINT)
				ELSE $3::BIGINT
			END,
			parent_id=$4::BIGINT,
			title=$5,
			description=$6,
			assignee_id=$7::BIGINT,
			due_date=NULLIF($8,'')::date,
			completed_at=CASE
				WHEN $9 AND t.completed_at IS NULL AND t.parent_id IS NULL AND EXISTS(
					SELECT 1 FROM work_sections source
					WHERE source.id=t.section_id AND source.project_id=t.project_id
					  AND source.completed_section_id IS NOT NULL AND source.reset_completed_on_move
				) THEN NULL
				WHEN $9 THEN COALESCE(completed_at,now())
				ELSE NULL
			END,
			sort_order=CASE
				WHEN $9 AND t.completed_at IS NULL AND t.parent_id IS NULL AND EXISTS(
					SELECT 1 FROM work_sections source
					WHERE source.id=t.section_id AND source.project_id=t.project_id AND source.move_completed_to_end
				) THEN COALESCE((
					SELECT MAX(other.sort_order)+1
					FROM work_tasks other
					WHERE other.project_id=t.project_id
					  AND other.parent_id IS NULL
					  AND other.id<>t.id
					  AND other.section_id IS NOT DISTINCT FROM COALESCE(
						(SELECT target.id
						 FROM work_sections source
						 JOIN work_sections target ON target.id=source.completed_section_id AND target.project_id=source.project_id
						 WHERE source.id=t.section_id AND source.project_id=t.project_id),
						$3::BIGINT)
				),1)
				ELSE $10
			END,
			version=version+1,
			updated_at=now()
		FROM work_project_members m
		WHERE t.id=$1
		  AND m.project_id=t.project_id
		  AND m.user_id=$2
		  AND t.version=$11
		  AND ($3::BIGINT IS NULL OR EXISTS(SELECT 1 FROM work_sections WHERE id=$3::BIGINT AND project_id=t.project_id))
		  AND ($4::BIGINT IS NULL OR EXISTS(SELECT 1 FROM work_tasks parent WHERE parent.id=$4::BIGINT AND parent.project_id=t.project_id AND parent.parent_id IS NULL))
		  AND ($7::BIGINT IS NULL OR EXISTS(SELECT 1 FROM work_project_members WHERE project_id=t.project_id AND user_id=$7::BIGINT))`, id, uid, in.SectionID, in.ParentID, in.Title, in.Description, in.AssigneeID, in.DueDate, in.Completed, in.SortOrder, in.Version)
	if err != nil {
		return domain.WorkTask{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.WorkTask{}, fmt.Errorf("задача изменена другим участником; обновите проект: %w", domain.ErrConflict)
	}
	v, err := s.workTask(ctx, id, uid)
	if err == nil {
		_, _ = s.db.ExecContext(ctx, `INSERT INTO work_events(project_id,actor_id,kind,message) VALUES($1,$2,'task_updated',$3)`, v.ProjectID, uid, "Обновлена задача «"+v.Title+"»")
	}
	return v, err
}
func (s *Repository) ClaimWorkTask(ctx context.Context, id int64) (domain.WorkTask, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkTask{}, err
	}
	var projectID int64
	var title string
	err = s.db.QueryRowContext(ctx, `UPDATE work_tasks task SET assignee_id=$2,version=version+1,updated_at=now() FROM work_project_members member WHERE task.id=$1 AND member.project_id=task.project_id AND member.user_id=$2 AND task.assignee_id IS NULL AND task.created_by IS DISTINCT FROM $2 RETURNING task.project_id,task.title`, id, uid).Scan(&projectID, &title)
	if err == sql.ErrNoRows {
		current, lookupErr := s.workTask(ctx, id, uid)
		if lookupErr != nil {
			return domain.WorkTask{}, lookupErr
		}
		if current.AssigneeID != nil && *current.AssigneeID == uid {
			return current, nil
		}
		if current.CreatedByID != nil && *current.CreatedByID == uid {
			return domain.WorkTask{}, domain.ErrForbidden
		}
		return domain.WorkTask{}, fmt.Errorf("задачу уже взял другой участник; обновите проект: %w", domain.ErrConflict)
	}
	if err != nil {
		return domain.WorkTask{}, err
	}
	_, _ = s.db.ExecContext(ctx, `INSERT INTO work_events(project_id,actor_id,kind,message) VALUES($1,$2,'task_claimed',$3)`, projectID, uid, "Взята задача «"+title+"»")
	return s.workTask(ctx, id, uid)
}
func (s *Repository) DeleteWorkTask(ctx context.Context, id int64) error {
	uid, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM work_tasks t USING work_project_members m WHERE t.id=$1 AND m.project_id=t.project_id AND m.user_id=$2`, id, uid)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
func (s *Repository) CreateWorkComment(ctx context.Context, taskID int64, body string) (domain.WorkComment, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkComment{}, err
	}
	var v domain.WorkComment
	err = s.db.QueryRowContext(ctx, `INSERT INTO work_comments(task_id,author_id,body) SELECT t.id,$2,$3 FROM work_tasks t JOIN work_project_members m ON m.project_id=t.project_id WHERE t.id=$1 AND m.user_id=$2 AND (SELECT count(*) FROM work_comments WHERE task_id=$1)<$4 RETURNING id,task_id,author_id,'','',body,to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SSOF')`, taskID, uid, body, application.MaxWorkTaskComments).Scan(&v.ID, &v.TaskID, &v.AuthorID, &v.Author, &v.Avatar, &v.Body, &v.CreatedAt)
	if err == sql.ErrNoRows {
		var visible, capacity bool
		lookupErr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM work_tasks t JOIN work_project_members m ON m.project_id=t.project_id WHERE t.id=$1 AND m.user_id=$2),(SELECT count(*)<$3 FROM work_comments WHERE task_id=$1)`, taskID, uid, application.MaxWorkTaskComments).Scan(&visible, &capacity)
		if lookupErr != nil {
			return v, lookupErr
		}
		if visible && !capacity {
			return v, fmt.Errorf("в задаче достигнут лимит комментариев: %w", domain.ErrConflict)
		}
		return v, domain.ErrForbidden
	}
	if err == nil {
		_ = s.db.QueryRowContext(ctx, `SELECT `+workPublicNameSQL+`,`+workPublicAvatarSQL+` FROM users u LEFT JOIN user_profiles p ON p.user_id=u.id WHERE u.id=$1`, uid).Scan(&v.Author, &v.Avatar)
	}
	return v, err
}
func (s *Repository) WorkTaskComments(ctx context.Context, taskID int64) ([]domain.WorkComment, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	var visible bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM work_tasks t JOIN work_project_members m ON m.project_id=t.project_id WHERE t.id=$1 AND m.user_id=$2)`, taskID, uid).Scan(&visible); err != nil {
		return nil, err
	}
	if !visible {
		return nil, domain.ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.task_id,c.author_id,`+workPublicNameSQL+`,`+workPublicAvatarSQL+`,c.body,to_char(c.created_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_comments c JOIN work_tasks t ON t.id=c.task_id JOIN work_project_members m ON m.project_id=t.project_id AND m.user_id=$2 JOIN users u ON u.id=c.author_id LEFT JOIN user_profiles p ON p.user_id=u.id WHERE c.task_id=$1 ORDER BY c.created_at`, taskID, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WorkComment{}
	for rows.Next() {
		var v domain.WorkComment
		if err := rows.Scan(&v.ID, &v.TaskID, &v.AuthorID, &v.Author, &v.Avatar, &v.Body, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func hashWorkToken(token string) string {
	v := sha256.Sum256([]byte(token))
	return hex.EncodeToString(v[:])
}
func (s *Repository) CreateWorkInvite(ctx context.Context, projectID int64, token string, expires time.Time) error {
	uid, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO work_invites(token_hash,project_id,created_by,expires_at) SELECT $3,$1,$2,$4 FROM work_projects WHERE id=$1 AND owner_id=$2`, projectID, uid, hashWorkToken(token), expires)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrForbidden
	}
	return nil
}
func (s *Repository) RevokeWorkInvites(ctx context.Context, projectID int64) error {
	uid, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE work_invites i SET revoked_at=now() FROM work_projects p WHERE i.project_id=$1 AND p.id=i.project_id AND p.owner_id=$2 AND i.revoked_at IS NULL`, projectID, uid)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		var owner bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM work_projects WHERE id=$1 AND owner_id=$2)`, projectID, uid).Scan(&owner); err != nil {
			return err
		}
		if !owner {
			return domain.ErrForbidden
		}
	}
	return nil
}
func (s *Repository) RemoveWorkMember(ctx context.Context, projectID, userID int64) error {
	uid, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM work_project_members m USING work_projects p WHERE m.project_id=$1 AND m.user_id=$3 AND m.role='member' AND p.id=m.project_id AND p.owner_id=$2`, projectID, uid, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrForbidden
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_tasks SET assignee_id=NULL,version=version+1,updated_at=now() WHERE project_id=$1 AND assignee_id=$2`, projectID, userID); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Repository) LeaveWorkProject(ctx context.Context, projectID int64) error {
	uid, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM work_project_members WHERE project_id=$1 AND user_id=$2 AND role='member'`, projectID, uid)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("владелец не может покинуть проект: %w", domain.ErrConflict)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_tasks SET assignee_id=NULL,version=version+1,updated_at=now() WHERE project_id=$1 AND assignee_id=$2`, projectID, uid); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Repository) AcceptWorkInvite(ctx context.Context, token string, now time.Time) (domain.WorkProjectDetail, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `SELECT project_id FROM work_invites WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at>$2`, hashWorkToken(token), now).Scan(&id)
	if err == sql.ErrNoRows {
		return domain.WorkProjectDetail{}, fmt.Errorf("ссылка недействительна или истекла: %w", domain.ErrConflict)
	}
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO work_project_members(project_id,user_id,role) VALUES($1,$2,'member') ON CONFLICT DO NOTHING`, id, uid)
	if err != nil {
		return domain.WorkProjectDetail{}, err
	}
	if added, _ := result.RowsAffected(); added > 0 {
		_, _ = s.db.ExecContext(ctx, `INSERT INTO work_events(project_id,actor_id,kind,message) VALUES($1,$2,'member_joined','Участник присоединился к проекту')`, id, uid)
	}
	return s.WorkProjectCore(ctx, id)
}
func (s *Repository) CreateWorkAttachment(ctx context.Context, projectID int64, taskID *int64, name, mimeType string, content []byte) (domain.WorkAttachment, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkAttachment{}, err
	}
	var v domain.WorkAttachment
	err = s.db.QueryRowContext(ctx, `INSERT INTO work_attachments(project_id,task_id,uploader_id,name,mime_type,size_bytes,content) SELECT $1,$3::BIGINT,$2,$4,$5,$6,$7 FROM work_project_members WHERE project_id=$1 AND user_id=$2 AND ($3::BIGINT IS NULL OR EXISTS(SELECT 1 FROM work_tasks WHERE id=$3::BIGINT AND project_id=$1)) AND (SELECT count(*) FROM work_attachments WHERE project_id=$1)<$8 RETURNING id,task_id,project_id,name,mime_type,size_bytes,to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SSOF')`, projectID, uid, taskID, name, mimeType, len(content), content, application.MaxWorkProjectAttachments).Scan(&v.ID, &v.TaskID, &v.ProjectID, &v.Name, &v.MimeType, &v.Size, &v.CreatedAt)
	if err == sql.ErrNoRows {
		var member, capacity bool
		lookupErr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM work_project_members WHERE project_id=$1 AND user_id=$2),(SELECT count(*)<$3 FROM work_attachments WHERE project_id=$1)`, projectID, uid, application.MaxWorkProjectAttachments).Scan(&member, &capacity)
		if lookupErr != nil {
			return v, lookupErr
		}
		if member && !capacity {
			return v, fmt.Errorf("в проекте достигнут лимит файлов: %w", domain.ErrConflict)
		}
		return v, domain.ErrForbidden
	}
	return v, err
}
func (s *Repository) WorkAttachment(ctx context.Context, id int64) (domain.WorkAttachmentData, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkAttachmentData{}, err
	}
	var v domain.WorkAttachmentData
	err = s.db.QueryRowContext(ctx, `SELECT a.id,a.task_id,a.project_id,a.name,a.mime_type,a.size_bytes,to_char(a.created_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),a.content FROM work_attachments a JOIN work_project_members m ON m.project_id=a.project_id WHERE a.id=$1 AND m.user_id=$2`, id, uid).Scan(&v.ID, &v.TaskID, &v.ProjectID, &v.Name, &v.MimeType, &v.Size, &v.CreatedAt, &v.Content)
	if err == sql.ErrNoRows {
		return v, domain.ErrNotFound
	}
	return v, err
}

func (s *Repository) DeleteWorkAttachment(ctx context.Context, id int64) error {
	uid, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM work_attachments a USING work_project_members m WHERE a.id=$1 AND m.project_id=a.project_id AND m.user_id=$2`, id, uid)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Repository) CreateWorkLink(ctx context.Context, projectID int64, in domain.WorkLinkInput) (domain.WorkLink, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkLink{}, err
	}
	var v domain.WorkLink
	err = s.db.QueryRowContext(ctx, `INSERT INTO work_links(project_id,created_by,title,url) SELECT $1,$2,$3,$4 FROM work_project_members WHERE project_id=$1 AND user_id=$2 AND (SELECT count(*) FROM work_links WHERE project_id=$1)<$5 RETURNING id,project_id,title,url,to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SSOF')`, projectID, uid, in.Title, in.URL, application.MaxWorkProjectLinks).Scan(&v.ID, &v.ProjectID, &v.Title, &v.URL, &v.CreatedAt)
	if err == sql.ErrNoRows {
		var member, capacity bool
		lookupErr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM work_project_members WHERE project_id=$1 AND user_id=$2),(SELECT count(*)<$3 FROM work_links WHERE project_id=$1)`, projectID, uid, application.MaxWorkProjectLinks).Scan(&member, &capacity)
		if lookupErr != nil {
			return v, lookupErr
		}
		if member && !capacity {
			return v, fmt.Errorf("в проекте достигнут лимит ссылок: %w", domain.ErrConflict)
		}
		return v, domain.ErrForbidden
	}
	return v, err
}

func (s *Repository) DeleteWorkLink(ctx context.Context, id int64) error {
	uid, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM work_links l USING work_project_members m WHERE l.id=$1 AND m.project_id=l.project_id AND m.user_id=$2`, id, uid)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Repository) CreateWorkNote(ctx context.Context, projectID int64, in domain.WorkNoteInput) (domain.WorkNote, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkNote{}, err
	}
	var v domain.WorkNote
	err = s.db.QueryRowContext(ctx, `INSERT INTO work_notes(project_id,created_by,title,body) SELECT $1,$2,$3,$4 FROM work_project_members WHERE project_id=$1 AND user_id=$2 AND (SELECT count(*) FROM work_notes WHERE project_id=$1)<$5 RETURNING id,project_id,title,body,version,to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SSOF')`, projectID, uid, in.Title, in.Body, application.MaxWorkProjectNotes).Scan(&v.ID, &v.ProjectID, &v.Title, &v.Body, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		var member, capacity bool
		lookupErr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM work_project_members WHERE project_id=$1 AND user_id=$2),(SELECT count(*)<$3 FROM work_notes WHERE project_id=$1)`, projectID, uid, application.MaxWorkProjectNotes).Scan(&member, &capacity)
		if lookupErr != nil {
			return v, lookupErr
		}
		if member && !capacity {
			return v, fmt.Errorf("в проекте достигнут лимит заметок: %w", domain.ErrConflict)
		}
		return v, domain.ErrForbidden
	}
	return v, err
}

func (s *Repository) WorkNote(ctx context.Context, id int64) (domain.WorkNote, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkNote{}, err
	}
	var v domain.WorkNote
	err = s.db.QueryRowContext(ctx, `SELECT note.id,note.project_id,note.title,note.body,note.version,to_char(note.created_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),to_char(note.updated_at,'YYYY-MM-DD"T"HH24:MI:SSOF') FROM work_notes note JOIN work_project_members member ON member.project_id=note.project_id WHERE note.id=$1 AND member.user_id=$2`, id, uid).Scan(&v.ID, &v.ProjectID, &v.Title, &v.Body, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return v, domain.ErrNotFound
	}
	return v, err
}

func (s *Repository) UpdateWorkNote(ctx context.Context, id int64, in domain.WorkNoteInput) (domain.WorkNote, error) {
	uid, err := currentUserID(ctx)
	if err != nil {
		return domain.WorkNote{}, err
	}
	var v domain.WorkNote
	err = s.db.QueryRowContext(ctx, `UPDATE work_notes note SET title=$3,body=$4,version=note.version+1,updated_at=now() FROM work_project_members member WHERE note.id=$1 AND member.project_id=note.project_id AND member.user_id=$2 AND note.version=$5 RETURNING note.id,note.project_id,note.title,note.body,note.version,to_char(note.created_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),to_char(note.updated_at,'YYYY-MM-DD"T"HH24:MI:SSOF')`, id, uid, in.Title, in.Body, in.Version).Scan(&v.ID, &v.ProjectID, &v.Title, &v.Body, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if err != sql.ErrNoRows {
		return v, err
	}
	var visible bool
	if lookupErr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM work_notes note JOIN work_project_members member ON member.project_id=note.project_id WHERE note.id=$1 AND member.user_id=$2)`, id, uid).Scan(&visible); lookupErr != nil {
		return v, lookupErr
	}
	if visible {
		return v, fmt.Errorf("заметка изменена другим участником; обновите проект: %w", domain.ErrConflict)
	}
	return v, domain.ErrNotFound
}

func (s *Repository) DeleteWorkNote(ctx context.Context, id int64) error {
	uid, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM work_notes note USING work_project_members member WHERE note.id=$1 AND member.project_id=note.project_id AND member.user_id=$2`, id, uid)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return domain.ErrNotFound
	}
	return nil
}
