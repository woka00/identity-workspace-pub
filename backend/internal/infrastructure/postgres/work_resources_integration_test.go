package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

func TestWorkProfileVisibilityAndCardPhotoFallback(t *testing.T) {
	repository := registrationTestRepository(t)
	ownerContext := application.WithUser(context.Background(), domain.User{ID: 1})
	const cardPhoto = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	if _, err := repository.db.Exec(`UPDATE user_profiles SET photo=$2 WHERE user_id=$1`, 1, cardPhoto); err != nil {
		t.Fatalf("set card photo: %v", err)
	}
	if err := repository.UpdateWorkProfile(ownerContext, domain.WorkProfileInput{DisplayName: "Публичный ник", ShowAvatar: true}); err != nil {
		t.Fatalf("update work profile: %v", err)
	}
	project, err := repository.CreateWorkProject(ownerContext, domain.WorkProjectInput{Title: "Публичный профиль", Status: "on_track"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	ownerID := int64(1)
	task, err := repository.CreateWorkTask(ownerContext, project.Project.ID, domain.WorkTaskInput{Title: "Задача", AssigneeID: &ownerID})
	if err != nil {
		t.Fatalf("create assigned task: %v", err)
	}
	comment, err := repository.CreateWorkComment(ownerContext, task.ID, "Комментарий")
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if len(project.Project.Members) != 1 || project.Project.Members[0].Name != "Публичный ник" || project.Project.Members[0].Avatar != cardPhoto {
		t.Fatalf("public project member = %+v", project.Project.Members)
	}
	if task.Assignee != "Публичный ник" || comment.Author != "Публичный ник" || comment.Avatar != cardPhoto {
		t.Fatalf("public task/comment identity: task=%+v comment=%+v", task, comment)
	}
	if err := repository.UpdateWorkProfile(ownerContext, domain.WorkProfileInput{DisplayName: "Новый ник", ShowAvatar: false}); err != nil {
		t.Fatalf("hide work profile: %v", err)
	}
	reloaded, err := repository.WorkProject(ownerContext, project.Project.ID)
	if err != nil {
		t.Fatalf("reload hidden work profile: %v", err)
	}
	if reloaded.Project.ContentRevision <= project.Project.ContentRevision {
		t.Fatalf("revision after work profile update = %d, want > %d", reloaded.Project.ContentRevision, project.Project.ContentRevision)
	}
	if len(reloaded.Project.Members) != 1 || reloaded.Project.Members[0].Name != "Новый ник" || reloaded.Project.Members[0].Avatar != "" {
		t.Fatalf("hidden project member = %+v", reloaded.Project.Members)
	}
	if len(reloaded.Tasks) != 1 || reloaded.Tasks[0].Assignee != "Новый ник" || len(reloaded.Comments) != 1 || reloaded.Comments[0].Author != "Новый ник" || reloaded.Comments[0].Avatar != "" {
		t.Fatalf("hidden task/comment identity: tasks=%+v comments=%+v", reloaded.Tasks, reloaded.Comments)
	}
	if err := repository.SetPhoto(ownerContext, cardPhoto); err != nil {
		t.Fatalf("update fallback photo: %v", err)
	}
	afterPhoto, err := repository.WorkProjectRevision(ownerContext, project.Project.ID)
	if err != nil || afterPhoto.Revision <= reloaded.Project.ContentRevision {
		t.Fatalf("revision after photo update: value=%+v err=%v", afterPhoto, err)
	}
}

func TestWorkResourcesPersistAndRemainMemberScoped(t *testing.T) {
	repository := registrationTestRepository(t)
	ownerContext := application.WithUser(context.Background(), domain.User{ID: 1})

	project, err := repository.CreateWorkProject(ownerContext, domain.WorkProjectInput{
		Title:  "Ресурсы",
		Status: "on_track",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	attachment, err := repository.CreateWorkAttachment(
		ownerContext,
		project.Project.ID,
		nil,
		"brief.txt",
		"text/plain",
		[]byte("project brief"),
	)
	if err != nil {
		t.Fatalf("create attachment: %v", err)
	}
	link, err := repository.CreateWorkLink(ownerContext, project.Project.ID, domain.WorkLinkInput{
		Title: "Документация",
		URL:   "https://example.com/docs",
	})
	if err != nil {
		t.Fatalf("create link: %v", err)
	}
	note, err := repository.CreateWorkNote(ownerContext, project.Project.ID, domain.WorkNoteInput{
		Title: "Общие решения",
		Body:  "Запуск переносим на пятницу.",
	})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	summary, err := repository.WorkProjectResourceSummary(ownerContext, project.Project.ID)
	if err != nil || len(summary.Attachments) != 1 || len(summary.Links) != 1 || len(summary.Notes) != 1 {
		t.Fatalf("resource summary: value=%+v err=%v", summary, err)
	}
	if summary.Notes[0].Preview != "Запуск переносим на пятницу." {
		t.Fatalf("note preview = %q", summary.Notes[0].Preview)
	}
	loadedNote, err := repository.WorkNote(ownerContext, note.ID)
	if err != nil || loadedNote.Body != note.Body {
		t.Fatalf("load note: value=%+v err=%v", loadedNote, err)
	}

	reloaded, err := repository.WorkProject(ownerContext, project.Project.ID)
	if err != nil {
		t.Fatalf("reload project: %v", err)
	}
	if len(reloaded.Attachments) != 1 || reloaded.Attachments[0].ID != attachment.ID {
		t.Fatalf("unexpected attachments after reload: %+v", reloaded.Attachments)
	}
	if len(reloaded.Links) != 1 || reloaded.Links[0].ID != link.ID || reloaded.Links[0].URL != "https://example.com/docs" {
		t.Fatalf("unexpected links after reload: %+v", reloaded.Links)
	}
	if len(reloaded.Notes) != 1 || reloaded.Notes[0].ID != note.ID || reloaded.Notes[0].Body != "Запуск переносим на пятницу." {
		t.Fatalf("unexpected notes after reload: %+v", reloaded.Notes)
	}
	updatedNote, err := repository.UpdateWorkNote(ownerContext, note.ID, domain.WorkNoteInput{Title: note.Title, Body: "Запуск в пятницу.", Version: note.Version})
	if err != nil || updatedNote.Version != note.Version+1 {
		t.Fatalf("update note: value=%+v err=%v", updatedNote, err)
	}
	updatedSummary, err := repository.WorkProjectResourceSummary(ownerContext, project.Project.ID)
	if err != nil || len(updatedSummary.Notes) != 1 || updatedSummary.Notes[0].Preview != "Запуск в пятницу." {
		t.Fatalf("updated note preview: value=%+v err=%v", updatedSummary.Notes, err)
	}
	if _, err := repository.UpdateWorkNote(ownerContext, note.ID, domain.WorkNoteInput{Title: note.Title, Body: "Устаревшая правка", Version: note.Version}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale note update error = %v, want conflict", err)
	}

	outsiderContext := application.WithUser(context.Background(), domain.User{ID: 2})
	if _, err := repository.WorkProjectResourceSummary(outsiderContext, project.Project.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider resource summary error = %v, want not found", err)
	}
	if _, err := repository.WorkNote(outsiderContext, note.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider note error = %v, want not found", err)
	}
	if _, err := repository.CreateWorkAttachment(outsiderContext, project.Project.ID, nil, "private.txt", "text/plain", []byte("private")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("outsider attachment error = %v, want forbidden", err)
	}
	if _, err := repository.CreateWorkLink(outsiderContext, project.Project.ID, domain.WorkLinkInput{Title: "Private", URL: "https://example.com/private"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("outsider link error = %v, want forbidden", err)
	}
	if _, err := repository.CreateWorkNote(outsiderContext, project.Project.ID, domain.WorkNoteInput{Title: "Private"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("outsider note error = %v, want forbidden", err)
	}
	if _, err := repository.UpdateWorkNote(outsiderContext, note.ID, domain.WorkNoteInput{Title: note.Title, Body: "Private", Version: updatedNote.Version}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider update note error = %v, want not found", err)
	}
	if err := repository.DeleteWorkNote(outsiderContext, note.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider delete note error = %v, want not found", err)
	}
	if err := repository.DeleteWorkAttachment(outsiderContext, attachment.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider delete attachment error = %v, want not found", err)
	}
	if err := repository.DeleteWorkAttachment(ownerContext, attachment.ID); err != nil {
		t.Fatalf("delete attachment: %v", err)
	}
	reloaded, err = repository.WorkProject(ownerContext, project.Project.ID)
	if err != nil {
		t.Fatalf("reload project after attachment delete: %v", err)
	}
	if len(reloaded.Attachments) != 0 {
		t.Fatalf("attachments after delete: %+v", reloaded.Attachments)
	}
	if err := repository.DeleteWorkNote(ownerContext, note.ID); err != nil {
		t.Fatalf("delete note: %v", err)
	}

	var constraint string
	err = repository.db.QueryRow(`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname='work_attachments_size_bytes_check'`).Scan(&constraint)
	if err != nil {
		t.Fatalf("read attachment size constraint: %v", err)
	}
	if !strings.Contains(constraint, "52428800") {
		t.Fatalf("attachment size constraint = %q, want 50 MB", constraint)
	}
}

func TestPersonalProjectPinDoesNotBumpSharedRevision(t *testing.T) {
	repository := registrationTestRepository(t)
	ownerContext := application.WithUser(context.Background(), domain.User{ID: 1})
	project, err := repository.CreateWorkProject(ownerContext, domain.WorkProjectInput{Title: "Личный pin", Status: "on_track"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := repository.WorkProjectRevision(ownerContext, project.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.db.Exec(`UPDATE work_project_members SET pinned=NOT pinned WHERE project_id=$1 AND user_id=1`, project.Project.ID); err != nil {
		t.Fatal(err)
	}
	after, err := repository.WorkProjectRevision(ownerContext, project.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision {
		t.Fatalf("personal pin changed shared revision from %d to %d", before.Revision, after.Revision)
	}
}

func TestWorkContentRevisionAndSplitLoading(t *testing.T) {
	repository := registrationTestRepository(t)
	ownerContext := application.WithUser(context.Background(), domain.User{ID: 1})
	project, err := repository.CreateWorkProject(ownerContext, domain.WorkProjectInput{Title: "Быстрая загрузка", Status: "on_track"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if project.Project.ContentRevision <= 0 {
		t.Fatalf("initial content revision = %d", project.Project.ContentRevision)
	}
	if len(project.Comments) != 0 || len(project.Attachments) != 0 || len(project.Links) != 0 || len(project.Notes) != 0 || len(project.Events) != 0 {
		t.Fatalf("create response must contain only project core: %+v", project)
	}

	before := project.Project.ContentRevision
	task, err := repository.CreateWorkTask(ownerContext, project.Project.ID, domain.WorkTaskInput{Title: "Задача"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	afterTask, err := repository.WorkProjectRevision(ownerContext, project.Project.ID)
	if err != nil || afterTask.Revision <= before {
		t.Fatalf("revision after task: value=%+v err=%v", afterTask, err)
	}
	if _, err := repository.CreateWorkComment(ownerContext, task.ID, "Комментарий"); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	comments, err := repository.WorkTaskComments(ownerContext, task.ID)
	if err != nil || len(comments) != 1 || comments[0].Body != "Комментарий" {
		t.Fatalf("task comments: value=%+v err=%v", comments, err)
	}
	resources, err := repository.WorkProjectResources(ownerContext, project.Project.ID)
	if err != nil {
		t.Fatalf("load resources: %v", err)
	}
	if resources.Revision <= afterTask.Revision || len(resources.Comments) != 1 {
		t.Fatalf("resources after comment: %+v", resources)
	}
	core, err := repository.WorkProjectCore(ownerContext, project.Project.ID)
	if err != nil {
		t.Fatalf("load project core: %v", err)
	}
	if len(core.Tasks) != 1 || len(core.Comments) != 0 || len(core.Attachments) != 0 || len(core.Links) != 0 || len(core.Notes) != 0 || len(core.Events) != 0 {
		t.Fatalf("unexpected project core: %+v", core)
	}
	projects, err := repository.WorkProjects(ownerContext)
	if err != nil || len(projects) == 0 || len(projects[0].Members) == 0 {
		t.Fatalf("project list with batched members: projects=%+v err=%v", projects, err)
	}

	outsiderContext := application.WithUser(context.Background(), domain.User{ID: 2})
	if _, err := repository.WorkTaskComments(outsiderContext, task.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider task comments error = %v, want not found", err)
	}
	if _, err := repository.WorkProjectRevision(outsiderContext, project.Project.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider revision error = %v, want not found", err)
	}
	if _, err := repository.WorkProjectResources(outsiderContext, project.Project.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider resources error = %v, want not found", err)
	}
}

func TestWorkTaskClaimIsAtomicAndCreatorCannotClaim(t *testing.T) {
	repository := registrationTestRepository(t)
	ownerContext := application.WithUser(context.Background(), domain.User{ID: 1})
	var secondID, thirdID int64
	if err := repository.db.QueryRow(`SELECT id FROM users WHERE login_normalized='avatar02'`).Scan(&secondID); err != nil {
		t.Fatalf("find second member: %v", err)
	}
	if err := repository.db.QueryRow(`SELECT id FROM users WHERE login_normalized='avatar03'`).Scan(&thirdID); err != nil {
		t.Fatalf("find third member: %v", err)
	}
	secondContext := application.WithUser(context.Background(), domain.User{ID: secondID})
	thirdContext := application.WithUser(context.Background(), domain.User{ID: thirdID})

	project, err := repository.CreateWorkProject(ownerContext, domain.WorkProjectInput{Title: "Общий проект", Status: "on_track"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := repository.db.Exec(`INSERT INTO work_project_members(project_id,user_id,role) VALUES($1,$2,'member'),($1,$3,'member')`, project.Project.ID, secondID, thirdID); err != nil {
		t.Fatalf("add project members: %v", err)
	}
	task, err := repository.CreateWorkTask(ownerContext, project.Project.ID, domain.WorkTaskInput{Title: "Взять в работу"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.CreatedByID == nil || *task.CreatedByID != 1 {
		t.Fatalf("task creator = %v, want 1", task.CreatedByID)
	}
	if _, err := repository.ClaimWorkTask(ownerContext, task.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("creator claim error = %v, want forbidden", err)
	}

	type claimResult struct {
		userID int64
		task   domain.WorkTask
		err    error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	for userID, memberContext := range map[int64]context.Context{secondID: secondContext, thirdID: thirdContext} {
		go func() {
			<-start
			claimed, claimErr := repository.ClaimWorkTask(memberContext, task.ID)
			results <- claimResult{userID: userID, task: claimed, err: claimErr}
		}()
	}
	close(start)
	var winner claimResult
	conflicts := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			winner = result
		} else if errors.Is(result.err, domain.ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("claim error = %v", result.err)
		}
	}
	if winner.userID == 0 || conflicts != 1 || winner.task.AssigneeID == nil || *winner.task.AssigneeID != winner.userID || winner.task.Version != task.Version+1 {
		t.Fatalf("claim race: winner=%+v conflicts=%d", winner, conflicts)
	}
	winnerContext := secondContext
	if winner.userID == thirdID {
		winnerContext = thirdContext
	}
	repeated, err := repository.ClaimWorkTask(winnerContext, task.ID)
	if err != nil || repeated.AssigneeID == nil || *repeated.AssigneeID != winner.userID || repeated.Version != winner.task.Version {
		t.Fatalf("repeated claim must be idempotent: value=%+v err=%v", repeated, err)
	}
}

func TestWorkSectionAndTaskCreationWithEmptyOptionalFields(t *testing.T) {
	repository := registrationTestRepository(t)
	ownerContext := application.WithUser(context.Background(), domain.User{ID: 1})
	project, err := repository.CreateWorkProject(ownerContext, domain.WorkProjectInput{
		Title:  "План работ",
		Status: "on_track",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	section, err := repository.CreateWorkSection(ownerContext, project.Project.ID, domain.WorkSectionInput{Title: "В работе"})
	if err != nil {
		t.Fatalf("create section: %v", err)
	}
	task, err := repository.CreateWorkTask(ownerContext, project.Project.ID, domain.WorkTaskInput{
		SectionID: &section.ID,
		Title:     "Первая задача",
	})
	if err != nil {
		t.Fatalf("create task with empty parent and assignee: %v", err)
	}
	if task.SectionID == nil || *task.SectionID != section.ID || task.ParentID != nil || task.AssigneeID != nil {
		t.Fatalf("unexpected task fields: %+v", task)
	}
	above, err := repository.CreateWorkSection(ownerContext, project.Project.ID, domain.WorkSectionInput{Title: "Сверху", RelativeTo: &section.ID, Placement: "above"})
	if err != nil {
		t.Fatalf("create section above: %v", err)
	}
	below, err := repository.CreateWorkSection(ownerContext, project.Project.ID, domain.WorkSectionInput{Title: "Снизу", RelativeTo: &section.ID, Placement: "below"})
	if err != nil {
		t.Fatalf("create section below: %v", err)
	}
	configured, err := repository.UpdateWorkSectionCompletion(ownerContext, section.ID, &below.ID, false, false)
	if err != nil || configured.CompletedSectionID == nil || *configured.CompletedSectionID != below.ID || configured.ResetCompletedOnMove || configured.MoveCompletedToEnd {
		t.Fatalf("configure completed task section: value=%+v err=%v", configured, err)
	}
	completedTask, err := repository.UpdateWorkTask(ownerContext, task.ID, domain.WorkTaskInput{
		SectionID: task.SectionID,
		Title:     task.Title,
		Completed: true,
		SortOrder: task.SortOrder,
		Version:   task.Version,
	})
	if err != nil || completedTask.SectionID == nil || *completedTask.SectionID != below.ID || !completedTask.Completed {
		t.Fatalf("complete task into configured section: value=%+v err=%v", completedTask, err)
	}
	task, err = repository.UpdateWorkTask(ownerContext, completedTask.ID, domain.WorkTaskInput{
		SectionID: &section.ID,
		Title:     completedTask.Title,
		Completed: false,
		SortOrder: completedTask.SortOrder,
		Version:   completedTask.Version,
	})
	if err != nil || task.SectionID == nil || *task.SectionID != section.ID {
		t.Fatalf("return task to source section: value=%+v err=%v", task, err)
	}
	configured, err = repository.UpdateWorkSectionCompletion(ownerContext, section.ID, &below.ID, true, false)
	if err != nil || !configured.ResetCompletedOnMove {
		t.Fatalf("configure completion reset: value=%+v err=%v", configured, err)
	}
	resetTask, err := repository.UpdateWorkTask(ownerContext, task.ID, domain.WorkTaskInput{
		SectionID: task.SectionID,
		Title:     task.Title,
		Completed: true,
		SortOrder: task.SortOrder,
		Version:   task.Version,
	})
	if err != nil || resetTask.SectionID == nil || *resetTask.SectionID != below.ID || resetTask.Completed {
		t.Fatalf("move completed task and reset completion: value=%+v err=%v", resetTask, err)
	}
	targetTail, err := repository.CreateWorkTask(ownerContext, project.Project.ID, domain.WorkTaskInput{SectionID: &below.ID, Title: "Последняя задача"})
	if err != nil {
		t.Fatalf("create target tail task: %v", err)
	}
	moveToEndTask, err := repository.CreateWorkTask(ownerContext, project.Project.ID, domain.WorkTaskInput{SectionID: &section.ID, Title: "Перенести вниз"})
	if err != nil {
		t.Fatalf("create move-to-end task: %v", err)
	}
	configured, err = repository.UpdateWorkSectionCompletion(ownerContext, section.ID, &below.ID, false, true)
	if err != nil || !configured.MoveCompletedToEnd {
		t.Fatalf("configure completed task order: value=%+v err=%v", configured, err)
	}
	movedToEnd, err := repository.UpdateWorkTask(ownerContext, moveToEndTask.ID, domain.WorkTaskInput{
		SectionID: moveToEndTask.SectionID,
		Title:     moveToEndTask.Title,
		Completed: true,
		SortOrder: moveToEndTask.SortOrder,
		Version:   moveToEndTask.Version,
	})
	if err != nil || movedToEnd.SectionID == nil || *movedToEnd.SectionID != below.ID || movedToEnd.SortOrder <= targetTail.SortOrder {
		t.Fatalf("move completed task to section end: value=%+v tail=%+v err=%v", movedToEnd, targetTail, err)
	}
	sameSectionTask, err := repository.CreateWorkTask(ownerContext, project.Project.ID, domain.WorkTaskInput{SectionID: &section.ID, Title: "Оставить и перенести вниз"})
	if err != nil {
		t.Fatalf("create same-section task: %v", err)
	}
	sameSectionTail, err := repository.CreateWorkTask(ownerContext, project.Project.ID, domain.WorkTaskInput{SectionID: &section.ID, Title: "Нижняя задача раздела"})
	if err != nil {
		t.Fatalf("create same-section tail task: %v", err)
	}
	configured, err = repository.UpdateWorkSectionCompletion(ownerContext, section.ID, nil, false, true)
	if err != nil || configured.CompletedSectionID != nil || !configured.MoveCompletedToEnd {
		t.Fatalf("configure same-section completion order: value=%+v err=%v", configured, err)
	}
	movedWithinSection, err := repository.UpdateWorkTask(ownerContext, sameSectionTask.ID, domain.WorkTaskInput{
		SectionID: sameSectionTask.SectionID,
		Title:     sameSectionTask.Title,
		Completed: true,
		SortOrder: sameSectionTask.SortOrder,
		Version:   sameSectionTask.Version,
	})
	if err != nil || movedWithinSection.SectionID == nil || *movedWithinSection.SectionID != section.ID || movedWithinSection.SortOrder <= sameSectionTail.SortOrder {
		t.Fatalf("move completed task to current section end: value=%+v tail=%+v err=%v", movedWithinSection, sameSectionTail, err)
	}
	manuallyMovedCompleted, err := repository.UpdateWorkTask(ownerContext, movedWithinSection.ID, domain.WorkTaskInput{
		SectionID: movedWithinSection.SectionID,
		Title:     movedWithinSection.Title,
		Completed: true,
		SortOrder: sameSectionTail.SortOrder - 1,
		Version:   movedWithinSection.Version,
	})
	if err != nil || !manuallyMovedCompleted.Completed || manuallyMovedCompleted.SortOrder >= sameSectionTail.SortOrder {
		t.Fatalf("manually move completed task: value=%+v target=%+v err=%v", manuallyMovedCompleted, sameSectionTail, err)
	}
	ordered, err := repository.WorkProject(ownerContext, project.Project.ID)
	if err != nil {
		t.Fatalf("reload ordered sections: %v", err)
	}
	positions := map[int64]int{}
	for index, item := range ordered.Sections {
		positions[item.ID] = index
	}
	if !(positions[above.ID] < positions[section.ID] && positions[section.ID] < positions[below.ID]) {
		t.Fatalf("unexpected relative section order: %+v", ordered.Sections)
	}
	renamed, err := repository.UpdateWorkSection(ownerContext, section.ID, "Переименовано")
	if err != nil || renamed.Title != "Переименовано" {
		t.Fatalf("rename section: value=%+v err=%v", renamed, err)
	}
	outsiderContext := application.WithUser(context.Background(), domain.User{ID: 2})
	if _, err := repository.UpdateWorkSection(outsiderContext, section.ID, "Чужое изменение"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider rename section error = %v, want not found", err)
	}
	if _, err := repository.UpdateWorkSectionCompletion(outsiderContext, section.ID, &below.ID, false, false); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider completion target error = %v, want not found", err)
	}
	if err := repository.DeleteWorkSection(outsiderContext, section.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider delete section error = %v, want not found", err)
	}
	if err := repository.DeleteWorkSection(ownerContext, section.ID); err != nil {
		t.Fatalf("delete section: %v", err)
	}
	reloaded, err := repository.WorkProject(ownerContext, project.Project.ID)
	if err != nil {
		t.Fatalf("reload project after section delete: %v", err)
	}
	for _, reloadedTask := range reloaded.Tasks {
		if reloadedTask.ID == task.ID && (reloadedTask.SectionID == nil || *reloadedTask.SectionID != below.ID) {
			t.Fatalf("task section after deleting unrelated section = %v, want %d", reloadedTask.SectionID, below.ID)
		}
	}
}

func TestNewWorkTaskIsCreatedFirstInItsSection(t *testing.T) {
	repository := registrationTestRepository(t)
	ownerContext := application.WithUser(context.Background(), domain.User{ID: 1})
	project, err := repository.CreateWorkProject(ownerContext, domain.WorkProjectInput{Title: "Порядок задач", Status: "on_track"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	section, err := repository.CreateWorkSection(ownerContext, project.Project.ID, domain.WorkSectionInput{Title: "Раздел"})
	if err != nil {
		t.Fatalf("create section: %v", err)
	}
	first, err := repository.CreateWorkTask(ownerContext, project.Project.ID, domain.WorkTaskInput{SectionID: &section.ID, Title: "Старая задача", SortOrder: 999})
	if err != nil {
		t.Fatalf("create first task: %v", err)
	}
	newest, err := repository.CreateWorkTask(ownerContext, project.Project.ID, domain.WorkTaskInput{SectionID: &section.ID, Title: "Новая задача", SortOrder: 999})
	if err != nil {
		t.Fatalf("create newest task: %v", err)
	}
	if newest.SortOrder >= first.SortOrder {
		t.Fatalf("new task order = %d, want before existing order %d", newest.SortOrder, first.SortOrder)
	}
	loaded, err := repository.WorkProject(ownerContext, project.Project.ID)
	if err != nil {
		t.Fatalf("reload project: %v", err)
	}
	if len(loaded.Tasks) != 2 || loaded.Tasks[0].ID != newest.ID || loaded.Tasks[1].ID != first.ID {
		t.Fatalf("unexpected task order: %+v", loaded.Tasks)
	}
}
