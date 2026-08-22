// @vitest-environment jsdom

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Issue } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { IssueEngineRows } from "./engine-picker";

const RUNTIME_CLAUDE = "11111111-1111-1111-1111-111111111111";
const RUNTIME_OPENCLAW = "22222222-2222-2222-2222-222222222222";

const mockState = vi.hoisted(() => ({
  runtimes: [] as unknown[],
  writes: [] as Array<{ issueId: string; key: string; value: string | null }>,
}));

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/core/runtimes", () => ({
  runtimeListOptions: () => ({
    queryKey: ["runtimes", "ws-1", "list"],
    queryFn: () => Promise.resolve(mockState.runtimes),
  }),
  runtimeDisplayLabel: (r: { name: string }) => r.name,
}));

// The model half is the agent inspector's picker, reused as-is. Stubbed here so
// this file tests the engine rows — the wiring, the labels, the writes — and not
// the model catalog discovery that picker already owns its own tests for.
vi.mock("../../agents/components/inspector/model-picker", () => ({
  ModelPicker: ({ runtimeId, value }: { runtimeId: string | null; value: string }) => (
    <span data-testid="model-picker" data-runtime={runtimeId}>
      {value || "model-empty"}
    </span>
  ),
}));

vi.mock("@multica/core/issues", async () => {
  const actual =
    await vi.importActual<typeof import("@multica/core/issues/engine")>(
      "@multica/core/issues/engine",
    );
  return {
    ISSUE_ENGINE_RUNTIME_KEY: actual.ISSUE_ENGINE_RUNTIME_KEY,
    ISSUE_ENGINE_MODEL_KEY: actual.ISSUE_ENGINE_MODEL_KEY,
    isOpenclawProvider: actual.isOpenclawProvider,
    readIssueEnginePin: actual.readIssueEnginePin,
    useSetIssueEngineKey: () => ({
      mutateAsync: (vars: { issueId: string; key: string; value: string | null }) => {
        mockState.writes.push(vars);
        return Promise.resolve({ metadata: {} });
      },
    }),
  };
});

function issueWith(metadata: Record<string, string>): Issue {
  return { id: "issue-1", metadata } as unknown as Issue;
}

function renderRows(issue: Issue) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <div className="grid grid-cols-[auto_1fr]">
        <IssueEngineRows issue={issue} />
      </div>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mockState.runtimes = [
    { id: RUNTIME_CLAUDE, name: "Claude (host)", provider: "claude", status: "online" },
    { id: RUNTIME_OPENCLAW, name: "OpenClaw (host)", provider: "openclaw", status: "offline" },
  ];
  mockState.writes = [];
});

afterEach(cleanup);

describe("IssueEngineRows", () => {
  it("reads as 'no choice' — and offers no model row — when nothing is pinned", async () => {
    renderRows(issueWith({}));
    expect(await screen.findByText("Agent's engine")).toBeTruthy();
    expect(screen.queryByTestId("model-picker")).toBeNull();
  });

  it("shows the pinned engine and its model", async () => {
    renderRows(
      issueWith({ engine_runtime_id: RUNTIME_CLAUDE, engine_model: "claude-opus-5" }),
    );
    expect(await screen.findByText("Claude (host)")).toBeTruthy();
    expect(screen.getByTestId("model-picker").textContent).toBe("claude-opus-5");
    expect(screen.getByText("Model")).toBeTruthy();
  });

  // The openclaw trap: that field is an agent id, and calling it "Model" would
  // be a lie the user has no way to catch.
  it("labels the second row 'OpenClaw agent' when the engine is openclaw", async () => {
    renderRows(issueWith({ engine_runtime_id: RUNTIME_OPENCLAW, engine_model: "deepseek" }));
    expect(await screen.findByText("OpenClaw agent")).toBeTruthy();
    expect(screen.queryByText("Model")).toBeNull();
  });

  // A pinned engine with no model is a complete setting ("use that engine's
  // default"), not a half-filled form.
  it("keeps the model row present and empty when only the engine is pinned", async () => {
    renderRows(issueWith({ engine_runtime_id: RUNTIME_CLAUDE }));
    expect(await screen.findByTestId("model-picker")).toBeTruthy();
    expect(screen.getByTestId("model-picker").textContent).toBe("model-empty");
  });

  it("writes the runtime key when an engine is picked", async () => {
    renderRows(issueWith({}));
    fireEvent.click(await screen.findByLabelText("Engine"));
    fireEvent.click(await screen.findByText("Claude (host)"));
    await waitFor(() => expect(mockState.writes.length).toBe(1));
    expect(mockState.writes[0]).toEqual({
      issueId: "issue-1",
      key: "engine_runtime_id",
      value: RUNTIME_CLAUDE,
    });
  });

  // Switching engines must not carry the old engine's model across: on openclaw
  // that value is an agent id that does not exist on the other runtime.
  it("clears the model when the engine changes", async () => {
    renderRows(
      issueWith({ engine_runtime_id: RUNTIME_CLAUDE, engine_model: "claude-opus-5" }),
    );
    fireEvent.click(await screen.findByLabelText("Engine"));
    fireEvent.click(await screen.findByText("OpenClaw (host)"));
    await waitFor(() => expect(mockState.writes.length).toBe(2));
    expect(mockState.writes[1]).toEqual({
      issueId: "issue-1",
      key: "engine_model",
      value: null,
    });
  });

  it("clears both keys when the issue goes back to the agent's engine", async () => {
    renderRows(
      issueWith({ engine_runtime_id: RUNTIME_CLAUDE, engine_model: "claude-opus-5" }),
    );
    fireEvent.click(await screen.findByLabelText("Engine"));
    // The label appears twice once the popover is open (trigger + empty row);
    // the row is the last one in document order.
    const rows = await screen.findAllByText("Agent's engine");
    fireEvent.click(rows[rows.length - 1]!);
    await waitFor(() => expect(mockState.writes.length).toBe(2));
    expect(mockState.writes.map((w) => w.value)).toEqual([null, null]);
  });

  // A runtime that no longer exists in the list must not read as "not pinned":
  // the server still routes this issue's tasks at that id.
  it("does not disguise an unavailable runtime as no choice", async () => {
    renderRows(issueWith({ engine_runtime_id: "99999999-9999-9999-9999-999999999999" }));
    expect(await screen.findByText("Unavailable runtime")).toBeTruthy();
  });
});
