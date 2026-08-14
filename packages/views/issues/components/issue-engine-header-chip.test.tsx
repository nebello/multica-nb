// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "@multica/core/i18n/react";
import type { AgentRuntime, Issue, IssueMetadata } from "@multica/core/types";
import enCommon from "../../locales/en/common.json";
import enIssues from "../../locales/en/issues.json";
import enRuntimes from "../../locales/en/runtimes.json";

const mockListRuntimes = vi.hoisted(() => vi.fn());
const mockInitiateListModels = vi.hoisted(() => vi.fn());
const mockGetListModelsResult = vi.hoisted(() => vi.fn());
const mockSetIssueMetadataKey = vi.hoisted(() => vi.fn());
const mockDeleteIssueMetadataKey = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/api", () => ({
  api: {
    listRuntimes: (...args: unknown[]) => mockListRuntimes(...args),
    initiateListModels: (...args: unknown[]) => mockInitiateListModels(...args),
    getListModelsResult: (...args: unknown[]) => mockGetListModelsResult(...args),
    setIssueMetadataKey: (...args: unknown[]) => mockSetIssueMetadataKey(...args),
    deleteIssueMetadataKey: (...args: unknown[]) =>
      mockDeleteIssueMetadataKey(...args),
  },
}));

vi.mock("@multica/core/auth", () => ({
  useAuthStore: Object.assign(
    (selector: (state: { user: { id: string } }) => unknown) =>
      selector({ user: { id: "user-1" } }),
    { getState: () => ({ user: { id: "user-1" } }) },
  ),
}));

import { IssueEngineHeaderChip } from "./issue-engine-header-chip";

const WS_ID = "ws-1";

const OPENCLAW_RUNTIME = {
  id: "runtime-openclaw",
  workspace_id: WS_ID,
  name: "Openclaw (mac)",
  custom_name: null,
  provider: "openclaw",
  status: "online",
  owner_id: "user-1",
  visibility: "private",
} as unknown as AgentRuntime;

function issueWith(metadata: IssueMetadata): Issue {
  return {
    id: "issue-1",
    identifier: "MUL-1",
    title: "An issue",
    metadata,
  } as unknown as Issue;
}

function renderChip(metadata: IssueMetadata) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <I18nProvider
      locale="en"
      resources={{ en: { common: enCommon, issues: enIssues, runtimes: enRuntimes } }}
    >
      <QueryClientProvider client={queryClient}>
        <IssueEngineHeaderChip wsId={WS_ID} issue={issueWith(metadata)} />
      </QueryClientProvider>
    </I18nProvider>,
  );
}

describe("IssueEngineHeaderChip", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListRuntimes.mockResolvedValue([OPENCLAW_RUNTIME]);
    const catalog = {
      id: "req-1",
      runtime_id: OPENCLAW_RUNTIME.id,
      status: "completed",
      models: [{ id: "deepseek", label: "deepseek" }],
      supported: true,
      created_at: "2026-08-14T00:00:00Z",
      updated_at: "2026-08-14T00:00:00Z",
    };
    mockInitiateListModels.mockResolvedValue(catalog);
    mockGetListModelsResult.mockResolvedValue(catalog);
    mockSetIssueMetadataKey.mockResolvedValue({});
    mockDeleteIssueMetadataKey.mockResolvedValue({});
  });

  afterEach(cleanup);

  it("renders nothing for an issue with no pinned engine", () => {
    const { container } = renderChip({ pipeline_status: "running" });
    expect(container).toBeEmptyDOMElement();
    expect(mockListRuntimes).not.toHaveBeenCalled();
  });

  it("shows the set engine and model, labelled as a setting", async () => {
    renderChip({
      engine_runtime_id: OPENCLAW_RUNTIME.id,
      engine_model: "deepseek",
    });
    // "Set" is the whole point of the chip: it must not read as the model a
    // past run consumed, which is the execution log's story.
    expect(await screen.findByText("Set")).toBeTruthy();
    // Awaited: until the runtime list lands the chip cannot know the engine is
    // openclaw, and it must not guess the "agent" wording before it does.
    expect(await screen.findByText(/agent deepseek/)).toBeTruthy();
    // The engine name does not fit the collapsed chip, so it has to be in the
    // accessible name — otherwise a screen reader hears the model alone.
    expect(
      screen.getByRole("button", { name: /Openclaw \(mac\)/ }),
    ).toBeTruthy();
  });

  it("calls the second field an openclaw agent, never a model", async () => {
    renderChip({ engine_runtime_id: OPENCLAW_RUNTIME.id });
    fireEvent.click(await screen.findByRole("button"));
    expect(await screen.findByText("openclaw agent")).toBeTruthy();
    expect(screen.queryByText("Model")).toBeNull();
    // The catalog entries for openclaw are agents, and they are what the
    // picker offers.
    expect(await screen.findByText("deepseek")).toBeTruthy();
  });

  it("writes the picked openclaw agent onto the issue", async () => {
    renderChip({ engine_runtime_id: OPENCLAW_RUNTIME.id });
    fireEvent.click(await screen.findByRole("button"));
    fireEvent.click(await screen.findByText("deepseek"));
    await waitFor(() =>
      expect(mockSetIssueMetadataKey).toHaveBeenCalledWith(
        "issue-1",
        "engine_model",
        "deepseek",
      ),
    );
    // The runtime is re-asserted rather than left alone: one mutation writes
    // the whole pin, so a half-applied pair can never survive a failure.
    expect(mockSetIssueMetadataKey).toHaveBeenCalledWith(
      "issue-1",
      "engine_runtime_id",
      OPENCLAW_RUNTIME.id,
    );
  });

  it("clears both keys when the pin is removed", async () => {
    renderChip({
      engine_runtime_id: OPENCLAW_RUNTIME.id,
      engine_model: "deepseek",
    });
    fireEvent.click(await screen.findByRole("button"));
    fireEvent.click(await screen.findByText("Clear the set engine"));
    await waitFor(() =>
      expect(mockDeleteIssueMetadataKey).toHaveBeenCalledWith(
        "issue-1",
        "engine_runtime_id",
      ),
    );
    expect(mockDeleteIssueMetadataKey).toHaveBeenCalledWith(
      "issue-1",
      "engine_model",
    );
  });
});
