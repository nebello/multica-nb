import { describe, expect, it } from "vitest";
import {
  EMPTY_ISSUE_METADATA_RESPONSE,
  IssueMetadataResponseSchema,
} from "../api/schemas";
import { parseWithFallback } from "../api/schema";
import type { Issue } from "../types";
import {
  ISSUE_ENGINE_MODEL_KEY,
  ISSUE_ENGINE_RUNTIME_KEY,
  isOpenclawProvider,
  readIssueEnginePin,
} from "./engine";

const RUNTIME = "11111111-1111-1111-1111-111111111111";

const issueWith = (metadata: Record<string, unknown>) =>
  ({ metadata }) as unknown as Issue;

describe("readIssueEnginePin", () => {
  it("reads an unpinned issue as following the agent's engine", () => {
    expect(readIssueEnginePin(issueWith({}))).toEqual({ runtimeId: null, model: "" });
    expect(readIssueEnginePin(undefined)).toEqual({ runtimeId: null, model: "" });
  });

  it("reads both keys when the issue pins an engine", () => {
    expect(
      readIssueEnginePin(
        issueWith({ [ISSUE_ENGINE_RUNTIME_KEY]: RUNTIME, [ISSUE_ENGINE_MODEL_KEY]: "deepseek" }),
      ),
    ).toEqual({ runtimeId: RUNTIME, model: "deepseek" });
  });

  it("treats a pinned engine with no model as pinned, not as unset", () => {
    expect(readIssueEnginePin(issueWith({ [ISSUE_ENGINE_RUNTIME_KEY]: RUNTIME }))).toEqual({
      runtimeId: RUNTIME,
      model: "",
    });
  });

  // Metadata is a free-form bag any agent can write. A non-string (or empty)
  // value must read as "not pinned" rather than reach the UI as a value.
  it("ignores values that are not usable strings", () => {
    expect(
      readIssueEnginePin(
        issueWith({ [ISSUE_ENGINE_RUNTIME_KEY]: 42, [ISSUE_ENGINE_MODEL_KEY]: true }),
      ),
    ).toEqual({ runtimeId: null, model: "" });
    expect(readIssueEnginePin(issueWith({ [ISSUE_ENGINE_RUNTIME_KEY]: "" }))).toEqual({
      runtimeId: null,
      model: "",
    });
  });
});

describe("isOpenclawProvider", () => {
  it("is true only for openclaw", () => {
    expect(isOpenclawProvider("openclaw")).toBe(true);
    expect(isOpenclawProvider("claude")).toBe(false);
    expect(isOpenclawProvider(undefined)).toBe(false);
  });
});

describe("IssueMetadataResponseSchema", () => {
  it("parses the full bag both metadata writes answer with", () => {
    const parsed = parseWithFallback(
      { metadata: { [ISSUE_ENGINE_RUNTIME_KEY]: RUNTIME, pipeline_status: "green", attempts: 2 } },
      IssueMetadataResponseSchema,
      EMPTY_ISSUE_METADATA_RESPONSE,
      { endpoint: "test" },
    );
    expect(parsed.metadata[ISSUE_ENGINE_RUNTIME_KEY]).toBe(RUNTIME);
    expect(parsed.metadata.attempts).toBe(2);
  });

  // A malformed body must degrade to an empty bag, never throw into the click
  // handler: the write already happened server-side, and the WS event will
  // deliver the real state.
  it("falls back to an empty bag on a malformed body", () => {
    for (const bad of [null, "nope", { metadata: [] }, {}]) {
      const parsed = parseWithFallback(
        bad,
        IssueMetadataResponseSchema,
        EMPTY_ISSUE_METADATA_RESPONSE,
        { endpoint: "test" },
      );
      expect(readIssueEnginePin({ metadata: parsed.metadata } as Issue).runtimeId).toBeNull();
    }
  });
});
