import { describe, expect, it } from "vitest";
import { soleMentionedIssueId } from "./issue-mentions";

describe("soleMentionedIssueId", () => {
  it("returns the id of the only mentioned issue", () => {
    expect(
      soleMentionedIssueId(
        "Take [MUL-12](mention://issue/11111111-2222-3333-4444-555555555555) please",
      ),
    ).toBe("11111111-2222-3333-4444-555555555555");
  });

  it("returns null with no issue mention", () => {
    expect(soleMentionedIssueId("just a message")).toBeNull();
    expect(
      soleMentionedIssueId("[@Bot](mention://agent/abc) hello"),
    ).toBeNull();
  });

  it("returns null when two different issues are mentioned", () => {
    expect(
      soleMentionedIssueId(
        "[MUL-1](mention://issue/aaa) and [MUL-2](mention://issue/bbb)",
      ),
    ).toBeNull();
  });

  it("treats repeated mentions of one issue as a single target", () => {
    expect(
      soleMentionedIssueId(
        "[MUL-1](mention://issue/aaa) again [MUL-1](mention://issue/aaa)",
      ),
    ).toBe("aaa");
  });

  it("ignores plain links that are not mentions", () => {
    expect(
      soleMentionedIssueId("[docs](https://example.com/issue/abc)"),
    ).toBeNull();
  });
});
