import { act, renderHook } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { useComposerDraft } from "../src/useComposerDraft.js";

function editor(key) {
  return renderHook((slot) => useComposerDraft(slot, vi.fn()), { initialProps: key });
}

test("an accepted request clears only the submitted draft, even after switching conversations", () => {
  const { result, rerender } = editor("conversation-a");
  const file = { id: "receipt" }, nextFile = { id: "agenda" };
  const issue = { type: "issue", id: "plan" };
  act(() => {
    result.current.setText("第一个问题");
    result.current.setAttachments([file]);
  });
  let request;
  act(() => { request = result.current.begin(); });
  expect(request.sent.attachments).toEqual([file]);
  expect(result.current.text).toBe("");
  act(() => {
    result.current.setText("第二个问题");
    result.current.setAttachments([nextFile]);
    result.current.setReferences([issue]);
  });
  rerender("conversation-b");
  act(() => {
    result.current.setText("另一段对话的草稿");
    request.finish(true);
  });
  expect(result.current.text).toBe("另一段对话的草稿");
  expect(JSON.parse(localStorage.getItem("conversation-a"))).toEqual({
    text: "第二个问题", attachments: [nextFile], references: [issue],
  });
  rerender("conversation-a");
  expect(result.current.text).toBe("第二个问题");
});

test("a rejected first message restores text, files and references with the follow-up in the created session", () => {
  const { result, rerender } = editor("new:project");
  const file = { id: "itinerary" }, issue = { type: "issue", id: "trip" };
  act(() => {
    result.current.setText("第一问");
    result.current.setAttachments([file]);
    result.current.setReferences([issue]);
  });
  let request;
  act(() => { request = result.current.begin(); });
  act(() => {
    result.current.setText("第二问");
    result.current.setReferences([issue]);
    expect(result.current.begin()).toBeNull();
    request.bind("created-session", true);
  });
  rerender("created-session");
  expect(result.current.text).toBe("第二问");
  expect(JSON.parse(localStorage.getItem("created-session")).text).toBe("第一问\n第二问");
  act(() => request.finish(false));
  expect(result.current.text).toBe("第一问\n第二问");
  expect(result.current.attachments).toEqual([file]);
  expect(result.current.references).toEqual([issue]);
  expect(localStorage.getItem("new:project")).toBeNull();
});

test("leaving during creation keeps edits in the chosen draft when the old request finishes", () => {
  const { result } = editor("new:personal");
  act(() => result.current.setText("待确认的问题"));
  let request;
  act(() => { request = result.current.begin(); });
  act(() => {
    result.current.setText("我决定另开一段对话");
    request.bind("previously-requested", false);
    request.finish(false);
  });
  expect(result.current.text).toBe("我决定另开一段对话");
  expect(JSON.parse(localStorage.getItem("previously-requested")).text).toBe("待确认的问题");
});
