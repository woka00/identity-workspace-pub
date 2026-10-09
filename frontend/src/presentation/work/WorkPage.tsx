import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { api } from "../../infrastructure/http/apiClient";
import type { Profile, WorkComment, WorkNote, WorkNoteInput, WorkNoteSummary, WorkProject, WorkProjectDetail, WorkProjectInput, WorkSection, WorkTask, WorkTaskInput } from "../../domain/models";
import ProjectProfileDialog from "../profile/ProjectProfileDialog";

type Props = { currentUserId: number; currentUserLogin: string; profile: Profile; onProfileChanged: () => Promise<void>; onError: (cause: unknown) => void; onChanged?: () => void };
type WorkSort = "manual" | "due" | "assignee" | "created" | "updated" | "completed" | "title";
type SortDirection = "asc" | "desc";
const emptyProject: WorkProjectInput = { title: "", description: "", summary: "", status: "on_track", deadline: "", completed: false, pinned: false, version: 0 };
const statusLabel = { on_track: "По плану", at_risk: "Под угрозой", off_track: "Отстаёт" } as const;
const workCalendarIcon = new URL("../../assets/work-calendar.svg", import.meta.url).href;
const workMembersIcon = new URL("../../assets/work-members.svg", import.meta.url).href;
const workProjectCreatedIcon = new URL("../../assets/work-project-created.svg", import.meta.url).href;
const workFilterIcon = new URL("../../assets/work-filter.svg", import.meta.url).href;
const workSortIcon = new URL("../../assets/work-sort.svg", import.meta.url).href;
const workSearchIcon = new URL("../../assets/work-search.svg", import.meta.url).href;
const workCompletionIcon = new URL("../../assets/work-completion.svg", import.meta.url).href;
const workAssigneeIcon = new URL("../../assets/work-assignee.svg", import.meta.url).href;
const workDueIcon = new URL("../../assets/work-due.svg", import.meta.url).href;
const workClockIcon = new URL("../../assets/work-clock.svg", import.meta.url).href;
const workOrderIcon = new URL("../../assets/work-order.svg", import.meta.url).href;
const workResourceFileIcon = new URL("../../assets/work-resource-file.svg", import.meta.url).href;
const workResourceLinkIcon = new URL("../../assets/work-resource-link.svg", import.meta.url).href;
const workResourceNoteIcon = new URL("../../assets/work-resource-note.svg", import.meta.url).href;
const workResourceOpenIcon = new URL("../../assets/work-resource-open.svg", import.meta.url).href;
const workResourceRemoveIcon = new URL("../../assets/work-resource-remove.svg", import.meta.url).href;
const workSectionRenameIcon = new URL("../../assets/work-section-rename.svg", import.meta.url).href;
const workSectionAddIcon = new URL("../../assets/work-section-add.svg", import.meta.url).href;
const workSectionDeleteIcon = new URL("../../assets/work-section-delete.svg", import.meta.url).href;
const workTaskAssigneeEmptyIcon = new URL("../../assets/work-task-assignee-empty.svg", import.meta.url).href;
const workTaskDueEmptyIcon = new URL("../../assets/work-task-due-empty.svg", import.meta.url).href;
const workTaskOpenIcon = new URL("../../assets/work-task-open.svg", import.meta.url).href;
const workTaskCompleteIcon = new URL("../../assets/work-task-complete.svg", import.meta.url).href;
const workTaskCompleteHoverIcon = new URL("../../assets/work-task-complete-hover.svg", import.meta.url).href;
const MAX_WORK_ATTACHMENT_BYTES = 50 * 1024 * 1024;

function WorkMobileIcon({ name }: { name: "menu" | "share" | "plus" | "close" }) {
  return <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
    {name === "menu" && <path d="M4 7h16M4 12h16M4 17h16"/>}
    {name === "plus" && <path d="M12 5v14M5 12h14"/>}
    {name === "close" && <path d="m6 6 12 12M18 6 6 18"/>}
    {name === "share" && <><circle cx="18" cy="5" r="2.5"/><circle cx="6" cy="12" r="2.5"/><circle cx="18" cy="19" r="2.5"/><path d="m8.2 10.8 7.6-4.5M8.2 13.2l7.6 4.5"/></>}
  </svg>;
}

type LinkDialogRequest = { id: number; title: string };
type WorkTaskGroup = { key: string; title: string; sectionId: number | null; tasks: WorkTask[] };
type SectionPlacement = "above" | "below" | "";
type TaskDropTarget = { id: number; placement: "before" | "after" };
type TaskContextMenuState = { task: WorkTask; x: number; y: number };
type WorkProjectView = Omit<WorkProjectDetail, "notes"> & { notes: WorkNoteSummary[] };
type WorkMemoryCache = {
  projects: WorkProject[];
  selectedProjectId: number | null;
  details: Map<number, WorkProjectView>;
  resourcesReady: Set<number>;
  notes: Map<number, WorkNote>;
  comments: Map<number, WorkComment[]>;
};

const workMemoryCaches = new Map<number, WorkMemoryCache>();

function workMemoryCache(userID: number) {
  let cache = workMemoryCaches.get(userID);
  if (!cache) {
    cache = { projects: [], selectedProjectId: null, details: new Map(), resourcesReady: new Set(), notes: new Map(), comments: new Map() };
    workMemoryCaches.set(userID, cache);
  }
  return cache;
}

function noteSummary(note: WorkNote): WorkNoteSummary {
  return { id: note.id, projectId: note.projectId, title: note.title, preview: note.body.trim().replace(/\s+/g, " ").slice(0, 240), version: note.version, createdAt: note.createdAt, updatedAt: note.updatedAt };
}

function safePercent(completed: number, total: number) {
  if (!Number.isFinite(total) || total <= 0) return 0;
  const value = Math.round((Number.isFinite(completed) ? completed : 0) / total * 100);
  return Math.min(100, Math.max(0, value));
}

function pluralizeCount(value: number, one: string, few: string, many: string) {
  const mod100 = value % 100;
  if (mod100 >= 11 && mod100 <= 14) return many;
  const mod10 = value % 10;
  if (mod10 === 1) return one;
  if (mod10 >= 2 && mod10 <= 4) return few;
  return many;
}

function fitWorkTaskTitle(element: HTMLTextAreaElement | null) {
  if (!element) return;
  element.dataset.titleLines = "1";
  element.style.height = "auto";
  const style = getComputedStyle(element);
  const padding = (Number.parseFloat(style.paddingTop) || 0) + (Number.parseFloat(style.paddingBottom) || 0);
  const lineHeight = Number.parseFloat(style.lineHeight) || Number.parseFloat(style.fontSize) * 1.12;
  const lines = Math.max(1, Math.round((element.scrollHeight - padding) / lineHeight));
  element.dataset.titleLines = String(Math.min(lines, 4));
  element.style.height = "auto";
  const resizedStyle = getComputedStyle(element);
  const borders = (Number.parseFloat(resizedStyle.borderTopWidth) || 0) + (Number.parseFloat(resizedStyle.borderBottomWidth) || 0);
  element.style.height = `${Math.ceil(element.scrollHeight + borders)}px`;
}

function workInitials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  return (parts.length > 1 ? parts.slice(0,2).map((part)=>part[0]).join("") : parts[0]?.slice(0,2) || "У").toUpperCase();
}

function WorkAvatar({ name, avatar, className = "", title }: { name: string; avatar: string; className?: string; title?: string }) {
  return <span className={`workPersonAvatar ${className}`} title={title}>{avatar ? <img src={avatar} alt=""/> : workInitials(name)}</span>;
}

function formatDeadline(value: string) {
  if (!value) return "";
  const date = new Date(`${value}T12:00:00`);
  return Number.isNaN(date.getTime()) ? "" : new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long", year: "numeric" }).format(date);
}

function formatActivityDate(value: string) {
  if (!value) return "";
  const date = new Date(value.replace(/([+-]\d{2})$/, "$1:00"));
  if (Number.isNaN(date.getTime())) return "";
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const eventDay = new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
  const time = new Intl.DateTimeFormat("ru-RU", { hour: "2-digit", minute: "2-digit" }).format(date);
  if (eventDay === today) return `Сегодня, ${time}`;
  if (eventDay === today - 86400000) return `Вчера, ${time}`;
  return `${new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long" }).format(date)}, ${time}`;
}

function formatFileSize(value: number) {
  if (value < 1024 * 1024) return `${Math.max(1, Math.ceil(value / 1024))} КБ`;
  return `${new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 1 }).format(value / 1024 / 1024)} МБ`;
}

function formatTaskDate(value: string) {
  if (!value) return "";
  const normalized = /^\d{4}-\d{2}-\d{2}$/.test(value) ? `${value}T12:00:00` : value.replace(/([+-]\d{2})$/, "$1:00");
  const date = new Date(normalized);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "short" }).format(date).replace(".", "");
}

function taskDueIsOverdue(task: WorkTask) {
  if (!task.dueDate || task.completed) return false;
  const now = new Date();
  const today = new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 10);
  return task.dueDate < today;
}

function taskInput(task: WorkTask, patch: Partial<WorkTaskInput> = {}): WorkTaskInput {
  return { sectionId: task.sectionId, parentId: task.parentId, title: task.title, description: task.description, assigneeId: task.assigneeId, dueDate: task.dueDate, completed: task.completed, sortOrder: task.sortOrder, version: task.version, ...patch };
}

export function WorkPage({ currentUserId, currentUserLogin, profile, onProfileChanged, onError, onChanged }: Props) {
  const memoryCache = useMemo(() => workMemoryCache(currentUserId), [currentUserId]);
  const initialDetail = memoryCache.selectedProjectId === null ? null : memoryCache.details.get(memoryCache.selectedProjectId) ?? null;
  const [projects, setProjects] = useState<WorkProject[]>(() => memoryCache.projects);
  const [detail, setDetail] = useState<WorkProjectView | null>(() => initialDetail);
  const [initialLoading, setInitialLoading] = useState(() => !initialDetail);
  const [resourcesReady, setResourcesReady] = useState(() => Boolean(initialDetail && memoryCache.resourcesReady.has(initialDetail.project.id)));
  const [tab, setTab] = useState<"overview" | "notes" | "list" | "board">("list");
  const [projectMenuOpen, setProjectMenuOpen] = useState(false);
  const [selectedNote, setSelectedNote] = useState<number | "new" | null>(null);
  const [openedNote, setOpenedNote] = useState<WorkNote | null>(null);
  const [noteReady, setNoteReady] = useState(false);
  const [selectedTask, setSelectedTask] = useState<number | null>(null);
  const [taskComments, setTaskComments] = useState<WorkComment[]>([]);
  const [taskCommentsReady, setTaskCommentsReady] = useState(false);
  const [projectSearch, setProjectSearch] = useState("");
  const [taskSearch, setTaskSearch] = useState("");
  const [assigneeFilter, setAssigneeFilter] = useState("");
  const [doneFilter, setDoneFilter] = useState("all");
  const [dueFilter, setDueFilter] = useState("all");
  const [sort, setSort] = useState<WorkSort>("manual");
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc");
  const [dragTaskId, setDragTaskId] = useState<number | null>(null);
  const [taskDropTarget, setTaskDropTarget] = useState<TaskDropTarget | null>(null);
  const [sectionDropTargetKey, setSectionDropTargetKey] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [createTitle, setCreateTitle] = useState("");
  const [taskCreateTarget, setTaskCreateTarget] = useState<{ sectionId: number | null; parentId: number | null } | null>(null);
  const [taskCreateTitle, setTaskCreateTitle] = useState("");
  const [taskCreateAssigneeID, setTaskCreateAssigneeID] = useState<number | null>(null);
  const [sectionCreateTarget, setSectionCreateTarget] = useState<{ relativeTo: number | null; placement: SectionPlacement } | null>(null);
  const [sectionCreateTitle, setSectionCreateTitle] = useState("");
  const [sectionRenameTarget, setSectionRenameTarget] = useState<WorkSection | null>(null);
  const [sectionRenameTitle, setSectionRenameTitle] = useState("");
  const [sectionCompletionTarget, setSectionCompletionTarget] = useState<WorkSection | null>(null);
  const [sectionCompletionTargetID, setSectionCompletionTargetID] = useState<number | null>(null);
  const [sectionCompletionReset, setSectionCompletionReset] = useState(false);
  const [sectionCompletionMoveToEnd, setSectionCompletionMoveToEnd] = useState(false);
  const [collapsedSections, setCollapsedSections] = useState<Set<string>>(()=>new Set());
  const [completionPendingTasks, setCompletionPendingTasks] = useState<Set<number>>(()=>new Set());
  const [assigneePendingTasks, setAssigneePendingTasks] = useState<Set<number>>(()=>new Set());
  const [taskContextMenu, setTaskContextMenu] = useState<TaskContextMenuState | null>(null);
  const [taskDeleteTarget, setTaskDeleteTarget] = useState<WorkTask | null>(null);
  const [oversizedFile, setOversizedFile] = useState<File | null>(null);
  const [linkDialogRequest, setLinkDialogRequest] = useState<LinkDialogRequest | null>(null);
  const [profileOpen, setProfileOpen] = useState(false);
  const [inviteLink, setInviteLink] = useState("");
  const [inviteCreating, setInviteCreating] = useState(false);
  const [inviteCopied, setInviteCopied] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const inviteInputRef = useRef<HTMLInputElement>(null);
  const projectMenuButtonRef = useRef<HTMLButtonElement>(null);
  const projectMenuCloseRef = useRef<HTMLButtonElement>(null);
  const taskDeleteReturnFocusRef = useRef<HTMLElement | null>(null);
  const taskDeletePendingRef = useRef(false);
  const dragTaskIdRef = useRef<number | null>(null);
  const detailRef = useRef<WorkProjectView | null>(initialDetail);
  const revisionRef = useRef(initialDetail?.project.contentRevision ?? 0);
  const loadGenerationRef = useRef(0);
  const projectCacheRef = useRef(memoryCache.details);
  const resourcesReadyProjectsRef = useRef(memoryCache.resourcesReady);

  useEffect(() => {
    detailRef.current = detail;
    if(!detail)return;
    memoryCache.selectedProjectId=detail.project.id;
    projectCacheRef.current.set(detail.project.id,detail);
    if(resourcesReady)resourcesReadyProjectsRef.current.add(detail.project.id);
  }, [detail,memoryCache,resourcesReady]);

  useEffect(() => {
    if (!projectMenuOpen) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    projectMenuCloseRef.current?.focus();
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      setProjectMenuOpen(false);
      requestAnimationFrame(()=>projectMenuButtonRef.current?.focus());
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [projectMenuOpen]);

  const refreshProjects = useCallback(async () => {
    const list = await api.workProjects();
    memoryCache.projects = list;
    setProjects(list);
    return list;
  }, [memoryCache]);

  const loadProject = useCallback(async (id: number, knownRevision?: number) => {
    const generation = ++loadGenerationRef.current;
    const cached = projectCacheRef.current.get(id);
    const canReuseResources = knownRevision !== undefined
      && cached?.project.contentRevision === knownRevision
      && resourcesReadyProjectsRef.current.has(id);
    let resourcesFailure: unknown;
    const requestResources = () => api.workProjectResourceSummary(id).catch((cause) => {
      resourcesFailure = cause;
      return null;
    });
    let resourcesPromise = canReuseResources ? null : requestResources();
    let core = await api.workProjectCore(id);
    if (generation !== loadGenerationRef.current) return;
    let reuseResources = Boolean(cached && resourcesReadyProjectsRef.current.has(id) && cached.project.contentRevision === core.project.contentRevision);
    if (!reuseResources && !resourcesPromise) resourcesPromise = requestResources();
    const nextDetail = reuseResources && cached
      ? { ...core, comments: [], attachments: cached.attachments, links: cached.links, notes: cached.notes, events: cached.events }
      : { ...core, notes: [] };
    revisionRef.current = core.project.contentRevision;
    detailRef.current = nextDetail;
    projectCacheRef.current.set(id, nextDetail);
    setDetail(nextDetail);
    setResourcesReady(reuseResources);
    setInitialLoading(false);
    if (reuseResources) return;
    resourcesReadyProjectsRef.current.delete(id);
    try {
      let resources = await resourcesPromise!;
      if (generation !== loadGenerationRef.current) return;
      if (!resources) {
        onError(resourcesFailure);
        return;
      }
      if (resources.revision !== core.project.contentRevision) {
        [core, resources] = await Promise.all([api.workProjectCore(id), api.workProjectResourceSummary(id)]);
        if (generation !== loadGenerationRef.current || resources.revision !== core.project.contentRevision) return;
        revisionRef.current = core.project.contentRevision;
      }
      const loaded: WorkProjectView = { ...core, comments: [], attachments: resources.attachments, links: resources.links, notes: resources.notes, events: resources.events };
      detailRef.current = loaded;
      projectCacheRef.current.set(id, loaded);
      setDetail(loaded);
      resourcesReadyProjectsRef.current.add(id);
      setResourcesReady(true);
    } catch (cause) { onError(cause); }
  }, [onError]);

  const showProjectImmediately = useCallback((project: WorkProject) => {
    const cached=projectCacheRef.current.get(project.id);
    const canUseCache=cached?.project.contentRevision===project.contentRevision;
    const immediate:WorkProjectView=canUseCache&&cached?cached:{project,sections:[],tasks:[],comments:[],attachments:[],links:[],notes:[],events:[]};
    const ready=canUseCache&&resourcesReadyProjectsRef.current.has(project.id);
    revisionRef.current=project.contentRevision;
    detailRef.current=immediate;
    setDetail(immediate);
    setResourcesReady(ready);
    setInitialLoading(false);
  },[]);

  const loadProjects = useCallback(async (prefer?: number | null) => {
    try {
      const list = await refreshProjects();
      const id = prefer === null ? list[0]?.id : prefer ?? detailRef.current?.project.id ?? list[0]?.id;
      const project=list.find((item)=>item.id===id);
      if (id&&project){showProjectImmediately(project);await loadProject(id,project.contentRevision);}else { memoryCache.selectedProjectId=null;setDetail(null); setResourcesReady(false); }
    } catch (cause) { onError(cause); } finally { setInitialLoading(false); }
  }, [loadProject, memoryCache, onError, refreshProjects, showProjectImmediately]);

  useEffect(() => { void loadProjects(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  const selectedNoteVersion = typeof selectedNote === "number" ? detail?.notes.find((item)=>item.id===selectedNote)?.version : undefined;
  useEffect(() => {
    if (typeof selectedNote !== "number") {
      setOpenedNote(null);
      setNoteReady(selectedNote === "new");
      return;
    }
    const cached = memoryCache.notes.get(selectedNote);
    if (cached && cached.version === selectedNoteVersion) {
      setOpenedNote(cached);
      setNoteReady(true);
      return;
    }
    let cancelled = false;
    setOpenedNote(null);
    setNoteReady(false);
    void api.workNote(selectedNote).then((note) => {
      if (cancelled) return;
      memoryCache.notes.set(note.id, note);
      setOpenedNote(note);
      setNoteReady(true);
    }).catch((cause) => {
      if (cancelled) return;
      setNoteReady(true);
      onError(cause);
    });
    return () => { cancelled = true; };
  }, [memoryCache, onError, selectedNote, selectedNoteVersion]);
  useEffect(() => {
    if (selectedTask === null) {
      setTaskComments([]);
      setTaskCommentsReady(false);
      return;
    }
    let cancelled = false;
    const cached = memoryCache.comments.get(selectedTask);
    setTaskComments(cached ?? []);
    setTaskCommentsReady(Boolean(cached));
    void api.workTaskComments(selectedTask).then((comments) => {
      if (cancelled) return;
      memoryCache.comments.set(selectedTask, comments);
      setTaskComments(comments);
      setTaskCommentsReady(true);
    }).catch((cause) => {
      if (cancelled) return;
      setTaskCommentsReady(true);
      onError(cause);
    });
    return () => { cancelled = true; };
  }, [memoryCache, onError, selectedTask]);
  function selectWorkNote(value: number | "new" | null) {
    if (typeof value === "number") {
      const summary = detailRef.current?.notes.find((item)=>item.id===value);
      const cached = memoryCache.notes.get(value);
      const usable = Boolean(cached && cached.version === summary?.version);
      setOpenedNote(usable ? cached! : null);
      setNoteReady(usable);
    } else {
      setOpenedNote(null);
      setNoteReady(value === "new");
    }
    setSelectedNote(value);
  }
  function selectWorkTask(value: number | null) {
    const cached = value === null ? undefined : memoryCache.comments.get(value);
    setTaskComments(cached ?? []);
    setTaskCommentsReady(Boolean(cached));
    setSelectedTask(value);
  }
  function openTaskContextMenu(task: WorkTask, x: number, y: number) {
    const menuWidth = 220;
    const menuHeight = 58;
    setTaskContextMenu({
      task,
      x: Math.max(8, Math.min(x, window.innerWidth - menuWidth - 8)),
      y: Math.max(8, Math.min(y, window.innerHeight - menuHeight - 8)),
    });
  }
  function requestTaskDelete(task: WorkTask, returnFocus?: HTMLElement | null) {
    taskDeleteReturnFocusRef.current = returnFocus ?? null;
    setTaskContextMenu(null);
    setTaskDeleteTarget(task);
  }
  function closeTaskDeleteDialog() {
    const taskID = taskDeleteTarget?.id;
    setTaskDeleteTarget(null);
    requestAnimationFrame(() => {
      const explicitTarget = taskDeleteReturnFocusRef.current;
      taskDeleteReturnFocusRef.current = null;
      if (explicitTarget?.isConnected) {
        explicitTarget.focus();
        return;
      }
      if (taskID !== undefined) document.querySelector<HTMLButtonElement>(`[data-work-task-open="${taskID}"]`)?.focus();
    });
  }
  async function confirmTaskDelete() {
    if (!taskDeleteTarget || !detail || taskDeletePendingRef.current) return;
    taskDeletePendingRef.current = true;
    const target = taskDeleteTarget;
    const projectID = detail.project.id;
    const removedTaskIDs = new Set<number>([target.id]);
    let foundChild = true;
    while (foundChild) {
      foundChild = false;
      for (const task of detail.tasks) {
        if (task.parentId !== null && removedTaskIDs.has(task.parentId) && !removedTaskIDs.has(task.id)) {
          removedTaskIDs.add(task.id);
          foundChild = true;
        }
      }
    }
    const previousDetail = detail;
    const previousSelectedTask = selectedTask;
    const previousTaskComments = taskComments;
    const previousCommentsReady = taskCommentsReady;

    setTaskDeleteTarget(null);
    taskDeleteReturnFocusRef.current = null;
    setDetail((current)=>current?.project.id===projectID?{
      ...current,
      tasks:current.tasks.filter((task)=>!removedTaskIDs.has(task.id)),
      comments:current.comments.filter((comment)=>!removedTaskIDs.has(comment.taskId)),
      attachments:current.attachments.filter((attachment)=>attachment.taskId===null||!removedTaskIDs.has(attachment.taskId)),
    }:current);
    if (previousSelectedTask !== null && removedTaskIDs.has(previousSelectedTask)) {
      setSelectedTask(null);
      setTaskComments([]);
      setTaskCommentsReady(false);
    }
    try {
      await api.deleteWorkTask(target.id);
      for (const taskID of removedTaskIDs) memoryCache.comments.delete(taskID);
      void acknowledgeLocalChange(projectID);
      void refreshProjects().catch(onError);
      onChanged?.();
    } catch (cause) {
      setDetail((current)=>current?.project.id===projectID?previousDetail:current);
      if (previousSelectedTask !== null && removedTaskIDs.has(previousSelectedTask)) {
        setSelectedTask(previousSelectedTask);
        setTaskComments(previousTaskComments);
        setTaskCommentsReady(previousCommentsReady);
      }
      onError(cause);
    } finally {
      taskDeletePendingRef.current = false;
    }
  }
  useEffect(() => {
    const projectID = detail?.project.id;
    if (!projectID) return;
    let timer: number | null = null;
    let checking = false;
    const check = async () => {
      if (document.visibilityState !== "visible" || checking) return;
      checking = true;
      try {
        const value = await api.workProjectRevision(projectID);
        if (value.revision !== revisionRef.current) {
          const taskID = selectedTask;
          await Promise.all([
            loadProject(projectID),
            refreshProjects(),
            taskID === null ? Promise.resolve() : api.workTaskComments(taskID).then((comments) => {
              memoryCache.comments.set(taskID, comments);
              if (selectedTask === taskID) {
                setTaskComments(comments);
                setTaskCommentsReady(true);
              }
            }),
          ]);
        }
      } catch (cause) { onError(cause); } finally { checking = false; }
    };
    const start = (immediate: boolean) => {
      if (timer !== null) window.clearInterval(timer);
      timer = null;
      if (document.visibilityState !== "visible") return;
      if (immediate) void check();
      timer = window.setInterval(() => void check(), 15000);
    };
    const visible = () => start(true);
    start(false);
    document.addEventListener("visibilitychange", visible);
    return () => { if (timer !== null) window.clearInterval(timer); document.removeEventListener("visibilitychange", visible); };
  }, [detail?.project.id, loadProject, memoryCache, onError, refreshProjects, selectedTask]);
  useEffect(() => {
    const url = new URL(location.href);
    const token = new URLSearchParams(url.hash.slice(1)).get("work_invite") ?? url.searchParams.get("work_invite");
    if (!token) return;
    url.searchParams.delete("work_invite");
    url.hash = "";
    history.replaceState(null, "", `${url.pathname}${url.search}`);
    void api.acceptWorkInvite(token).then((joined) => { const view:WorkProjectView={...joined,notes:[]};revisionRef.current=joined.project.contentRevision;detailRef.current=view;setDetail(view);setResourcesReady(false);setSelectedNote(null);setTab("overview");void refreshProjects().catch(onError);void refreshResources(joined.project.id).catch(onError); }).catch(onError);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  async function acknowledgeLocalChange(projectID: number, expectedDelta = 1) {
    const previousRevision = revisionRef.current;
    try {
      const value = await api.workProjectRevision(projectID);
      if (detailRef.current?.project.id !== projectID) return;
      if (value.revision !== previousRevision + expectedDelta) {
        await loadProject(projectID);
        return;
      }
      revisionRef.current = value.revision;
      setDetail((current)=>current?.project.id===projectID?{...current,project:{...current.project,contentRevision:value.revision}}:current);
    } catch (cause) { onError(cause); }
  }

  async function refreshResources(projectID: number, expectedDelta = 0) {
    const previousRevision = revisionRef.current;
    const resources = await api.workProjectResourceSummary(projectID);
    if (detailRef.current?.project.id !== projectID) return;
    if (resources.revision !== previousRevision + expectedDelta) {
      await loadProject(projectID);
      return;
    }
    revisionRef.current = resources.revision;
    setDetail((current)=>current?.project.id===projectID?{...current,project:{...current.project,contentRevision:resources.revision},comments:[],attachments:resources.attachments,links:resources.links,notes:resources.notes,events:resources.events}:current);
    resourcesReadyProjectsRef.current.add(projectID);
    setResourcesReady(true);
  }

  async function mutate(action: () => Promise<unknown>, projectID: number | null = detail?.project.id ?? null, scope: "core" | "resources" = "core") {
    if (busy) return false;
    setBusy(true);
    try {
      await action();
      if(projectID===null)await loadProjects(null);else{if(scope==="resources")await refreshResources(projectID,1);else await loadProject(projectID);void refreshProjects().catch(onError);}
      onChanged?.();
      return true;
    } catch (cause) {
      onError(cause);
      return false;
    } finally {
      setBusy(false);
    }
  }
  async function createProject(title: string) {
    if (!title.trim()) return;
    if (busy) return; setBusy(true);
    try { const created = await api.createWorkProject({ ...emptyProject, title: title.trim() });const view:WorkProjectView={...created,notes:[]};revisionRef.current=created.project.contentRevision;detailRef.current=view;setDetail(view);setResourcesReady(false);void refreshResources(created.project.id).catch(onError);void refreshProjects().catch(onError);setCreateOpen(false);setCreateTitle("");onChanged?.(); }
    catch (cause) { onError(cause); } finally { setBusy(false); }
  }
  async function saveProject(patch: Partial<WorkProjectInput>) {
    if (!detail) return; const p = detail.project;
    if (busy) return;setBusy(true);
    try { const updated=await api.updateWorkProject(p.id,{title:p.title,description:p.description,summary:p.summary,status:p.status,deadline:p.deadline,completed:p.completed,pinned:p.pinned,version:p.version,...patch});revisionRef.current=updated.project.contentRevision;setDetail((current)=>current?.project.id===p.id?{...current,project:updated.project,sections:updated.sections,tasks:updated.tasks}:current);void refreshProjects().catch(onError);onChanged?.(); }
    catch(cause){onError(cause);}finally{setBusy(false);}
  }
  function addTask(sectionId: number | null, parentId: number | null = null) {
    if (!detail || busy) return;
    setTaskCreateTitle("");
    setTaskCreateAssigneeID(null);
    setTaskCreateTarget({ sectionId: parentId ? null : sectionId, parentId });
  }
  async function submitTask() {
    if (!detail || !taskCreateTarget || !taskCreateTitle.trim() || busy) return;
    const isSubtask = taskCreateTarget.parentId !== null;
    setBusy(true);
    try {
      const projectID=detail.project.id;
      const created=await api.createWorkTask(projectID,{...taskCreateTarget,title:taskCreateTitle.trim(),description:"",assigneeId:taskCreateAssigneeID,dueDate:"",completed:false,sortOrder:0,version:0});
      setDetail((current)=>current?.project.id===projectID?{...current,tasks:[created,...current.tasks]}:current);
      void acknowledgeLocalChange(projectID);
      void refreshProjects().catch(onError);
      setTaskCreateTarget(null);
      setTaskCreateTitle("");
      setTaskCreateAssigneeID(null);
      if (!isSubtask && tab === "overview") setTab("list");
      onChanged?.();
    } catch (cause) { onError(cause); } finally { setBusy(false); }
  }
  function openSectionCreate(relativeTo: number | null = null, placement: SectionPlacement = "") {
    if (!detail || busy) return;
    setSectionCreateTitle("");
    setSectionCreateTarget({ relativeTo, placement });
  }
  async function submitSection() {
    if (!detail || !sectionCreateTarget || !sectionCreateTitle.trim() || busy) return;
    setBusy(true);
    try {
      const projectID=detail.project.id;
      const created=await api.createWorkSection(projectID,{title:sectionCreateTitle.trim(),relativeTo:sectionCreateTarget.relativeTo,placement:sectionCreateTarget.placement});
      setDetail((current)=>current?.project.id===projectID?{...current,sections:[...current.sections,created].sort((a,b)=>a.sortOrder-b.sortOrder||a.id-b.id)}:current);
      void acknowledgeLocalChange(projectID);
      setSectionCreateTarget(null);
      setSectionCreateTitle("");
      onChanged?.();
    } catch (cause) { onError(cause); } finally { setBusy(false); }
  }
  function openSectionRename(sectionID: number) {
    const section = detail?.sections.find((item)=>item.id===sectionID);
    if (!section || busy) return;
    setSectionRenameTarget(section);
    setSectionRenameTitle(section.title);
  }
  async function submitSectionRename() {
    if (!detail || !sectionRenameTarget || !sectionRenameTitle.trim() || busy) return;
    setBusy(true);
    try {
      const projectID=detail.project.id;
      const updated=await api.updateWorkSection(sectionRenameTarget.id,sectionRenameTitle.trim());
      setDetail((current)=>current?.project.id===projectID?{...current,sections:current.sections.map((section)=>section.id===updated.id?updated:section)}:current);
      void acknowledgeLocalChange(projectID);
      setSectionRenameTarget(null);
      setSectionRenameTitle("");
      onChanged?.();
    } catch (cause) { onError(cause); } finally { setBusy(false); }
  }
  function openSectionCompletion(sectionID: number) {
    const section = detail?.sections.find((item)=>item.id===sectionID);
    if (!section || busy) return;
    setSectionCompletionTarget(section);
    setSectionCompletionTargetID(section.completedSectionId);
    setSectionCompletionReset(section.resetCompletedOnMove);
    setSectionCompletionMoveToEnd(section.moveCompletedToEnd);
  }
  async function submitSectionCompletion() {
    if (!detail || !sectionCompletionTarget || busy) return;
    setBusy(true);
    try {
      const projectID=detail.project.id;
      const updated=await api.updateWorkSectionCompletion(sectionCompletionTarget.id,sectionCompletionTargetID,sectionCompletionReset,sectionCompletionMoveToEnd);
      setDetail((current)=>current?.project.id===projectID?{...current,sections:current.sections.map((section)=>section.id===updated.id?updated:section)}:current);
      void acknowledgeLocalChange(projectID);
      setSectionCompletionTarget(null);
      onChanged?.();
    } catch (cause) { onError(cause); } finally { setBusy(false); }
  }
  function toggleSection(sectionKey: string) {
    setCollapsedSections((current)=>{
      const next = new Set(current);
      if (next.has(sectionKey)) next.delete(sectionKey); else next.add(sectionKey);
      return next;
    });
  }
  async function updateTask(task: WorkTask, patch: Partial<WorkTaskInput>) {
    if (Object.prototype.hasOwnProperty.call(patch,"assigneeId")) {
      await updateTaskAssignee(task,patch.assigneeId??null);
      return;
    }
    if(!detail||busy)return;setBusy(true);
    try{const projectID=detail.project.id;const updated=await api.updateWorkTask(task.id,taskInput(task,patch));setDetail((current)=>current?.project.id===projectID?{...current,tasks:current.tasks.map((item)=>item.id===updated.id?updated:item)}:current);void acknowledgeLocalChange(projectID);void refreshProjects().catch(onError);onChanged?.();}
    catch(cause){onError(cause);}finally{setBusy(false);}
  }
  async function updateTaskAssignee(task: WorkTask, assigneeId: number | null) {
    if(!detail||assigneePendingTasks.has(task.id))return;
    const projectID=detail.project.id;
    const member=detail.project.members.find((item)=>item.userId===assigneeId);
    const optimistic={...task,assigneeId,assignee:member?.name??""};
    setAssigneePendingTasks((current)=>new Set(current).add(task.id));
    setDetail((current)=>current?.project.id===projectID?{...current,tasks:current.tasks.map((item)=>item.id===task.id?optimistic:item)}:current);
    try{
      const updated=await api.updateWorkTask(task.id,taskInput(task,{assigneeId}));
      setDetail((current)=>current?.project.id===projectID?{...current,tasks:current.tasks.map((item)=>item.id===updated.id?updated:item)}:current);
      void acknowledgeLocalChange(projectID);
      void refreshProjects().catch(onError);
      onChanged?.();
    }catch(cause){
      setDetail((current)=>current?.project.id===projectID?{...current,tasks:current.tasks.map((item)=>item.id===task.id?task:item)}:current);
      onError(cause);
    }finally{
      setAssigneePendingTasks((current)=>{const next=new Set(current);next.delete(task.id);return next;});
    }
  }
  async function claimTask(task: WorkTask) {
    if(!detail||assigneePendingTasks.has(task.id))return;
    const projectID=detail.project.id;
    setAssigneePendingTasks((current)=>new Set(current).add(task.id));
    try{
      const updated=await api.claimWorkTask(task.id);
      setDetail((current)=>current?.project.id===projectID?{...current,tasks:current.tasks.map((item)=>item.id===updated.id?updated:item)}:current);
      void acknowledgeLocalChange(projectID);
      void refreshProjects().catch(onError);
      onChanged?.();
    }catch(cause){
      onError(cause);
      void loadProject(projectID).catch(onError);
    }finally{
      setAssigneePendingTasks((current)=>{const next=new Set(current);next.delete(task.id);return next;});
    }
  }
  async function toggleTaskCompletion(task: WorkTask) {
    if (completionPendingTasks.has(task.id)) return;
    const completed = !task.completed;
    const sourceSection = detail?.sections.find((section)=>section.id===task.sectionId);
    const completedSectionId = completed && task.parentId === null
      ? sourceSection?.completedSectionId ?? task.sectionId
      : task.sectionId;
    const completedSortOrder = completed && task.parentId === null && sourceSection?.moveCompletedToEnd
      ? Math.max(0, ...(detail?.tasks.filter((item)=>item.parentId===null&&item.sectionId===completedSectionId&&item.id!==task.id).map((item)=>item.sortOrder) ?? [])) + 1
      : task.sortOrder;
    const optimisticCompleted = completed && !(task.parentId === null && sourceSection?.completedSectionId && sourceSection.resetCompletedOnMove);
    setCompletionPendingTasks((current)=>new Set(current).add(task.id));
    setDetail((current)=>current ? { ...current, tasks: current.tasks.map((item)=>item.id===task.id ? { ...item, completed: optimisticCompleted, sectionId: completedSectionId, sortOrder: completedSortOrder } : item) } : current);
    try {
      const updated = await api.updateWorkTask(task.id, taskInput(task, { completed }));
      setDetail((current)=>current ? { ...current, tasks: current.tasks.map((item)=>item.id===task.id ? updated : item) } : current);
      void acknowledgeLocalChange(task.projectId);
      void refreshProjects().catch(onError);
      onChanged?.();
    } catch (cause) {
      setDetail((current)=>current ? { ...current, tasks: current.tasks.map((item)=>item.id===task.id ? task : item) } : current);
      onError(cause);
    } finally {
      setCompletionPendingTasks((current)=>{const next=new Set(current);next.delete(task.id);return next;});
    }
  }
  async function createLink(title: string, url: string) {
    if (!detail || busy) return false;
    setBusy(true);
    try {
      const projectID=detail.project.id;
      const created=await api.createWorkLink(projectID,title,url);
      setDetail((current)=>current?.project.id===projectID?{...current,links:[...current.links,created]}:current);
      void acknowledgeLocalChange(projectID);
      onChanged?.();
      return true;
    } catch (cause) {
      onError(cause);
      return false;
    } finally { setBusy(false); }
  }
  async function createNote(input: WorkNoteInput) {
    if (!detail || busy) return null;
    const projectID = detail.project.id;
    setBusy(true);
    try {
      const created = await api.createWorkNote(projectID, input);
      memoryCache.notes.set(created.id, created);
      setOpenedNote(created);
      setNoteReady(true);
      setDetail((current)=>current?.project.id===projectID?{...current,notes:[noteSummary(created),...current.notes]}:current);
      void acknowledgeLocalChange(projectID);
      onChanged?.();
      return created;
    } catch (cause) {
      onError(cause);
      return null;
    } finally { setBusy(false); }
  }
  async function updateNote(note: WorkNote, input: WorkNoteInput) {
    if (!detail || busy) return null;
    const projectID = detail.project.id;
    setBusy(true);
    try {
      const updated = await api.updateWorkNote(note.id, input);
      memoryCache.notes.set(updated.id, updated);
      setOpenedNote(updated);
      setNoteReady(true);
      const summary = noteSummary(updated);
      setDetail((current)=>current?.project.id===projectID?{...current,notes:current.notes.map((item)=>item.id===updated.id?summary:item).sort((a,b)=>b.updatedAt.localeCompare(a.updatedAt)||b.id-a.id)}:current);
      void acknowledgeLocalChange(projectID);
      onChanged?.();
      return updated;
    } catch (cause) {
      onError(cause);
      void refreshResources(projectID).catch(onError);
      return null;
    } finally { setBusy(false); }
  }
  async function deleteNote(note: WorkNote) {
    if (!detail || busy) return false;
    const projectID = detail.project.id;
    setBusy(true);
    try {
      await api.deleteWorkNote(note.id);
      memoryCache.notes.delete(note.id);
      setOpenedNote(null);
      setDetail((current)=>current?.project.id===projectID?{...current,notes:current.notes.filter((item)=>item.id!==note.id)}:current);
      void acknowledgeLocalChange(projectID);
      setSelectedNote(null);
      onChanged?.();
      return true;
    } catch (cause) {
      onError(cause);
      return false;
    } finally { setBusy(false); }
  }
  function addSelectedFile(file: File, taskID: number | null = null) {
    if (!detail) return;
    if (file.size > MAX_WORK_ATTACHMENT_BYTES) {
      setOversizedFile(file);
      return;
    }
    const projectID=detail.project.id;
    void api.uploadWorkAttachment(projectID,taskID,file).then((created)=>{setDetail((current)=>current?.project.id===projectID?{...current,attachments:[...current.attachments,created]}:current);void acknowledgeLocalChange(projectID);}).catch(onError);
  }
  function suggestCloudLink() {
    if (!oversizedFile) return;
    setSelectedTask(null);
    setTab("overview");
    setLinkDialogRequest({ id: Date.now(), title: oversizedFile.name });
    setOversizedFile(null);
  }
  async function addComment(taskID: number, body: string) {
    if(!detail)return;
    const projectID=detail.project.id;
    const created=await api.createWorkComment(taskID,body);
    const next = [...(memoryCache.comments.get(taskID) ?? taskComments), created];
    memoryCache.comments.set(taskID, next);
    if (selectedTask === taskID) {
      setTaskComments(next);
      setTaskCommentsReady(true);
    }
    void acknowledgeLocalChange(projectID);
  }
  async function moveTask(task: WorkTask, direction: -1 | 1) {
    if (!detail) return; const siblings = detail.tasks.filter((v) => v.parentId === task.parentId && v.sectionId === task.sectionId).sort((a,b) => a.sortOrder-b.sortOrder); const index=siblings.findIndex((v)=>v.id===task.id); const other=siblings[index+direction]; if(!other)return;
    if(busy)return;setBusy(true);try{const updated=await Promise.all([api.updateWorkTask(task.id,taskInput(task,{sortOrder:other.sortOrder})),api.updateWorkTask(other.id,taskInput(other,{sortOrder:task.sortOrder}))]);const byID=new Map(updated.map((item)=>[item.id,item]));setDetail((current)=>current?{...current,tasks:current.tasks.map((item)=>byID.get(item.id)??item)}:current);void acknowledgeLocalChange(detail.project.id,2);onChanged?.();}catch(cause){onError(cause);}finally{setBusy(false);}
  }
  function taskRowPositions() {
    const positions = new Map<number, DOMRect>();
    document.querySelectorAll<HTMLElement>("[data-work-task-id]").forEach((row)=>positions.set(Number(row.dataset.workTaskId),row.getBoundingClientRect()));
    return positions;
  }
  function animateTaskRows(previous: Map<number, DOMRect>) {
    requestAnimationFrame(()=>document.querySelectorAll<HTMLElement>("[data-work-task-id]").forEach((row)=>{
      const before = previous.get(Number(row.dataset.workTaskId));
      if (!before) return;
      const after = row.getBoundingClientRect();
      const delta = before.top - after.top;
      if (Math.abs(delta) > 1) row.animate([{transform:`translateY(${delta}px)`},{transform:"translateY(0)"}],{duration:190,easing:"cubic-bezier(.2,.8,.2,1)"});
    }));
  }
  async function dropTask(target: WorkTask, placement: "before" | "after") {
    const sourceID = dragTaskIdRef.current ?? dragTaskId;
    if (!detail || sourceID === null || sort !== "manual") return;
    const source = detail.tasks.find((task) => task.id === sourceID);
    dragTaskIdRef.current = null;
    setDragTaskId(null);
    setTaskDropTarget(null);
    setSectionDropTargetKey(null);
    if (!source || source.id === target.id || source.parentId || target.parentId) return;

    const previousTasks = detail.tasks;
    const sourceSection = source.sectionId;
    const targetSection = target.sectionId;
    const sourceSiblings = detail.tasks.filter((task)=>!task.parentId&&task.sectionId===sourceSection&&task.id!==source.id).sort((a,b)=>a.sortOrder-b.sortOrder);
    const targetSiblings = sourceSection === targetSection ? sourceSiblings : detail.tasks.filter((task)=>!task.parentId&&task.sectionId===targetSection&&task.id!==source.id).sort((a,b)=>a.sortOrder-b.sortOrder);
    const targetIndex = targetSiblings.findIndex((task)=>task.id===target.id);
    if (targetIndex < 0) return;
    const insertIndex = targetIndex + (placement === "after" ? 1 : 0);
    targetSiblings.splice(insertIndex,0,{...source,sectionId:targetSection});

    const changes = new Map<number, Partial<WorkTaskInput>>();
    if (sourceSection !== targetSection) sourceSiblings.forEach((task,index)=>changes.set(task.id,{sortOrder:(index+1)*100}));
    targetSiblings.forEach((task,index)=>changes.set(task.id,{sectionId:targetSection,sortOrder:(index+1)*100}));
    const oldPositions = taskRowPositions();
    setDetail((current)=>current ? { ...current, tasks: current.tasks.map((task)=>changes.has(task.id)?{...task,...changes.get(task.id)}:task) } : current);
    animateTaskRows(oldPositions);
    try {
      const changedTasks = previousTasks.filter((task)=>changes.has(task.id));
      const updated = await Promise.all(changedTasks.map((task)=>api.updateWorkTask(task.id,taskInput(task,changes.get(task.id)))));
      const byID = new Map(updated.map((task)=>[task.id,task]));
      setDetail((current)=>current ? { ...current, tasks: current.tasks.map((task)=>byID.get(task.id)??task) } : current);
      void acknowledgeLocalChange(detail.project.id, changedTasks.length);
      onChanged?.();
    } catch (cause) {
      const rollbackPositions = taskRowPositions();
      setDetail((current)=>current ? { ...current, tasks: previousTasks } : current);
      animateTaskRows(rollbackPositions);
      onError(cause);
    }
  }
  async function dropTaskToSection(sectionId: number | null) {
    const sourceID = dragTaskIdRef.current ?? dragTaskId;
    if (!detail || sourceID === null || sort !== "manual") return;
    const source = detail.tasks.find((task) => task.id === sourceID);
    dragTaskIdRef.current = null;
    setDragTaskId(null);
    setTaskDropTarget(null);
    setSectionDropTargetKey(null);
    if (!source || source.parentId) return;

    const previousTasks = detail.tasks;
    const sourceSiblings = detail.tasks.filter((task)=>!task.parentId&&task.sectionId===source.sectionId&&task.id!==source.id).sort((a,b)=>a.sortOrder-b.sortOrder);
    const targetSiblings = source.sectionId === sectionId ? sourceSiblings : detail.tasks.filter((task)=>!task.parentId&&task.sectionId===sectionId&&task.id!==source.id).sort((a,b)=>a.sortOrder-b.sortOrder);
    targetSiblings.push({...source,sectionId});

    const changes = new Map<number, Partial<WorkTaskInput>>();
    if (source.sectionId !== sectionId) sourceSiblings.forEach((task,index)=>changes.set(task.id,{sortOrder:(index+1)*100}));
    targetSiblings.forEach((task,index)=>changes.set(task.id,{sectionId,sortOrder:(index+1)*100}));
    const oldPositions = taskRowPositions();
    setDetail((current)=>current ? { ...current, tasks: current.tasks.map((task)=>changes.has(task.id)?{...task,...changes.get(task.id)}:task) } : current);
    animateTaskRows(oldPositions);
    try {
      const changedTasks = previousTasks.filter((task)=>changes.has(task.id));
      const updated = await Promise.all(changedTasks.map((task)=>api.updateWorkTask(task.id,taskInput(task,changes.get(task.id)))));
      const byID = new Map(updated.map((task)=>[task.id,task]));
      setDetail((current)=>current ? { ...current, tasks: current.tasks.map((task)=>byID.get(task.id)??task) } : current);
      void acknowledgeLocalChange(detail.project.id, changedTasks.length);
      onChanged?.();
    } catch (cause) {
      const rollbackPositions = taskRowPositions();
      setDetail((current)=>current ? { ...current, tasks: previousTasks } : current);
      animateTaskRows(rollbackPositions);
      onError(cause);
    }
  }
  const selected = detail?.tasks.find((task) => task.id === selectedTask) ?? null;
  const visibleTasks = useMemo(() => {
    if (!detail) return []; let rows = detail.tasks.filter((task) => !task.parentId && task.title.toLocaleLowerCase().includes(taskSearch.toLocaleLowerCase()));
    if (assigneeFilter) rows=rows.filter((task)=>assigneeFilter==="none"?task.assigneeId===null:String(task.assigneeId??"")===assigneeFilter); if(doneFilter!=="all")rows=rows.filter((task)=>task.completed===(doneFilter==="done"));
    const now=new Date();const today=new Date(now.getTime()-now.getTimezoneOffset()*60000).toISOString().slice(0,10);
    const weekday=(now.getDay()+6)%7;const weekStart=new Date(now);weekStart.setHours(0,0,0,0);weekStart.setDate(now.getDate()-weekday);const nextWeekStart=new Date(weekStart);nextWeekStart.setDate(weekStart.getDate()+7);const weekAfterNext=new Date(nextWeekStart);weekAfterNext.setDate(nextWeekStart.getDate()+7);const dateKey=(value:Date)=>new Date(value.getTime()-value.getTimezoneOffset()*60000).toISOString().slice(0,10);const currentWeekStart=dateKey(weekStart);const followingWeekStart=dateKey(nextWeekStart);const followingWeekEnd=dateKey(weekAfterNext);
    if(dueFilter==="today")rows=rows.filter((task)=>task.dueDate===today);if(dueFilter==="overdue")rows=rows.filter((task)=>!!task.dueDate&&task.dueDate<today&&!task.completed);if(dueFilter==="none")rows=rows.filter((task)=>!task.dueDate);if(dueFilter==="this_week")rows=rows.filter((task)=>task.dueDate>=currentWeekStart&&task.dueDate<followingWeekStart);if(dueFilter==="next_week")rows=rows.filter((task)=>task.dueDate>=followingWeekStart&&task.dueDate<followingWeekEnd);
    if(sort==="manual")rows.sort((a,b)=>a.sortOrder-b.sortOrder);else {const factor=sortDirection==="asc"?1:-1;rows.sort((a,b)=>{let value=0;if(sort==="title")value=a.title.localeCompare(b.title,"ru");if(sort==="due"){if(!a.dueDate||!b.dueDate)return Number(!a.dueDate)-Number(!b.dueDate);value=a.dueDate.localeCompare(b.dueDate)}if(sort==="assignee"){if(!a.assignee||!b.assignee)return Number(!a.assignee)-Number(!b.assignee);value=a.assignee.localeCompare(b.assignee,"ru")}if(sort==="created")value=a.createdAt.localeCompare(b.createdAt);if(sort==="updated")value=a.updatedAt.localeCompare(b.updatedAt);if(sort==="completed")value=Number(a.completed)-Number(b.completed);return value*factor||a.sortOrder-b.sortOrder})}return rows;
  }, [detail, taskSearch, assigneeFilter, doneFilter, dueFilter, sort, sortDirection]);
  const groups = useMemo(() => {
    if (!detail) return [] as WorkTaskGroup[];
    const sections: WorkTaskGroup[] = detail.sections.map((s)=>({key:String(s.id),title:s.title,sectionId:s.id,tasks:visibleTasks.filter((t)=>t.sectionId===s.id)})); const loose=visibleTasks.filter((t)=>!t.sectionId);if(loose.length)sections.push({key:"none",title:"Без раздела",sectionId:null,tasks:loose});return sections;
  }, [detail, visibleTasks]);
  const filteredProjects=projects.filter((p)=>p.title.toLocaleLowerCase().includes(projectSearch.toLocaleLowerCase()));
  const mainTasks = detail?.tasks.filter((task) => !task.parentId) ?? [];
  const taskCount = mainTasks.length;
  const completedCount = mainTasks.filter((task) => task.completed === true).length;
  const deadlineLabel = detail ? formatDeadline(detail.project.deadline) : "";
  const headerMembers = detail ? [...detail.project.members].sort((a,b)=>Number(b.userId===currentUserId)-Number(a.userId===currentUserId)).slice(0,4) : [];

  function beginTaskDrag(taskID: number) {
    dragTaskIdRef.current = taskID;
    setDragTaskId(taskID);
    setSectionDropTargetKey(null);
  }

  function endTaskDrag() {
    dragTaskIdRef.current = null;
    setDragTaskId(null);
    setTaskDropTarget(null);
    setSectionDropTargetKey(null);
  }

  async function shareProject() {
    if (!detail || inviteCreating) return;
    setInviteCreating(true);
    try {
      const invite = await api.createWorkInvite(detail.project.id);
      setInviteLink(invite.token);
      setInviteCopied(false);
    } catch (cause) { onError(cause); }
    finally { setInviteCreating(false); }
  }

  async function copyInviteLink() {
    if (!inviteLink) return;
    try {
      let copied = false;
      if (navigator.clipboard?.writeText) {
        try {
          await navigator.clipboard.writeText(inviteLink);
          copied = true;
        } catch { /* Fall back to selection-based copying below. */ }
      }
      if (!copied) {
        const input = inviteInputRef.current;
        if (!input) throw new Error("Поле со ссылкой недоступно");
        input.focus();
        input.select();
        if (!document.execCommand("copy")) throw new Error("Браузер не разрешил копирование");
      }
      setInviteCopied(true);
    } catch {
      inviteInputRef.current?.focus();
      inviteInputRef.current?.select();
      onError(new Error("Не удалось скопировать ссылку автоматически. Скопируйте выделенную ссылку вручную."));
    }
  }

  function closeProjectMenu(restoreFocus = true) {
    setProjectMenuOpen(false);
    if (restoreFocus) requestAnimationFrame(()=>projectMenuButtonRef.current?.focus());
  }

  return <section className="workPage">
    {projectMenuOpen && <button type="button" className="workSidebarBackdrop" aria-label="Закрыть список проектов" onClick={()=>closeProjectMenu()}/>}
    <aside className={`workSidebar ${projectMenuOpen ? "mobileOpen" : ""}`} aria-label="Проекты">
      <div className="workSidebarTitle"><strong>Работа</strong><span><button className="workSidebarAdd" type="button" onClick={()=>setCreateOpen(true)} aria-label="Создать проект">＋</button><button ref={projectMenuCloseRef} className="workSidebarClose" type="button" onClick={()=>closeProjectMenu()} aria-label="Закрыть список проектов"><WorkMobileIcon name="close"/></button></span></div>
      <div className="workProjectSearchField"><img src={workSearchIcon} alt=""/><input className="workSearch" value={projectSearch} onChange={(e)=>setProjectSearch(e.target.value)} placeholder="Поиск проектов" aria-label="Поиск проектов" />{projectSearch && <button type="button" aria-label="Очистить поиск проектов" onClick={()=>setProjectSearch("")}>×</button>}</div>
      <div className="workProjectList">{filteredProjects.map((project)=><button key={project.id} className={detail?.project.id===project.id?"active":""} onClick={()=>{setSelectedTask(null);setSelectedNote(null);setProjectMenuOpen(false);showProjectImmediately(project);void loadProject(project.id,project.contentRevision).catch(onError)}}><span>{project.title}</span>{project.taskCount > 0 && <small>{safePercent(project.completedCount, project.taskCount)}%</small>}</button>)}</div>
      <button type="button" className="workNewProject" onClick={()=>setCreateOpen(true)}>＋ Новый проект</button>
    </aside>
    {!detail ? <div className="workEmpty">{initialLoading ? <><h1>Загружаем проекты</h1><p>Подождите немного…</p></> : <><h1>Рабочих проектов пока нет</h1><p>Создайте проект, добавьте задачи и пригласите участников.</p><button type="button" onClick={()=>setCreateOpen(true)}>Создать проект</button></>}</div> : <div className={`workMain ${selected?"withDrawer":""}`}>
      <header className="workHeader">
        <button ref={projectMenuButtonRef} type="button" className="workMobileProjectMenuButton" aria-label="Открыть список проектов" aria-expanded={projectMenuOpen} onClick={()=>setProjectMenuOpen(true)}><WorkMobileIcon name="menu"/></button>
        <div className="workHeading">
          <div className="workHeadingLine"><h1>{detail.project.title}</h1><span className={`workStatus ${detail.project.status}`}><i aria-hidden="true"/>{statusLabel[detail.project.status]}</span></div>
          <p>{taskCount === 0 ? "В проекте пока нет задач" : `${completedCount} из ${taskCount} задач выполнено`}{deadlineLabel && ` · Срок: ${deadlineLabel}`}</p>
        </div>
        <div className="workHeaderActions">
          <div className="workAvatars">{headerMembers.map((member)=>member.userId===currentUserId
            ? <button type="button" className="workProfileAvatarButton" key={member.userId} title="Настроить профиль в проектах" aria-label="Настроить профиль в проектах" onClick={()=>setProfileOpen(true)}><WorkAvatar name={member.name} avatar={member.avatar} className="workHeaderAvatar"/></button>
            : <WorkAvatar key={member.userId} name={member.name} avatar={member.avatar} className="workHeaderAvatar" title={member.name}/>)}</div>
          {detail.project.role === "owner" && <button className="workShareButton" type="button" disabled={inviteCreating} onClick={()=>void shareProject()}><span>{inviteCreating ? "Создаём…" : "Поделиться"}</span><WorkMobileIcon name="share"/></button>}
        </div>
        <nav><button className={tab==="overview"?"active":""} onClick={()=>setTab("overview")}>Обзор</button><button className={tab==="notes"?"active":""} onClick={()=>setTab("notes")}>Заметки</button><button className={tab==="list"?"active":""} onClick={()=>setTab("list")}>Список</button><button className={tab==="board"?"active":""} onClick={()=>setTab("board")}>Доска</button></nav>
      </header>
      <div hidden={tab!=="notes"}><ProjectNotes detail={detail} resourcesReady={resourcesReady} busy={busy} selectedNote={selectedNote} note={openedNote} noteReady={noteReady} onSelectNote={selectWorkNote} onCreate={createNote} onUpdate={updateNote} onDelete={deleteNote}/></div>
      {tab==="overview" ? <ProjectOverview detail={detail} resourcesReady={resourcesReady} busy={busy} inviteCreating={inviteCreating} linkDialogRequest={linkDialogRequest} onLinkDialogConsumed={()=>setLinkDialogRequest(null)} onShare={shareProject} onSave={saveProject} onMutate={mutate} onAddFile={()=>fileRef.current?.click()} onAddLink={createLink} onOpenNote={(id)=>{selectWorkNote(id);setTab("notes")}} onCreateNote={()=>{selectWorkNote("new");setTab("notes")}}/> : tab!=="notes" ? <div className={`workList workList-${tab}`}>
        <WorkListToolbar detail={detail} currentUserId={currentUserId} taskSearch={taskSearch} assigneeFilter={assigneeFilter} doneFilter={doneFilter} dueFilter={dueFilter} sort={sort} sortDirection={sortDirection} onTaskSearch={setTaskSearch} onAssigneeFilter={setAssigneeFilter} onDoneFilter={setDoneFilter} onDueFilter={setDueFilter} onSort={setSort} onSortDirection={setSortDirection} onAddTask={()=>addTask(detail.sections[0]?.id??null)} onAddSection={()=>openSectionCreate()}/>
        {tab==="list" ? <div className="workTable">
          <div className="workTableHead"><span/><span>Название</span><span>Исполнитель</span><span>Срок выполнения</span><span>Создано</span></div>
          {groups.map((group)=><WorkSectionGroup key={group.key} group={group} detail={detail} currentUserId={currentUserId} sort={sort} collapsed={collapsedSections.has(group.key)} draggingTaskId={dragTaskId} dropTarget={taskDropTarget} sectionDropTargetKey={sectionDropTargetKey} completionPendingTasks={completionPendingTasks} assigneePendingTasks={assigneePendingTasks} onToggle={()=>toggleSection(group.key)} onAddTask={()=>addTask(group.sectionId)} onSelectTask={selectWorkTask} onTaskContextMenu={openTaskContextMenu} onUpdateTask={updateTask} onClaimTask={claimTask} onToggleTask={toggleTaskCompletion} onDragTask={beginTaskDrag} onDragEnd={endTaskDrag} onDragOver={(target)=>{setSectionDropTargetKey(null);setTaskDropTarget(target)}} onDragOverSection={(key)=>{setTaskDropTarget(null);setSectionDropTargetKey(key)}} onDropTask={dropTask} onDropSection={dropTaskToSection} onMoveTask={moveTask} onRename={openSectionRename} onConfigureCompletion={openSectionCompletion} onAddRelative={openSectionCreate} onDelete={(sectionID)=>{if(confirm("Удалить раздел? Задачи останутся в проекте без раздела."))void mutate(()=>api.deleteWorkSection(sectionID))}}/>)}
          <button className="workAddSection" onClick={()=>openSectionCreate()}>＋ Добавить раздел</button>
        </div> : <WorkBoard groups={groups} detail={detail} sort={sort} draggingTaskId={dragTaskId} dropTarget={taskDropTarget} sectionDropTargetKey={sectionDropTargetKey} completionPendingTasks={completionPendingTasks} onAddTask={addTask} onAddSection={()=>openSectionCreate()} onSelectTask={selectWorkTask} onTaskContextMenu={openTaskContextMenu} onToggleTask={toggleTaskCompletion} onDragTask={beginTaskDrag} onDragEnd={endTaskDrag} onDragOver={(target)=>{setSectionDropTargetKey(null);setTaskDropTarget(target)}} onDragOverSection={(key)=>{setTaskDropTarget(null);setSectionDropTargetKey(key)}} onDropTask={dropTask} onDropSection={dropTaskToSection}/>}
      </div> : null}
      {selected&&<TaskDrawer task={selected} detail={detail} resourcesReady={resourcesReady} comments={taskComments} commentsReady={taskCommentsReady} completionPendingTasks={completionPendingTasks} assigneePending={assigneePendingTasks.has(selected.id)} onClose={()=>setSelectedTask(null)} onUpdate={updateTask} onToggleComplete={toggleTaskCompletion} onRequestDelete={requestTaskDelete} onAddSubtask={()=>addTask(null,selected.id)} onAddComment={addComment} onError={onError} onAddFile={(file)=>addSelectedFile(file,selected.id)}/>}<input ref={fileRef} type="file" hidden onChange={(e)=>{const file=e.currentTarget.files?.[0];if(file)addSelectedFile(file);e.currentTarget.value=""}}/>
      {!selected && (tab === "list" || tab === "board") && <div className="workMobileBottomAction"><button type="button" onClick={()=>addTask(detail.sections[0]?.id??null)}><WorkMobileIcon name="plus"/>Добавить задачу</button></div>}
    </div>}
    {taskContextMenu&&<TaskContextMenu state={taskContextMenu} onClose={()=>setTaskContextMenu(null)} onDelete={(button)=>requestTaskDelete(taskContextMenu.task,button)}/>}
    {taskDeleteTarget&&<TaskDeleteDialog task={taskDeleteTarget} busy={busy} onClose={closeTaskDeleteDialog} onConfirm={confirmTaskDelete}/>}
    {profileOpen && <ProjectProfileDialog login={currentUserLogin} profile={profile} onClose={()=>setProfileOpen(false)} onSaved={async()=>{await onProfileChanged();await loadProjects(detail?.project.id)}}/>}
    {inviteLink && <div className="workCreateBackdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget)setInviteLink("")}}>
      <form className="workCreateDialog workInviteDialog" noValidate onSubmit={(event)=>{event.preventDefault();void copyInviteLink()}} aria-labelledby="work-invite-title">
        <header><div><span>Приглашение в проект</span><h2 id="work-invite-title">Добавить участников</h2></div><button type="button" onClick={()=>setInviteLink("")} aria-label="Закрыть">×</button></header>
        <p>Отправьте эту ссылку человеку, которого хотите добавить. Приглашение действует 7 дней.</p>
        <label>Ссылка приглашения<input ref={inviteInputRef} autoFocus readOnly value={inviteLink} onFocus={(event)=>event.currentTarget.select()} /></label>
        <div className="workCreateActions"><button type="button" onClick={()=>setInviteLink("")}>Закрыть</button><button type="submit">{inviteCopied ? "Скопировано" : "Скопировать ссылку"}</button></div>
      </form>
    </div>}
    {createOpen && <div className="workCreateBackdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget&&!busy)setCreateOpen(false)}}>
      <form className="workCreateDialog" noValidate onSubmit={(event)=>{event.preventDefault();void createProject(createTitle)}} aria-labelledby="work-create-title">
        <header><div><span>Новый проект</span><h2 id="work-create-title">Что будем делать?</h2></div><button type="button" onClick={()=>setCreateOpen(false)} disabled={busy} aria-label="Закрыть">×</button></header>
        <label>Название проекта<input autoFocus maxLength={160} value={createTitle} onChange={(event)=>setCreateTitle(event.target.value)} placeholder="Например, Запуск нового сайта" /></label>
        <div className="workCreateActions"><button type="button" onClick={()=>setCreateOpen(false)} disabled={busy}>Отмена</button><button type="submit" disabled={busy||!createTitle.trim()}>{busy?"Создаём…":"Создать проект"}</button></div>
      </form>
    </div>}
    {taskCreateTarget && <div className="workCreateBackdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget&&!busy)setTaskCreateTarget(null)}}>
      <form className="workCreateDialog workTaskCreateDialog" noValidate onSubmit={(event)=>{event.preventDefault();void submitTask()}} aria-labelledby="work-task-create-title">
        <header><div><span>{taskCreateTarget.parentId ? "Новая подзадача" : "Новая задача"}</span><h2 id="work-task-create-title">{taskCreateTarget.parentId ? "Добавить подзадачу" : "Добавить задачу"}</h2></div><button type="button" onClick={()=>setTaskCreateTarget(null)} disabled={busy} aria-label="Закрыть">×</button></header>
        <label>Название<input autoFocus maxLength={240} value={taskCreateTitle} onChange={(event)=>setTaskCreateTitle(event.target.value)} placeholder={taskCreateTarget.parentId ? "Название подзадачи" : "Что нужно сделать?"} /></label>
        <label>Исполнитель<select value={taskCreateAssigneeID??""} onChange={(event)=>setTaskCreateAssigneeID(event.target.value ? Number(event.target.value) : null)}><option value="">Не назначен</option>{detail?.project.members.map((member)=><option key={member.userId} value={member.userId}>{member.name}</option>)}</select></label>
        <div className="workCreateActions"><button type="button" onClick={()=>setTaskCreateTarget(null)} disabled={busy}>Отмена</button><button type="submit" disabled={busy||!taskCreateTitle.trim()}>{busy?"Создаём…":"Создать"}</button></div>
      </form>
    </div>}
    {sectionCreateTarget && <div className="workCreateBackdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget&&!busy)setSectionCreateTarget(null)}}>
      <form className="workCreateDialog workSectionCreateDialog" noValidate onSubmit={(event)=>{event.preventDefault();void submitSection()}} aria-labelledby="work-section-create-title">
        <header><div><span>Новый раздел</span><h2 id="work-section-create-title">{sectionCreateTarget.placement === "above" ? "Добавить раздел выше" : sectionCreateTarget.placement === "below" ? "Добавить раздел ниже" : "Добавить раздел"}</h2></div><button type="button" onClick={()=>setSectionCreateTarget(null)} disabled={busy} aria-label="Закрыть">×</button></header>
        <label>Название<input autoFocus maxLength={120} value={sectionCreateTitle} onChange={(event)=>setSectionCreateTitle(event.target.value)} placeholder="Например, В работе" /></label>
        <div className="workCreateActions"><button type="button" onClick={()=>setSectionCreateTarget(null)} disabled={busy}>Отмена</button><button type="submit" disabled={busy||!sectionCreateTitle.trim()}>{busy?"Создаём…":"Создать раздел"}</button></div>
      </form>
    </div>}
    {sectionRenameTarget && <div className="workCreateBackdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget&&!busy)setSectionRenameTarget(null)}}>
      <form className="workCreateDialog workSectionCreateDialog" noValidate onSubmit={(event)=>{event.preventDefault();void submitSectionRename()}} aria-labelledby="work-section-rename-title">
        <header><div><span>Раздел</span><h2 id="work-section-rename-title">Переименовать раздел</h2></div><button type="button" onClick={()=>setSectionRenameTarget(null)} disabled={busy} aria-label="Закрыть">×</button></header>
        <label>Название<input autoFocus maxLength={120} value={sectionRenameTitle} onChange={(event)=>setSectionRenameTitle(event.target.value)} /></label>
        <div className="workCreateActions"><button type="button" onClick={()=>setSectionRenameTarget(null)} disabled={busy}>Отмена</button><button type="submit" disabled={busy||!sectionRenameTitle.trim()}>{busy?"Сохраняем…":"Сохранить"}</button></div>
      </form>
    </div>}
    {sectionCompletionTarget && <div className="workCreateBackdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget&&!busy)setSectionCompletionTarget(null)}}>
      <form className="workCreateDialog workSectionCompletionDialog" noValidate onSubmit={(event)=>{event.preventDefault();void submitSectionCompletion()}} aria-labelledby="work-section-completion-title">
        <header><div><span>Раздел «{sectionCompletionTarget.title}»</span><h2 id="work-section-completion-title">Поведение выполненных задач</h2></div><button type="button" onClick={()=>setSectionCompletionTarget(null)} disabled={busy} aria-label="Закрыть">×</button></header>
        <p>Настройте, куда и в какое место перемещать задачи сразу после выполнения.</p>
        <label>Раздел назначения<select autoFocus value={sectionCompletionTargetID ?? ""} onChange={(event)=>setSectionCompletionTargetID(event.target.value ? Number(event.target.value) : null)}><option value="">Оставлять в текущем разделе</option>{detail?.sections.filter((section)=>section.id!==sectionCompletionTarget.id).map((section)=><option key={section.id} value={section.id}>{section.title}</option>)}</select></label>
        <fieldset className="workSectionCompletionState" disabled={sectionCompletionTargetID===null}>
          <legend>Состояние после переноса</legend>
          <label><input type="radio" name="completion-state" checked={!sectionCompletionReset} onChange={()=>setSectionCompletionReset(false)}/><span><strong>Оставить выполненной</strong><small>Задача сохранит отметку выполнения</small></span></label>
          <label><input type="radio" name="completion-state" checked={sectionCompletionReset} onChange={()=>setSectionCompletionReset(true)}/><span><strong>Сбросить выполнение</strong><small>В новом разделе задача снова станет активной</small></span></label>
        </fieldset>
        <fieldset className="workSectionCompletionState workSectionCompletionOrder">
          <legend>Положение в разделе</legend>
          <label><input type="checkbox" checked={sectionCompletionMoveToEnd} onChange={(event)=>setSectionCompletionMoveToEnd(event.target.checked)}/><span><strong>Переносить в конец раздела</strong><small>После выполнения задача окажется ниже остальных задач</small></span></label>
        </fieldset>
        <div className="workCreateActions"><button type="button" onClick={()=>setSectionCompletionTarget(null)} disabled={busy}>Отмена</button><button type="submit" disabled={busy}>{busy?"Сохраняем…":"Сохранить"}</button></div>
      </form>
    </div>}
    {oversizedFile && <div className="workCreateBackdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget)setOversizedFile(null)}}>
      <div className="workCreateDialog workOversizedDialog" role="dialog" aria-modal="true" aria-labelledby="work-file-too-large-title">
        <header><div><span>Основные ресурсы</span><h2 id="work-file-too-large-title">Файл больше 50 МБ</h2></div><button type="button" onClick={()=>setOversizedFile(null)} aria-label="Закрыть">×</button></header>
        <p><strong>{oversizedFile.name}</strong> нельзя загрузить в проект. Загрузите файл на Яндекс Диск или Google Диск и добавьте ссылку.</p>
        <div className="workCreateActions"><button type="button" onClick={()=>setOversizedFile(null)}>Отмена</button><button type="button" onClick={suggestCloudLink}>Добавить ссылку</button></div>
      </div>
    </div>}
  </section>;
}

function WorkBoard({ groups, detail, sort, draggingTaskId, dropTarget, sectionDropTargetKey, completionPendingTasks, onAddTask, onAddSection, onSelectTask, onTaskContextMenu, onToggleTask, onDragTask, onDragEnd, onDragOver, onDragOverSection, onDropTask, onDropSection }: {
  groups: WorkTaskGroup[];
  detail: WorkProjectView;
  sort: WorkSort;
  draggingTaskId: number | null;
  dropTarget: TaskDropTarget | null;
  sectionDropTargetKey: string | null;
  completionPendingTasks: Set<number>;
  onAddTask: (sectionId: number | null) => void;
  onAddSection: () => void;
  onSelectTask: (id: number) => void;
  onTaskContextMenu: (task: WorkTask, x: number, y: number) => void;
  onToggleTask: (task: WorkTask) => Promise<void>;
  onDragTask: (id: number) => void;
  onDragEnd: () => void;
  onDragOver: (target: TaskDropTarget) => void;
  onDragOverSection: (key: string | null) => void;
  onDropTask: (task: WorkTask, placement: "before" | "after") => Promise<void>;
  onDropSection: (sectionId: number | null) => Promise<void>;
}) {
  return <div className="workBoard" aria-label="Доска задач">
    {groups.map((group)=><section
      className={`workBoardColumn ${sectionDropTargetKey===group.key ? "drop-section" : ""}`}
      key={group.key}
      onDragOver={(event)=>{if(sort!=="manual"||draggingTaskId===null)return;event.preventDefault();onDragOverSection(group.key)}}
      onDragLeave={(event)=>{if(!event.currentTarget.contains(event.relatedTarget as Node))onDragOverSection(null)}}
      onDrop={(event)=>{if(sort!=="manual"||draggingTaskId===null)return;event.preventDefault();void onDropSection(group.sectionId)}}
    >
      <header><strong>{group.title}</strong><span>{group.tasks.length}</span></header>
      <div className="workBoardCards">
        {group.tasks.map((task)=>{
          const target=dropTarget?.id===task.id?dropTarget:null;
          const member=detail.project.members.find((item)=>item.userId===task.assigneeId);
          return <article
            className={`workBoardCard ${task.completed?"done":""} ${draggingTaskId===task.id?"dragging":""} ${target?`drop-${target.placement}`:""}`}
            data-work-task-id={task.id}
            draggable={sort==="manual"}
            key={task.id}
            onContextMenu={(event)=>{event.preventDefault();onTaskContextMenu(task,event.clientX,event.clientY)}}
            onDragStart={(event)=>{if(sort!=="manual")return;event.dataTransfer.effectAllowed="move";event.dataTransfer.setData("text/plain",String(task.id));onDragTask(task.id)}}
            onDragEnd={onDragEnd}
            onDragOver={(event)=>{
              if(sort!=="manual"||draggingTaskId===null||draggingTaskId===task.id)return;
              event.preventDefault();event.stopPropagation();
              const rect=event.currentTarget.getBoundingClientRect();
              onDragOver({id:task.id,placement:event.clientY<rect.top+rect.height/2?"before":"after"});
            }}
            onDrop={(event)=>{if(sort!=="manual"||draggingTaskId===null||draggingTaskId===task.id)return;event.preventDefault();event.stopPropagation();const rect=event.currentTarget.getBoundingClientRect();void onDropTask(task,event.clientY<rect.top+rect.height/2?"before":"after")}}
          >
            <div className="workBoardCardTitle">
              <button className={`workBoardCheck ${task.completed?"done":""}`} disabled={completionPendingTasks.has(task.id)} onClick={()=>void onToggleTask(task)} aria-label={task.completed ? "Вернуть задачу в работу" : "Завершить задачу"}><img className="normal" src={task.completed?workTaskCompleteIcon:workTaskOpenIcon} alt=""/><img className="hover" src={workTaskCompleteHoverIcon} alt=""/></button>
              <button className="workBoardTaskTitle" data-work-task-open={task.id} aria-haspopup="menu" onClick={()=>onSelectTask(task.id)} onKeyDown={(event)=>{if(event.key!=="ContextMenu"&&!(event.shiftKey&&event.key==="F10"))return;event.preventDefault();const rect=event.currentTarget.getBoundingClientRect();onTaskContextMenu(task,rect.left,rect.bottom+4)}}>{task.title}</button>
            </div>
            {(task.assignee || task.dueDate) && <footer>
              {task.assignee && <WorkAvatar name={task.assignee} avatar={member?.avatar || ""} className="workBoardAvatar" title={task.assignee}/>} {task.dueDate && <time className={taskDueIsOverdue(task)?"overdue":""} dateTime={task.dueDate}>{formatTaskDate(task.dueDate)}</time>}
            </footer>}
          </article>;
        })}
      </div>
      <button type="button" className="workBoardAddTask" onClick={()=>onAddTask(group.sectionId)}>＋ Добавить задачу</button>
    </section>)}
    <button type="button" className="workBoardAddSection" onClick={onAddSection}>＋ Добавить раздел</button>
  </div>;
}

function WorkSectionGroup({ group, detail, currentUserId, sort, collapsed, draggingTaskId, dropTarget, sectionDropTargetKey, completionPendingTasks, assigneePendingTasks, onToggle, onAddTask, onSelectTask, onTaskContextMenu, onUpdateTask, onClaimTask, onToggleTask, onDragTask, onDragEnd, onDragOver, onDragOverSection, onDropTask, onDropSection, onMoveTask, onRename, onConfigureCompletion, onAddRelative, onDelete }: {
  group: WorkTaskGroup;
  detail: WorkProjectView;
  currentUserId: number;
  sort: WorkSort;
  collapsed: boolean;
  draggingTaskId: number | null;
  dropTarget: TaskDropTarget | null;
  sectionDropTargetKey: string | null;
  completionPendingTasks: Set<number>;
  assigneePendingTasks: Set<number>;
  onToggle: () => void;
  onAddTask: () => void;
  onSelectTask: (id: number) => void;
  onTaskContextMenu: (task: WorkTask, x: number, y: number) => void;
  onUpdateTask: (task: WorkTask, patch: Partial<WorkTaskInput>) => Promise<void>;
  onClaimTask: (task: WorkTask) => Promise<void>;
  onToggleTask: (task: WorkTask) => Promise<void>;
  onDragTask: (id: number) => void;
  onDragEnd: () => void;
  onDragOver: (target: TaskDropTarget) => void;
  onDragOverSection: (key: string | null) => void;
  onDropTask: (task: WorkTask, placement: "before" | "after") => Promise<void>;
  onDropSection: (sectionId: number | null) => Promise<void>;
  onMoveTask: (task: WorkTask, direction: -1 | 1) => Promise<void>;
  onRename: (sectionID: number) => void;
  onConfigureCompletion: (sectionID: number) => void;
  onAddRelative: (sectionID: number, placement: SectionPlacement) => void;
  onDelete: (sectionID: number) => void;
}) {
  function closeMenu(target: HTMLElement) {
    target.closest("details")?.removeAttribute("open");
  }
  return <section className={`workSectionGroup ${collapsed ? "collapsed" : ""} ${sectionDropTargetKey===group.key ? "drop-section" : ""}`}>
    <div
      className="workSectionHeader"
      onDragOver={(event)=>{if(sort!=="manual"||draggingTaskId===null)return;event.preventDefault();onDragOverSection(group.key)}}
      onDragLeave={(event)=>{if(!event.currentTarget.contains(event.relatedTarget as Node))onDragOverSection(null)}}
      onDrop={(event)=>{if(sort!=="manual"||draggingTaskId===null)return;event.preventDefault();void onDropSection(group.sectionId)}}
    >
      <button type="button" className="workSectionToggle" onClick={onToggle} aria-expanded={!collapsed} aria-label={collapsed ? `Развернуть раздел ${group.title}` : `Свернуть раздел ${group.title}`}><span aria-hidden="true"/></button>
      <strong>{group.title}</strong>
      <span className="workSectionCount">{group.tasks.length}</span>
      {group.sectionId !== null && <div className="workSectionActions">
        <button type="button" className="workSectionQuickAdd" onClick={onAddTask} aria-label={`Добавить задачу в раздел ${group.title}`}/>
        <details className="workSectionMenu">
          <summary aria-label={`Действия с разделом ${group.title}`}><span/><span/><span/></summary>
          <div className="workSectionMenuPanel">
            <button type="button" onClick={(event)=>{closeMenu(event.currentTarget);onRename(group.sectionId!)}}><img src={workSectionRenameIcon} alt=""/>Переименовать раздел</button>
            <button type="button" onClick={(event)=>{closeMenu(event.currentTarget);onConfigureCompletion(group.sectionId!)}}><span className="workSectionCompletionIcon" aria-hidden="true">✓</span>Настроить поведение выполненных задач</button>
            <div className="workSectionInsert">
              <button type="button" aria-haspopup="menu" onClick={(event)=>event.currentTarget.focus()}><img src={workSectionAddIcon} alt=""/>Добавить раздел<span>›</span></button>
              <div className="workSectionInsertMenu">
                <button type="button" onClick={(event)=>{closeMenu(event.currentTarget);onAddRelative(group.sectionId!,"above")}}><span>↑</span>Добавить раздел выше</button>
                <button type="button" onClick={(event)=>{closeMenu(event.currentTarget);onAddRelative(group.sectionId!,"below")}}><span>↓</span>Добавить раздел ниже</button>
              </div>
            </div>
            <button type="button" className="danger" onClick={(event)=>{closeMenu(event.currentTarget);onDelete(group.sectionId!)}}><img src={workSectionDeleteIcon} alt=""/>Удалить раздел</button>
          </div>
        </details>
      </div>}
    </div>
    {!collapsed && <>
      {group.tasks.map((task)=>{
        const target=dropTarget?.id===task.id?dropTarget:null;
        return <div
          className={`workTaskRow ${task.completed?"done":""} ${draggingTaskId===task.id?"dragging":""} ${target?`drop-${target.placement}`:""}`}
          data-work-task-id={task.id}
          key={task.id}
          onContextMenu={(event)=>{event.preventDefault();onTaskContextMenu(task,event.clientX,event.clientY)}}
          onDragOver={(event)=>{
            if(sort!=="manual"||draggingTaskId===null||draggingTaskId===task.id)return;
            event.preventDefault();event.stopPropagation();
            const rect=event.currentTarget.getBoundingClientRect();
            onDragOver({id:task.id,placement:event.clientY<rect.top+rect.height/2?"before":"after"});
          }}
          onDrop={(event)=>{if(sort!=="manual"||draggingTaskId===null||draggingTaskId===task.id)return;event.preventDefault();event.stopPropagation();const rect=event.currentTarget.getBoundingClientRect();void onDropTask(task,event.clientY<rect.top+rect.height/2?"before":"after")}}
        >
          <span
            className={`workDragHandle ${sort!=="manual"?"disabled":""}`}
            role="button"
            tabIndex={sort==="manual"?0:-1}
            draggable={sort==="manual"}
            aria-label={`Переместить задачу ${task.title}`}
            onDragStart={(event)=>{event.dataTransfer.effectAllowed="move";event.dataTransfer.setData("text/plain",String(task.id));onDragTask(task.id)}}
            onDragEnd={onDragEnd}
            onKeyDown={(event)=>{if(event.key==="ArrowUp"){event.preventDefault();void onMoveTask(task,-1)}if(event.key==="ArrowDown"){event.preventDefault();void onMoveTask(task,1)}}}
          ><i/><i/><i/><i/><i/><i/></span>
          <button className={`workCheck ${task.completed?"done":""}`} disabled={completionPendingTasks.has(task.id)} onClick={()=>void onToggleTask(task)} aria-label={task.completed ? "Вернуть задачу в работу" : "Завершить задачу"}><img className="normal" src={task.completed?workTaskCompleteIcon:workTaskOpenIcon} alt=""/><img className="hover" src={workTaskCompleteHoverIcon} alt=""/></button>
          <button className="workTaskTitle" data-work-task-open={task.id} aria-haspopup="menu" onClick={()=>onSelectTask(task.id)} onKeyDown={(event)=>{if(event.key!=="ContextMenu"&&!(event.shiftKey&&event.key==="F10"))return;event.preventDefault();const rect=event.currentTarget.getBoundingClientRect();onTaskContextMenu(task,rect.left,rect.bottom+4)}}>{task.title}</button>
          <TaskAssigneeCell task={task} detail={detail} currentUserId={currentUserId} pending={assigneePendingTasks.has(task.id)} onUpdate={onUpdateTask} onClaim={onClaimTask}/>
          <TaskDueCell task={task} onUpdate={onUpdateTask}/>
          <time className="workTaskCreated" dateTime={task.createdAt}>{formatTaskDate(task.createdAt)}</time>
        </div>;
      })}
      <button className="workAddRow" onClick={onAddTask}>＋ Добавить задачу</button>
    </>}
  </section>;
}

function TaskAssigneeCell({ task, detail, currentUserId, pending, onUpdate, onClaim }: { task: WorkTask; detail: WorkProjectView; currentUserId: number; pending: boolean; onUpdate: (task: WorkTask, patch: Partial<WorkTaskInput>) => Promise<void>; onClaim: (task: WorkTask) => Promise<void> }) {
  const member = detail.project.members.find((item)=>item.userId===task.assigneeId);
  if (task.assigneeId === null && task.createdById !== currentUserId) {
    return <div className="workTaskAssignee empty claimable">
      <button type="button" className="workTaskClaim" aria-busy={pending} disabled={pending} onClick={()=>void onClaim(task)}><img src={workTaskAssigneeEmptyIcon} alt=""/><span>{pending ? "Берём…" : "Взять задачу"}</span></button>
    </div>;
  }
  return <label className={`workTaskAssignee ${task.assignee ? "assigned" : "empty"}`}>
    {task.assignee ? <WorkAvatar name={task.assignee} avatar={member?.avatar || ""} className="workTaskAvatar"/> : <img src={workTaskAssigneeEmptyIcon} alt=""/>}
    {task.assignee && <span className="workTaskAssigneeName">{task.assignee}</span>}
    <select aria-label={`Исполнитель задачи ${task.title}`} aria-busy={pending} disabled={pending} value={task.assigneeId??""} onChange={(event)=>void onUpdate(task,{assigneeId:event.target.value?Number(event.target.value):null})}><option value="">Не назначен</option>{detail.project.members.map((item)=><option key={item.userId} value={item.userId}>{item.name}</option>)}</select>
  </label>;
}

function TaskDueCell({ task, onUpdate }: { task: WorkTask; onUpdate: (task: WorkTask, patch: Partial<WorkTaskInput>) => Promise<void> }) {
  const inputRef = useRef<HTMLInputElement>(null);
  function openPicker() {
    const input = inputRef.current;
    if (!input) return;
    try {
      if (typeof input.showPicker === "function") input.showPicker();
      else { input.focus(); input.click(); }
    } catch {
      input.focus();
      input.click();
    }
  }
  return <div className={`workTaskDue ${taskDueIsOverdue(task)?"overdue":""} ${task.dueDate?"filled":"empty"}`}>
    <button type="button" onClick={openPicker} aria-label={task.dueDate ? `Изменить срок выполнения задачи ${task.title}` : `Добавить срок выполнения задачи ${task.title}`}>{task.dueDate ? <span>{formatTaskDate(task.dueDate)}</span> : <img src={workTaskDueEmptyIcon} alt=""/>}</button>
    <input ref={inputRef} tabIndex={-1} aria-hidden="true" type="date" value={task.dueDate} onChange={(event)=>void onUpdate(task,{dueDate:event.target.value})}/>
  </div>;
}

function WorkListToolbar({ detail, currentUserId, taskSearch, assigneeFilter, doneFilter, dueFilter, sort, sortDirection, onTaskSearch, onAssigneeFilter, onDoneFilter, onDueFilter, onSort, onSortDirection, onAddTask, onAddSection }: {
  detail: WorkProjectView;
  currentUserId: number;
  taskSearch: string;
  assigneeFilter: string;
  doneFilter: string;
  dueFilter: string;
  sort: WorkSort;
  sortDirection: SortDirection;
  onTaskSearch: (value: string) => void;
  onAssigneeFilter: (value: string) => void;
  onDoneFilter: (value: string) => void;
  onDueFilter: (value: string) => void;
  onSort: (value: WorkSort) => void;
  onSortDirection: (value: SortDirection) => void;
  onAddTask: () => void;
  onAddSection: () => void;
}) {
  const [open, setOpen] = useState<"add" | "filter" | "sort" | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const toolbarRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const filterCount = Number(doneFilter !== "all") + Number(assigneeFilter !== "") + Number(dueFilter !== "all");
  const sortOptions: { value: WorkSort; label: string; icon: string }[] = [
    { value: "manual", label: "Ручной порядок", icon: workOrderIcon },
    { value: "due", label: "Срок выполнения", icon: workDueIcon },
    { value: "assignee", label: "Исполнитель", icon: workAssigneeIcon },
    { value: "created", label: "Дата создания", icon: workClockIcon },
    { value: "updated", label: "Последнее изменение", icon: workClockIcon },
    { value: "completed", label: "Выполнение", icon: workCompletionIcon },
    { value: "title", label: "По алфавиту", icon: workSortIcon },
  ];

  useEffect(() => {
    const close = (event: PointerEvent) => {
      if (!toolbarRef.current?.contains(event.target as Node)) setOpen(null);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") { setOpen(null); setSearchOpen(false); }
    };
    document.addEventListener("pointerdown", close);
    document.addEventListener("keydown", escape);
    return () => { document.removeEventListener("pointerdown", close); document.removeEventListener("keydown", escape); };
  }, []);
  useEffect(() => {
    if (searchOpen) searchRef.current?.focus();
  }, [searchOpen]);

  function clearFilters() {
    onDoneFilter("all");
    onAssigneeFilter("");
    onDueFilter("all");
  }
  function toggleDone(value: "todo" | "done") {
    onDoneFilter(doneFilter === value ? "all" : value);
  }
  function toggleDue(value: "this_week" | "next_week") {
    onDueFilter(dueFilter === value ? "all" : value);
  }
  const taskCount = detail.tasks.filter((task)=>task.parentId === null).length;

  return <div ref={toolbarRef} className="workToolbar">
    <span className="workMobileTaskCount">{taskCount} {pluralizeCount(taskCount,"задача","задачи","задач")}</span>
    <div className="workAddTaskControl">
      <button type="button" className="workAddTaskMain" onClick={onAddTask}>＋ Добавить задачу</button>
      <button type="button" className="workAddTaskMore" aria-label="Другие варианты добавления" aria-expanded={open === "add"} onClick={()=>setOpen(open === "add" ? null : "add")}>⌄</button>
      {open === "add" && <div className="workToolbarPopover workAddPopover"><button type="button" onClick={()=>{setOpen(null);onAddTask()}}>＋ Задачу</button><button type="button" onClick={()=>{setOpen(null);onAddSection()}}>＋ Раздел</button></div>}
    </div>
    <div className="workToolbarActions">
      <div className="workToolbarControl workToolbarFilterControl">
        <button type="button" className={`workToolbarTrigger ${filterCount ? "active" : ""}`} aria-expanded={open === "filter"} onClick={()=>setOpen(open === "filter" ? null : "filter")}><img src={workFilterIcon} alt=""/><span>Фильтр</span>{filterCount > 0 && <b>{filterCount}</b>}</button>
        {open === "filter" && <div className="workToolbarPopover workFilterPopover">
          <header><strong>Фильтры</strong><button type="button" disabled={filterCount === 0} onClick={clearFilters}>Очистить</button></header>
          <p>Быстрые фильтры</p>
          <div className="workQuickFilters">
            <button type="button" className={doneFilter === "todo" ? "active" : ""} onClick={()=>toggleDone("todo")}><img src={workCompletionIcon} alt=""/>Незавершённые задачи</button>
            <button type="button" className={doneFilter === "done" ? "active" : ""} onClick={()=>toggleDone("done")}><img src={workCompletionIcon} alt=""/>Завершённые задачи</button>
            <button type="button" className={assigneeFilter === String(currentUserId) ? "active" : ""} onClick={()=>onAssigneeFilter(assigneeFilter === String(currentUserId) ? "" : String(currentUserId))}><img src={workAssigneeIcon} alt=""/>Только мои задачи</button>
            <button type="button" className={dueFilter === "this_week" ? "active" : ""} onClick={()=>toggleDue("this_week")}><img src={workDueIcon} alt=""/>Срок — на этой неделе</button>
            <button type="button" className={dueFilter === "next_week" ? "active" : ""} onClick={()=>toggleDue("next_week")}><img src={workDueIcon} alt=""/>Срок — на следующей неделе</button>
          </div>
          <div className="workFilterFields">
            <label><span><img src={workCompletionIcon} alt=""/>Статус выполнения</span><select value={doneFilter} onChange={(event)=>onDoneFilter(event.target.value)}><option value="all">Все задачи</option><option value="todo">Незавершённые</option><option value="done">Завершённые</option></select></label>
            <label><span><img src={workAssigneeIcon} alt=""/>Исполнитель</span><select value={assigneeFilter} onChange={(event)=>onAssigneeFilter(event.target.value)}><option value="">Все исполнители</option><option value="none">Без исполнителя</option>{detail.project.members.map((member)=><option key={member.userId} value={member.userId}>{member.name}</option>)}</select></label>
            <label><span><img src={workDueIcon} alt=""/>Срок выполнения</span><select value={dueFilter} onChange={(event)=>onDueFilter(event.target.value)}><option value="all">Любой срок</option><option value="today">Сегодня</option><option value="this_week">На этой неделе</option><option value="next_week">На следующей неделе</option><option value="overdue">Просроченные</option><option value="none">Без срока</option></select></label>
          </div>
        </div>}
      </div>
      <div className="workToolbarControl workToolbarSortControl">
        <button type="button" className={`workToolbarTrigger ${sort !== "manual" ? "active" : ""}`} aria-expanded={open === "sort"} onClick={()=>setOpen(open === "sort" ? null : "sort")}><img src={workSortIcon} alt=""/><span>Сортировка</span></button>
        {open === "sort" && <div className="workToolbarPopover workSortPopover">
          <strong>Сортировка</strong>
          <div>{sortOptions.map((option)=><button type="button" className={sort === option.value ? "active" : ""} key={option.value} onClick={()=>{onSort(option.value);if(option.value === "manual")setOpen(null)}}><img src={option.icon} alt=""/><span>{option.label}</span>{sort === option.value && <b>✓</b>}</button>)}</div>
          {sort !== "manual" && <button type="button" className="workSortDirection" onClick={()=>onSortDirection(sortDirection === "asc" ? "desc" : "asc")}><img src={workSortIcon} alt=""/>{sortDirection === "asc" ? "По возрастанию" : "По убыванию"}</button>}
        </div>}
      </div>
      {searchOpen || taskSearch ? <div className="workToolbarSearch"><img src={workSearchIcon} alt=""/><input ref={searchRef} value={taskSearch} onChange={(event)=>onTaskSearch(event.target.value)} placeholder="Поиск задач"/><button type="button" aria-label="Закрыть поиск" onClick={()=>{onTaskSearch("");setSearchOpen(false)}}>×</button></div> : <button type="button" className="workToolbarIconButton workToolbarSearchButton" aria-label="Поиск задач" onClick={()=>setSearchOpen(true)}><img src={workSearchIcon} alt=""/></button>}
    </div>
  </div>;
}

function ProjectNotes({ detail, resourcesReady, busy, selectedNote, note, noteReady, onSelectNote, onCreate, onUpdate, onDelete }: {
  detail: WorkProjectView;
  resourcesReady: boolean;
  busy: boolean;
  selectedNote: number | "new" | null;
  note: WorkNote | null;
  noteReady: boolean;
  onSelectNote: (id: number | "new" | null) => void;
  onCreate: (input: WorkNoteInput) => Promise<WorkNote | null>;
  onUpdate: (note: WorkNote, input: WorkNoteInput) => Promise<WorkNote | null>;
  onDelete: (note: WorkNote) => Promise<boolean>;
}) {
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [attempted, setAttempted] = useState(false);
  const [saved, setSaved] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const deleteButtonRef = useRef<HTMLButtonElement>(null);
  const cancelDeleteRef = useRef<HTMLButtonElement>(null);
  const confirmDeleteRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    setTitle(note?.title ?? "");
    setBody(note?.body ?? "");
    setAttempted(false);
    setSaved(false);
  }, [detail.project.id, selectedNote, note?.version]);
  useEffect(() => {
    if (!deleteOpen) return;
    cancelDeleteRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setDeleteOpen(false);
        requestAnimationFrame(()=>deleteButtonRef.current?.focus());
      } else if (event.key === "Tab") {
        if (event.shiftKey && document.activeElement === cancelDeleteRef.current) {
          event.preventDefault();
          confirmDeleteRef.current?.focus();
        } else if (!event.shiftKey && document.activeElement === confirmDeleteRef.current) {
          event.preventDefault();
          cancelDeleteRef.current?.focus();
        }
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [deleteOpen]);
  const dirty = selectedNote === "new" ? Boolean(title || body) : Boolean(note && (title !== note.title || body !== note.body));
  useEffect(() => {
    if (!dirty) return;
    const guard = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ""; };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [dirty]);
  async function saveNote(event: FormEvent) {
    event.preventDefault();
    setAttempted(true);
    const cleanTitle = title.trim();
    if (!cleanTitle) return;
    const input = { title: cleanTitle, body, version: note?.version ?? 0 };
    const savedNote = note ? await onUpdate(note, input) : await onCreate(input);
    if (!savedNote) return;
    setTitle(savedNote.title);
    setBody(savedNote.body);
    setSaved(true);
    setAttempted(false);
    onSelectNote(savedNote.id);
  }
  function closeEditor(discard: boolean) {
    if (dirty && !discard) return;
    if (discard) {
      setTitle(note?.title ?? "");
      setBody(note?.body ?? "");
      setAttempted(false);
      setSaved(false);
    }
    onSelectNote(null);
  }
  if (selectedNote === null) return <section className="workNotes">
    <header className="workNotesHeader"><div><h2>Общие заметки</h2><p>Текстовые документы доступны всем участникам проекта.</p></div><button type="button" disabled={!resourcesReady} onClick={()=>onSelectNote("new")}>＋ Новая заметка</button></header>
    {!resourcesReady ? <div className="workNotesState"><strong>Загружаем заметки…</strong><span>Задачи и обзор уже доступны</span></div> : detail.notes.length === 0 ? <div className="workNotesState"><span className="workNotesEmptyIcon"><img src={workResourceNoteIcon} alt=""/></span><strong>Заметок пока нет</strong><span>Создайте первый общий текстовый файл проекта</span><button type="button" onClick={()=>onSelectNote("new")}>Создать заметку</button></div> : <div className="workNoteList">{detail.notes.map((item)=><button type="button" key={item.id} onClick={()=>onSelectNote(item.id)}><span className="workResourceType note"><img src={workResourceNoteIcon} alt=""/></span><span className="workNoteListCopy"><strong>{item.title}</strong><small>{item.preview || "Пустая заметка"}</small></span><time dateTime={item.updatedAt}>{formatActivityDate(item.updatedAt)}</time><span aria-hidden="true">›</span></button>)}</div>}
  </section>;
  if (typeof selectedNote === "number" && !noteReady) return <section className="workNotes"><div className="workNotesState"><strong>Открываем заметку…</strong><span>Загружаем полный текст</span></div></section>;
  if (typeof selectedNote === "number" && !note) return <section className="workNotes"><div className="workNotesState"><strong>Не удалось открыть заметку</strong><button type="button" onClick={()=>onSelectNote(null)}>Вернуться к списку</button></div></section>;
  return <section className="workNoteEditor">
    <div className="workNoteEditorBar"><button type="button" className="workNoteBack" disabled={dirty} title={dirty?"Сохраните или отмените изменения":undefined} onClick={()=>closeEditor(false)}>← Все заметки</button><span>{saved ? "Сохранено" : dirty ? "Есть несохранённые изменения" : note ? `Изменено ${formatActivityDate(note.updatedAt)}` : "Новая общая заметка"}</span></div>
    <form noValidate onSubmit={(event)=>void saveNote(event)}>
      <label className="workNoteTitle"><span>Название заметки</span><input autoFocus maxLength={160} value={title} onChange={(event)=>{setTitle(event.target.value);setSaved(false)}} aria-invalid={attempted&&!title.trim()} aria-describedby={attempted&&!title.trim()?"work-note-title-error":undefined} placeholder="Например, Решения по проекту"/></label>
      {attempted&&!title.trim()&&<p className="workNoteError" id="work-note-title-error" role="alert">Введите название заметки.</p>}
      <label className="workNoteBody"><span>Текст заметки</span><textarea className="resize-none" maxLength={100000} value={body} onChange={(event)=>{setBody(event.target.value);setSaved(false)}} placeholder="Запишите договорённости, идеи или важный контекст проекта"/></label>
      <div className="workNoteActions">{note&&<button ref={deleteButtonRef} className="danger" type="button" disabled={busy} onClick={()=>setDeleteOpen(true)}>Удалить заметку</button>}<span/><button type="button" disabled={busy} onClick={()=>closeEditor(dirty)}>{dirty?"Отменить изменения":"Закрыть"}</button><button type="submit" disabled={busy||!dirty}>{busy?"Сохраняем…":"Сохранить"}</button></div>
    </form>
    {deleteOpen&&note&&<div className="workCreateBackdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget){setDeleteOpen(false);requestAnimationFrame(()=>deleteButtonRef.current?.focus())}}}><div className="workCreateDialog workNoteDeleteDialog" role="alertdialog" aria-modal="true" aria-labelledby="work-note-delete-title" aria-describedby="work-note-delete-description"><header><div><span>Общая заметка</span><h2 id="work-note-delete-title">Удалить «{note.title}»?</h2></div></header><p id="work-note-delete-description">Заметка исчезнет у всех участников проекта. Это действие нельзя отменить.</p><div className="workCreateActions"><button ref={cancelDeleteRef} type="button" disabled={busy} onClick={()=>{setDeleteOpen(false);requestAnimationFrame(()=>deleteButtonRef.current?.focus())}}>Отмена</button><button ref={confirmDeleteRef} className="danger" type="button" disabled={busy} onClick={()=>void onDelete(note).then((deleted)=>{if(deleted)setDeleteOpen(false)})}>{busy?"Удаляем…":"Удалить заметку"}</button></div></div></div>}
  </section>;
}

function ProjectOverview({ detail, resourcesReady, busy, inviteCreating, linkDialogRequest, onLinkDialogConsumed, onShare, onSave, onMutate, onAddFile, onAddLink, onOpenNote, onCreateNote }: {
  detail: WorkProjectView;
  resourcesReady: boolean;
  busy: boolean;
  inviteCreating: boolean;
  linkDialogRequest: LinkDialogRequest | null;
  onLinkDialogConsumed: () => void;
  onShare: () => Promise<void>;
  onSave: (patch: Partial<WorkProjectInput>) => Promise<void>;
  onMutate: (action: () => Promise<unknown>, projectID?: number | null, scope?: "core" | "resources") => Promise<boolean>;
  onAddFile: () => void;
  onAddLink: (title: string, url: string) => Promise<boolean>;
  onOpenNote: (id: number) => void;
  onCreateNote: () => void;
}) {
  const [linkOpen, setLinkOpen] = useState(false);
  const [linkTitle, setLinkTitle] = useState("");
  const [linkURL, setLinkURL] = useState("");
  const projectFiles = detail.attachments.filter((attachment) => !attachment.taskId);
  const projectLinks = detail.links ?? [];
  const projectNotes = detail.notes ?? [];
  const hasResources = projectFiles.length > 0 || projectLinks.length > 0 || projectNotes.length > 0;
  const memberJoinedAt = (userID: number, role: "owner" | "member") => detail.events.find((event)=>event.kind === "member_joined" && event.actorId === userID)?.createdAt || (role === "owner" ? detail.project.createdAt : "");
  useEffect(() => {
    if (!linkDialogRequest) return;
    setLinkTitle(linkDialogRequest.title);
    setLinkOpen(true);
    onLinkDialogConsumed();
  }, [linkDialogRequest?.id]); // eslint-disable-line react-hooks/exhaustive-deps
  return <div className="workOverview">
    <main>
      <EditableProjectDescription detail={detail} onSave={onSave} />

      <section className="workOverviewSection">
        <div className="workSectionHeading"><h2>Участники</h2>{detail.project.role === "owner" && <button type="button" disabled={inviteCreating} onClick={()=>void onShare()}>{inviteCreating ? "Создаём…" : "＋ Добавить"}</button>}</div>
        <div className="workMembers">{detail.project.members.map((member)=><div key={member.userId}><WorkAvatar name={member.name} avatar={member.avatar} className="workMemberAvatar"/><strong>{member.name}</strong><small>{member.role === "owner" ? "Владелец" : "Участник"}</small>{detail.project.role === "owner" && member.role === "member" && <button type="button" onClick={()=>void onMutate(()=>api.removeWorkMember(detail.project.id,member.userId))}>Удалить</button>}</div>)}</div>
      </section>

      <section className="workOverviewSection">
        <div className="workSectionHeading"><h2>Основные ресурсы</h2>{resourcesReady&&<details className="workResourceMenu"><summary>＋ Добавить</summary><div><button type="button" onClick={onCreateNote}>Создать заметку</button><button type="button" onClick={onAddFile}>Добавить файл</button><button type="button" onClick={()=>setLinkOpen(true)}>Добавить ссылку</button></div></details>}</div>
        <div className={`workResources ${hasResources ? "hasFiles" : ""}`}>{!resourcesReady ? <><strong>Загружаем ресурсы…</strong><p>Основная часть проекта уже доступна</p></> : hasResources ? <>{projectNotes.map((note)=><div className="workResourceRow note" key={`note-${note.id}`}><span className="workResourceType note"><img src={workResourceNoteIcon} alt=""/></span><div className="workResourceCopy"><strong>{note.title}</strong><small>Общая заметка · {formatActivityDate(note.updatedAt)}</small></div><button className="workResourceAction" type="button" onClick={()=>onOpenNote(note.id)} aria-label={`Открыть заметку ${note.title}`}><img src={workResourceOpenIcon} alt=""/></button></div>)}{projectFiles.map((attachment)=><div className="workResourceRow" key={`file-${attachment.id}`}><span className="workResourceType file"><img src={workResourceFileIcon} alt=""/></span><div className="workResourceCopy"><strong>{attachment.name}</strong><small>Файл · {formatFileSize(attachment.size)}</small></div><a className="workResourceAction" href={`/api/work/attachments/${attachment.id}`} aria-label={`Открыть файл ${attachment.name}`}><img src={workResourceOpenIcon} alt=""/></a><i className="workResourceDivider"/><button className="workResourceDelete" type="button" aria-label={`Удалить файл ${attachment.name}`} onClick={()=>{if(confirm("Удалить файл из проекта?"))void onMutate(()=>api.deleteWorkAttachment(attachment.id),detail.project.id,"resources")}}><img src={workResourceRemoveIcon} alt=""/></button></div>)}{projectLinks.map((link)=><div className="workResourceRow" key={`link-${link.id}`}><span className="workResourceType link"><img src={workResourceLinkIcon} alt=""/></span><div className="workResourceCopy"><strong>{link.title}</strong><small>{link.url}</small></div><a className="workResourceAction" href={link.url} target="_blank" rel="noreferrer" aria-label={`Открыть ссылку ${link.title}`}><img src={workResourceOpenIcon} alt=""/></a><i className="workResourceDivider"/><button className="workResourceDelete" type="button" aria-label={`Удалить ссылку ${link.title}`} onClick={()=>{if(confirm("Удалить ссылку из проекта?"))void onMutate(()=>api.deleteWorkLink(link.id),detail.project.id,"resources")}}><img src={workResourceRemoveIcon} alt=""/></button></div>)}</> : <><strong>Здесь пока нет ресурсов</strong><p>Создайте общую заметку, добавьте документ или полезную ссылку</p><div className="workResourceEmptyActions"><button type="button" onClick={onCreateNote}>＋ Создать заметку</button><button type="button" onClick={onAddFile}>＋ Добавить файл</button><button type="button" onClick={()=>setLinkOpen(true)}>＋ Добавить ссылку</button></div></>}</div>
      </section>
    </main>
    <aside className="workProjectInfo">
      {detail.project.role === "owner" ? <>
        <h2 className="workInfoTitle">Статус проекта</h2>
        <div className="workStatusButtons" aria-label="Статус проекта">{(["on_track","at_risk","off_track"] as const).map((status)=><button type="button" key={status} className={`${status} ${detail.project.status === status ? "active" : ""}`} disabled={busy} onClick={()=>void onSave({status})}><span>●</span>{statusLabel[status]}</button>)}</div>
        <details className="workProjectMenu workProjectInfoMenu"><summary aria-label="Действия с проектом">•••</summary><div><button type="button" onClick={()=>void onSave({completed:!detail.project.completed})}>{detail.project.completed ? "Вернуть проект в работу" : "Завершить проект"}</button><button className="danger" type="button" onClick={()=>{if(confirm("Удалить проект без возможности восстановления?"))void onMutate(()=>api.deleteWorkProject(detail.project.id),null)}}>Удалить проект</button></div></details>
        <ProjectDeadline project={detail.project} onSave={onSave}/>
      </> : <>
        <ProjectDeadline project={detail.project}/>
        <details className="workProjectMenu workProjectInfoMenu workMemberMenu"><summary aria-label="Действия с проектом">•••</summary><div><button className="danger" type="button" onClick={()=>{if(confirm("Покинуть проект?"))void onMutate(()=>api.leaveWorkProject(detail.project.id),null)}}>Покинуть проект</button></div></details>
      </>}
      <section className="workActivity"><div className="workEvents workMemberEvents">{detail.project.members.map((member)=>{const joinedAt=memberJoinedAt(member.userId,member.role);return <div key={member.userId}><span className="workEventIcon"><img src={workMembersIcon} alt=""/></span><p><strong>{member.role === "owner" ? "Владелец проекта" : `Присоединился: ${member.name}`}</strong><em>{member.name}{joinedAt && <time>{formatActivityDate(joinedAt)}</time>}</em><WorkAvatar name={member.name} avatar={member.avatar} className="workEventActor" title={member.name}/></p></div>})}<div><span className="workEventIcon"><img src={workProjectCreatedIcon} alt=""/></span><p><strong>Проект создан</strong><em>{detail.project.members.find((member)=>member.role === "owner")?.name || "Владелец"}{formatActivityDate(detail.project.createdAt) && <time>{formatActivityDate(detail.project.createdAt)}</time>}</em></p></div></div></section>
    </aside>
    {linkOpen && <div className="workCreateBackdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget&&!busy)setLinkOpen(false)}}><form className="workCreateDialog workLinkDialog" noValidate onSubmit={async(event)=>{event.preventDefault();if(await onAddLink(linkTitle,linkURL)){setLinkOpen(false);setLinkTitle("");setLinkURL("")}}} aria-labelledby="work-link-title"><header><div><span>Новый ресурс</span><h2 id="work-link-title">Добавить ссылку</h2></div><button type="button" onClick={()=>setLinkOpen(false)} disabled={busy} aria-label="Закрыть">×</button></header><label>Название<input autoFocus maxLength={200} value={linkTitle} onChange={(event)=>setLinkTitle(event.target.value)} placeholder="Например, Документация" /></label><label>Адрес ссылки<input type="url" required maxLength={2048} value={linkURL} onChange={(event)=>setLinkURL(event.target.value)} placeholder="https://example.com" /></label><div className="workCreateActions"><button type="button" onClick={()=>setLinkOpen(false)} disabled={busy}>Отмена</button><button type="submit" disabled={busy||!linkURL.trim()}>{busy?"Добавляем…":"Добавить"}</button></div></form></div>}
  </div>;
}

function ProjectDeadline({ project, onSave }: { project: WorkProject; onSave?: (patch: Partial<WorkProjectInput>) => Promise<void> }) {
  const text = project.deadline ? `Срок выполнения: до ${formatDeadline(project.deadline)}` : "Нет срока выполнения";
  if (!onSave) return <div className="workDeadlineControl readonly"><img src={workCalendarIcon} alt=""/><span>{text}</span></div>;
  return <EditableProjectDeadline project={project} text={text} onSave={onSave}/>;
}

function EditableProjectDeadline({ project, text, onSave }: { project: WorkProject; text: string; onSave: (patch: Partial<WorkProjectInput>) => Promise<void> }) {
  const inputRef = useRef<HTMLInputElement>(null);
  function openPicker() {
    const input = inputRef.current;
    if (!input) return;
    try {
      if (typeof input.showPicker === "function") input.showPicker();
      else { input.focus(); input.click(); }
    } catch {
      input.focus();
      input.click();
    }
  }
  return <div className="workDeadlinePicker"><button className="workDeadlineControl editable" type="button" onClick={openPicker} aria-label={project.deadline ? `Изменить срок: ${text}` : "Добавить срок выполнения"}><img src={workCalendarIcon} alt=""/><span>{text}</span></button><input ref={inputRef} className="workDeadlineInput" tabIndex={-1} aria-hidden="true" type="date" value={project.deadline} onChange={(event)=>void onSave({deadline:event.target.value})}/></div>;
}

function EditableProjectDescription({ detail, onSave }: { detail: WorkProjectView; onSave: (patch: Partial<WorkProjectInput>) => Promise<void> }) {
  const [value, setValue] = useState(detail.project.description);
  useEffect(()=>setValue(detail.project.description), [detail.project.id, detail.project.description]);
  return <label className="workDescriptionEditor">Описание проекта<textarea className="resize-none" value={value} onChange={(event)=>setValue(event.target.value)} onBlur={()=>{if(value !== detail.project.description)void onSave({description:value})}} placeholder="Расскажите, к какому результату ведёт проект" /></label>;
}

function TaskContextMenu({ state, onClose, onDelete }: { state: TaskContextMenuState; onClose: () => void; onDelete: (button: HTMLButtonElement) => void }) {
  const menuRef = useRef<HTMLDivElement>(null);
  const deleteButtonRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    deleteButtonRef.current?.focus();
    const closeOutside = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) onClose();
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    const close = () => onClose();
    document.addEventListener("pointerdown", closeOutside);
    document.addEventListener("keydown", closeOnEscape);
    window.addEventListener("resize", close);
    window.addEventListener("scroll", close, true);
    return () => {
      document.removeEventListener("pointerdown", closeOutside);
      document.removeEventListener("keydown", closeOnEscape);
      window.removeEventListener("resize", close);
      window.removeEventListener("scroll", close, true);
    };
  }, [onClose]);
  return <div ref={menuRef} className="workTaskContextMenu" role="menu" aria-label={`Действия с задачей ${state.task.title}`} style={{ left: state.x, top: state.y }}>
    <button ref={deleteButtonRef} type="button" role="menuitem" onClick={(event)=>onDelete(event.currentTarget)}><img src={workSectionDeleteIcon} alt=""/>Удалить задачу</button>
  </div>;
}

function TaskDeleteDialog({ task, busy, onClose, onConfirm }: { task: WorkTask; busy: boolean; onClose: () => void; onConfirm: () => Promise<void> }) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const confirmRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    cancelRef.current?.focus();
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) {
        event.preventDefault();
        onClose();
      } else if (event.key === "Tab") {
        if (event.shiftKey && document.activeElement === cancelRef.current) {
          event.preventDefault();
          confirmRef.current?.focus();
        } else if (!event.shiftKey && document.activeElement === confirmRef.current) {
          event.preventDefault();
          cancelRef.current?.focus();
        }
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [busy, onClose]);
  return <div className="workCreateBackdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget&&!busy)onClose()}}>
    <div className="workCreateDialog workTaskDeleteDialog" role="alertdialog" aria-modal="true" aria-labelledby="work-task-delete-title" aria-describedby="work-task-delete-description">
      <span className="workTaskDeleteIcon" aria-hidden="true"><img src={workSectionDeleteIcon} alt=""/></span>
      <div><span className="workTaskDeleteEyebrow">Удаление задачи</span><h2 id="work-task-delete-title">Удалить «{task.title}»?</h2></div>
      <p id="work-task-delete-description">Задача, её подзадачи, комментарии и вложения будут удалены без возможности восстановления.</p>
      <div className="workCreateActions"><button ref={cancelRef} type="button" disabled={busy} onClick={onClose}>Отмена</button><button ref={confirmRef} className="danger" type="button" disabled={busy} onClick={()=>void onConfirm()}>{busy?"Удаляем…":"Удалить задачу"}</button></div>
    </div>
  </div>;
}

function TaskDrawer({task,detail,resourcesReady,comments,commentsReady,completionPendingTasks,assigneePending,onClose,onUpdate,onToggleComplete,onRequestDelete,onAddSubtask,onAddComment,onError,onAddFile}:{task:WorkTask;detail:WorkProjectView;resourcesReady:boolean;comments:WorkComment[];commentsReady:boolean;completionPendingTasks:Set<number>;assigneePending:boolean;onClose:()=>void;onUpdate:(t:WorkTask,p:Partial<WorkTaskInput>)=>Promise<void>;onToggleComplete:(t:WorkTask)=>Promise<void>;onRequestDelete:(t:WorkTask,returnFocus?:HTMLElement|null)=>void;onAddSubtask:()=>void;onAddComment:(taskID:number,body:string)=>Promise<void>;onError:(v:unknown)=>void;onAddFile:(file:File)=>void}){
  const [comment,setComment]=useState("");
  const [description,setDescription]=useState(task.description);
  const fileRef=useRef<HTMLInputElement>(null);
  const titleRef=useRef<HTMLTextAreaElement>(null);
  const children=detail.tasks.filter((t)=>t.parentId===task.id);
  const files=detail.attachments.filter((a)=>a.taskId===task.id);
  const descriptionDirty=description!==task.description;
  useEffect(()=>setDescription(task.description),[task.id,task.version,task.description]);
  useEffect(()=>{
    const fit=()=>fitWorkTaskTitle(titleRef.current);
    fit();
    window.addEventListener("resize",fit);
    return ()=>window.removeEventListener("resize",fit);
  },[task.id,task.version,task.title]);
  return <aside className="workTaskDrawer" aria-label={`Задача: ${task.title}`}>
    <header className="workTaskDrawerHeader">
      <button type="button" className="workTaskDrawerClose" onClick={onClose} aria-label="Закрыть задачу">×</button>
    </header>

    <div className="workTaskHero">
      <textarea key={task.id+":"+task.version} ref={titleRef} className="workTaskName resize-none" data-caret-height="glyph" aria-label="Название задачи" rows={1} maxLength={300} defaultValue={task.title} onInput={(event)=>fitWorkTaskTitle(event.currentTarget)} onKeyDown={(event)=>{if(event.key==="Enter"&&!event.nativeEvent.isComposing){event.preventDefault();event.currentTarget.blur()}}} onBlur={(event)=>{const value=event.target.value.trim();if(value&&value!==task.title)void onUpdate(task,{title:value})}} />
    </div>

    <div className="workTaskFields">
      <label><span>Исполнитель</span><select aria-busy={assigneePending} disabled={assigneePending} value={task.assigneeId??""} onChange={(event)=>void onUpdate(task,{assigneeId:event.target.value?Number(event.target.value):null})}><option value="">Не назначен</option>{detail.project.members.map((member)=><option key={member.userId} value={member.userId}>{member.name}</option>)}</select></label>
      <label><span>Срок выполнения</span><input type="date" value={task.dueDate} onChange={(event)=>void onUpdate(task,{dueDate:event.target.value})}/></label>
    </div>

    <section className="workTaskDrawerSection workDescription">
      <div className="workTaskSectionHeading"><div><h3>Описание</h3><p>Детали и ссылки задачи</p></div></div>
      <textarea className="workTaskDescriptionInput resize-none" aria-label="Описание задачи" maxLength={2000} value={description} onChange={(event)=>setDescription(event.target.value)} placeholder="Необязательные подробности" />
      <div className="workDescriptionActions"><span aria-live="polite">{descriptionDirty?"Есть несохранённые изменения":"Описание сохранено"}</span><button type="button" disabled={!descriptionDirty} onClick={()=>void onUpdate(task,{description})}>{descriptionDirty?"Сохранить":"Сохранено"}</button></div>
    </section>

    <section className="workTaskDrawerSection">
      <div className="workTaskSectionHeading"><div><h3>Подзадачи</h3><p>{children.length ? `${children.length} ${pluralizeCount(children.length,"пункт","пункта","пунктов")}` : "Разбейте задачу на шаги"}</p></div><button type="button" onClick={onAddSubtask} aria-label="Добавить подзадачу">＋</button></div>
      <div className="workTaskSectionBody">{children.length===0?<p className="workTaskEmptyState">Подзадач пока нет</p>:children.map((child)=><div className={`workSubtask ${child.completed?"done":""}`} key={child.id}><button type="button" disabled={completionPendingTasks.has(child.id)} onClick={()=>void onToggleComplete(child)} aria-label={child.completed?`Вернуть подзадачу ${child.title}`:`Выполнить подзадачу ${child.title}`}><span aria-hidden="true">{child.completed?"✓":""}</span></button><span>{child.title}</span><small>{child.assignee||"Без исполнителя"}</small></div>)}</div>
    </section>

    <section className="workTaskDrawerSection">
      <div className="workTaskSectionHeading"><div><h3>Вложения</h3><p>{files.length ? `${files.length} ${pluralizeCount(files.length,"файл","файла","файлов")}` : "Файлы и материалы задачи"}</p></div><button type="button" onClick={()=>fileRef.current?.click()} aria-label="Добавить вложение">＋</button></div>
      <div className="workTaskSectionBody workTaskFiles">{!resourcesReady?<p className="workTaskEmptyState">Загружаем вложения…</p>:files.length===0?<p className="workTaskEmptyState">Вложений пока нет</p>:files.map((attachment)=><a key={attachment.id} href={`/api/work/attachments/${attachment.id}`}><span aria-hidden="true">↗</span><strong>{attachment.name}</strong><small>{formatFileSize(attachment.size)}</small></a>)}</div>
      <input ref={fileRef} hidden type="file" onChange={(event)=>{const file=event.currentTarget.files?.[0];if(file)onAddFile(file);event.currentTarget.value=""}}/>
    </section>

    <section className="workTaskDrawerSection workComments">
      <div className="workTaskSectionHeading"><div><h3>Комментарии</h3><p>{comments.length?`${comments.length} ${pluralizeCount(comments.length,"сообщение","сообщения","сообщений")}`:"Обсуждение задачи"}</p></div></div>
      <div className="workTaskSectionBody workCommentList">{!commentsReady&&<p className="workTaskEmptyState">Загружаем комментарии…</p>}{commentsReady&&comments.length===0&&<p className="workTaskEmptyState">Комментариев пока нет</p>}{comments.map((item)=><div key={item.id}><WorkAvatar name={item.author} avatar={item.avatar} className="workCommentAvatar"/><p><strong>{item.author}</strong><span>{item.body}</span>{formatActivityDate(item.createdAt)&&<time>{formatActivityDate(item.createdAt)}</time>}</p></div>)}</div>
      <form noValidate onSubmit={(event)=>{event.preventDefault();if(!comment.trim())return;void onAddComment(task.id,comment).then(()=>setComment("")).catch(onError)}}><textarea className="resize-none" value={comment} onChange={(event)=>setComment(event.target.value)} placeholder="Написать комментарий…" aria-label="Комментарий"/><button type="submit" disabled={!comment.trim()}>Отправить</button></form>
    </section>

    <div className="workTaskDangerZone"><div><strong>Удаление задачи</strong><span>Это действие нельзя отменить</span></div><button type="button" onClick={(event)=>onRequestDelete(task,event.currentTarget)}><img src={workSectionDeleteIcon} alt=""/>Удалить задачу</button></div>
  </aside>
}
