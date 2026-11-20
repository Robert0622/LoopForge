import { useEffect, useRef, useState, useSyncExternalStore } from "react";

const empty = { text: "", attachments: [], references: [] };
const hasContent = (draft) => draft.text || draft.attachments.length || draft.references.length;

function restore(sent, next) {
  if (!hasContent(next)) return sent;
  return {
    text: [sent.text, next.text].filter(Boolean).join("\n"),
    attachments: [
      ...sent.attachments.filter((a) => !next.attachments.some((b) => b.id === a.id)),
      ...next.attachments,
    ],
    references: [
      ...sent.references.filter((a) => !next.references.some((b) => b.type === a.type && b.id === a.id)),
      ...next.references,
    ],
  };
}

// Requests may finish after the user has opened a different conversation.
// Keep the submitted part and the editable part in their originating slot.
function createDraftStore(reportError) {
  const drafts = new Map();
  const pending = new Map();
  const unsaved = new Set();
  const listeners = new Set();
  function read(key) {
    if (!drafts.has(key)) {
      let draft = empty;
      try {
        const saved = key && JSON.parse(localStorage.getItem(key));
        if (saved) draft = {
          text: typeof saved.text === "string" ? saved.text : "",
          attachments: Array.isArray(saved.attachments) ? saved.attachments : [],
          references: Array.isArray(saved.references) ? saved.references : [],
        };
      } catch {}
      drafts.set(key, draft);
    }
    return drafts.get(key);
  }
  function write(key, draft) {
    drafts.set(key, draft);
    if (key) {
      // A reload before acknowledgement can recover both the original message
      // and anything typed since. Restoring a draft never sends a message.
      const saved = pending.has(key) ? restore(pending.get(key), draft) : draft;
      try {
        if (hasContent(saved)) localStorage.setItem(key, JSON.stringify(saved));
        else localStorage.removeItem(key);
        unsaved.delete(key);
      } catch {
        unsaved.add(key);
        reportError();
      }
    }
    listeners.forEach((listener) => listener());
  }
  return {
    read,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    forget(key) {
      if (!pending.has(key) && !unsaved.has(key)) drafts.delete(key);
    },
    edit(key, field, value) {
      const draft = read(key);
      const next = typeof value === "function" ? value(draft[field]) : value;
      if (next !== draft[field]) write(key, { ...draft, [field]: next });
    },
    begin(key) {
      const sent = read(key);
      if (pending.has(key) || (!sent.text.trim() && !sent.attachments.length)) return null;
      pending.set(key, sent);
      write(key, empty);
      return {
        sent,
        bind(sessionKey, carryEdits) {
          const edits = read(key);
          pending.delete(key);
          write(key, carryEdits ? empty : edits);
          key = sessionKey;
          pending.set(key, sent);
          write(key, carryEdits ? edits : read(key));
        },
        finish(accepted) {
          const edits = read(key);
          pending.delete(key);
          write(key, accepted ? edits : restore(sent, edits));
        },
      };
    },
  };
}

export function useComposerDraft(key, setError) {
  const error = useRef(setError);
  error.current = setError;
  const [store] = useState(() => createDraftStore(() =>
    error.current("浏览器存储空间不足，草稿仍保留在当前页面。请移除较大的附件后重试。"),
  ));
  const draft = useSyncExternalStore(store.subscribe, () => store.read(key));
  useEffect(() => () => store.forget(key), [store, key]);
  return {
    ...draft,
    setText: (value) => store.edit(key, "text", value),
    setAttachments: (value) => store.edit(key, "attachments", value),
    setReferences: (value) => store.edit(key, "references", value),
    begin: () => store.begin(key),
  };
}
