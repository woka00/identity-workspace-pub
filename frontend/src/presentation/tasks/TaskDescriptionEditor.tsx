import { useCallback, useEffect, useRef, useState } from "react";
import { useAnimatedCaret } from "../hooks/useAnimatedCaret";

const MAX_DESCRIPTION_LENGTH = 2000;

function escapeHTML(value: string) {
  return value.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function removeLegacyInlineFormatting(value: string) {
  return value
    .replace(/<\/?u>/gi, "")
    .replace(/\*\*([^*]+?)\*\*/g, "$1")
    .replace(/(^|[^*])\*([^*]+?)\*/g, "$1$2");
}

function checklistHTML(checked: boolean, content: string) {
  return `<div class="taskRichChecklist${checked ? " isChecked" : ""}" data-checklist="true" data-checked="${checked ? "true" : "false"}"><span class="taskRichChecklistToggle" data-check-toggle="true" contenteditable="false" role="checkbox" aria-checked="${checked ? "true" : "false"}" aria-label="${checked ? "Вернуть подзадачу" : "Выполнить подзадачу"}"></span><span data-check-content="true">${content || "<br>"}</span></div>`;
}

function descriptionToHTML(value: string) {
  if (!value) return '<div data-text-block="true"><br></div>';
  return value.split("\n").map((line) => {
    const checklist = line.match(/^\s*(?:-\s*)?\[([ xX])\]\s?(.*)$/) ?? line.match(/^\s*([☐☑])\s?(.*)$/);
    if (checklist) {
      const checked = checklist[1].toLocaleLowerCase() === "x" || checklist[1] === "☑";
      return checklistHTML(checked, escapeHTML(removeLegacyInlineFormatting(checklist[2])));
    }
    return `<div data-text-block="true">${escapeHTML(removeLegacyInlineFormatting(line)) || "<br>"}</div>`;
  }).join("");
}

function serializeInline(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? "";
  if (!(node instanceof HTMLElement)) return "";
  if (node.matches("[data-check-toggle]")) return "";
  if (node.tagName === "BR") return "\n";
  const content = [...node.childNodes].map(serializeInline).join("");
  return content;
}

function editorToDescription(editor: HTMLElement) {
  const lines = [...editor.childNodes].flatMap((node) => {
    if (node.nodeType === Node.TEXT_NODE) return (node.textContent ?? "").split("\n");
    if (!(node instanceof HTMLElement)) return [""];
    if (node.matches("[data-checklist]")) {
      const checked = node.dataset.checked === "true";
      const content = node.querySelector<HTMLElement>("[data-check-content]");
      return [`- [${checked ? "x" : " "}] ${content ? serializeInline(content) : ""}`.trimEnd()];
    }
    // A lone <br> keeps an empty block editable; the block already represents one line.
    if (node.childNodes.length === 1 && node.firstChild instanceof HTMLElement && node.firstChild.tagName === "BR") return [""];
    return serializeInline(node).split("\n");
  });
  return lines.join("\n").replace(/\n{3,}$/g, "\n\n");
}

function selectionInside(editor: HTMLElement, selection: Selection | null) {
  return Boolean(selection?.anchorNode && editor.contains(selection.anchorNode));
}

function setCaretAtStart(element: HTMLElement) {
  const selection = window.getSelection();
  if (!selection) return;
  const range = document.createRange();
  range.selectNodeContents(element);
  range.collapse(true);
  selection.removeAllRanges();
  selection.addRange(range);
}

function createChecklistElement(checked = false) {
  const wrapper = document.createElement("div");
  wrapper.innerHTML = checklistHTML(checked, "");
  return wrapper.firstElementChild as HTMLElement;
}

export default function TaskDescriptionEditor({ value, onChange, taskID }: { value: string; onChange: (value: string) => void; taskID: number }) {
  const editorRef = useRef<HTMLDivElement>(null);
  const { overlayRef, caretRef } = useAnimatedCaret(editorRef);
  const savedRangeRef = useRef<Range | null>(null);
  const lastValidHTMLRef = useRef("");
  const [empty, setEmpty] = useState(!value.trim());

  const rememberSelection = useCallback(() => {
    const editor = editorRef.current;
    const selection = window.getSelection();
    if (!editor || !selectionInside(editor, selection) || !selection?.rangeCount) return;
    savedRangeRef.current = selection.getRangeAt(0).cloneRange();
  }, []);

  const syncValue = useCallback(() => {
    const editor = editorRef.current;
    if (!editor) return;
    const next = editorToDescription(editor);
    if (next.length > MAX_DESCRIPTION_LENGTH) {
      editor.innerHTML = lastValidHTMLRef.current;
      return;
    }
    lastValidHTMLRef.current = editor.innerHTML;
    setEmpty(next.trim().length === 0);
    onChange(next);
    rememberSelection();
  }, [onChange, rememberSelection]);

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor) return;
    const html = descriptionToHTML(value);
    editor.innerHTML = html;
    lastValidHTMLRef.current = html;
    savedRangeRef.current = null;
    const normalizedValue = editorToDescription(editor);
    setEmpty(!normalizedValue.trim());
    if (normalizedValue !== value) onChange(normalizedValue);
  // The editor is intentionally uncontrolled while typing to preserve the native selection.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskID]);

  function restoreSelection() {
    const editor = editorRef.current;
    const selection = window.getSelection();
    if (!editor || !selection) return false;
    editor.focus({ preventScroll: true });
    const range = savedRangeRef.current;
    if (range) {
      selection.removeAllRanges();
      selection.addRange(range);
      return true;
    }
    if (!selectionInside(editor, selection)) {
      const end = document.createRange();
      end.selectNodeContents(editor);
      end.collapse(false);
      selection.removeAllRanges();
      selection.addRange(end);
    }
    return true;
  }

  function insertChecklist() {
    const editor = editorRef.current;
    if (!editor) return;
    restoreSelection();
    const selection = window.getSelection();
    const anchor = selection?.anchorNode instanceof HTMLElement ? selection.anchorNode : selection?.anchorNode?.parentElement;
    const currentChecklist = anchor?.closest<HTMLElement>("[data-checklist]");
    const row = createChecklistElement();
    if (currentChecklist && editor.contains(currentChecklist)) {
      currentChecklist.after(row);
    } else if (!editor.textContent?.trim()) {
      editor.replaceChildren(row);
    } else {
      document.execCommand("insertParagraph", false);
      const nextAnchor = selection?.anchorNode instanceof HTMLElement ? selection.anchorNode : selection?.anchorNode?.parentElement;
      const block = nextAnchor?.closest<HTMLElement>("div, p");
      if (block && block !== editor && editor.contains(block)) {
        const content = row.querySelector<HTMLElement>("[data-check-content]");
        if (content) {
          content.replaceChildren(...block.childNodes);
          if (!content.textContent && !content.querySelector("br")) content.append(document.createElement("br"));
        }
        block.replaceWith(row);
      } else {
        editor.append(row);
      }
    }
    const content = row.querySelector<HTMLElement>("[data-check-content]");
    if (content) setCaretAtStart(content);
    rememberSelection();
    syncValue();
  }

  function handleEditorKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    const editor = editorRef.current;
    const selection = window.getSelection();
    if (!editor || !selectionInside(editor, selection)) return;
    if ((event.ctrlKey || event.metaKey) && ["b", "i", "u"].includes(event.key.toLocaleLowerCase())) {
      event.preventDefault();
      return;
    }
    const anchor = selection?.anchorNode instanceof HTMLElement ? selection.anchorNode : selection?.anchorNode?.parentElement;
    const current = anchor?.closest<HTMLElement>("[data-checklist]");
    if (!current || !editor.contains(current)) return;
    const content = current.querySelector<HTMLElement>("[data-check-content]");
    if (!content) return;

    if (event.key === "Backspace" && !content.textContent?.trim()) {
      event.preventDefault();
      const textBlock = document.createElement("div");
      textBlock.dataset.textBlock = "true";
      textBlock.append(document.createElement("br"));
      current.replaceWith(textBlock);
      setCaretAtStart(textBlock);
      syncValue();
      return;
    }

    if (event.key !== "Enter" || event.shiftKey) return;
    event.preventDefault();
    if (!content.textContent?.trim()) {
      const textBlock = document.createElement("div");
      textBlock.dataset.textBlock = "true";
      textBlock.append(document.createElement("br"));
      current.replaceWith(textBlock);
      setCaretAtStart(textBlock);
      syncValue();
      return;
    }
    const next = createChecklistElement();
    const nextContent = next.querySelector<HTMLElement>("[data-check-content]");
    const range = selection?.rangeCount ? selection.getRangeAt(0) : null;
    if (range && nextContent && content.contains(range.startContainer)) {
      if (!range.collapsed) range.deleteContents();
      const tail = document.createRange();
      tail.selectNodeContents(content);
      tail.setStart(range.startContainer, range.startOffset);
      const fragment = tail.extractContents();
      nextContent.replaceChildren(fragment);
      if (!nextContent.textContent && !nextContent.querySelector("br")) nextContent.append(document.createElement("br"));
    }
    current.after(next);
    if (nextContent) setCaretAtStart(nextContent);
    syncValue();
  }

  function handleEditorClick(event: React.MouseEvent<HTMLDivElement>) {
    const target = event.target instanceof HTMLElement ? event.target.closest<HTMLElement>("[data-check-toggle]") : null;
    if (!target) {
      rememberSelection();
      return;
    }
    event.preventDefault();
    const row = target.closest<HTMLElement>("[data-checklist]");
    if (!row) return;
    const checked = row.dataset.checked !== "true";
    row.dataset.checked = String(checked);
    row.classList.toggle("isChecked", checked);
    target.setAttribute("aria-checked", String(checked));
    target.setAttribute("aria-label", checked ? "Вернуть подзадачу" : "Выполнить подзадачу");
    syncValue();
  }

  return (
    <div className={`taskRichDescription${empty ? " isEmpty" : ""}`}>
      <div className="taskRichDescriptionViewport">
        <div
          ref={editorRef}
          className="taskRichDescriptionEditor"
          contentEditable
          role="textbox"
          aria-multiline="true"
          aria-label="Описание"
          data-placeholder="Необязательные подробности"
          suppressContentEditableWarning
          onBeforeInput={(event) => {
            if (value.length < MAX_DESCRIPTION_LENGTH || !event.nativeEvent.inputType.startsWith("insert")) return;
            const selection = window.getSelection();
            if (selection?.isCollapsed) event.preventDefault();
          }}
          onPaste={(event) => {
            event.preventDefault();
            document.execCommand("insertText", false, event.clipboardData.getData("text/plain"));
          }}
          onInput={syncValue}
          onKeyDown={handleEditorKeyDown}
          onKeyUp={rememberSelection}
          onMouseUp={rememberSelection}
          onClick={handleEditorClick}
          onFocus={rememberSelection}
        />
        <div ref={overlayRef} className="taskRichCaretOverlay" aria-hidden="true">
          <span ref={caretRef} className="taskRichCaret"><span className="taskRichCaretLine" /></span>
        </div>
      </div>
      <div className="taskRichDescriptionToolbar" role="toolbar" aria-label="Инструменты описания">
        <button type="button" className="taskRichChecklistTool" onPointerDown={(event) => event.preventDefault()} onClick={insertChecklist}>
          <svg className="taskRichChecklistToolIcon" viewBox="0 0 20 20" aria-hidden="true">
            <rect x="2.5" y="3" width="5" height="5" rx="1.4" />
            <path d="m3.8 5.5 1.1 1.1 2-2.2M10 5.5h7M10 10h7M10 14.5h7" />
            <rect x="2.5" y="12" width="5" height="5" rx="1.4" />
          </svg>
          <span>Подзадача</span>
        </button>
      </div>
    </div>
  );
}
