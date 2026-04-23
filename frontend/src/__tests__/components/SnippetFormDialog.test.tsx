/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { useSnippetStore } from "../../stores/snippetStore";
import { useAssetStore } from "../../stores/assetStore";
import { SnippetFormDialog } from "../../components/snippet/SnippetFormDialog";
import { CreateSnippet, UpdateSnippet, ListSnippets } from "../../../wailsjs/go/app/App";

describe("SnippetFormDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useSnippetStore.setState({
      categories: [
        { id: "shell", assetType: "ssh", label: "Shell", source: "builtin" } as any,
        { id: "prompt", assetType: "", label: "Prompt", source: "builtin" } as any,
      ],
      categoriesLoading: false,
      list: [],
      listLoading: false,
      filter: { categories: [], keyword: "", tag: "" },
    });
    useAssetStore.setState({
      assets: [{ ID: 1, Name: "s1", Type: "ssh", GroupID: 0 } as any],
      groups: [],
      selectedAssetId: null,
      selectedGroupId: null,
      collapsedGroupIds: [],
      loading: false,
      initialized: true,
    });
    vi.mocked(ListSnippets).mockResolvedValue([]);
  });

  it("create mode: submit disabled with empty name", () => {
    render(<SnippetFormDialog open={true} mode="create" onOpenChange={() => {}} />);
    const submit = screen.getByRole("button", { name: "snippet.actions.create" });
    expect(submit).toBeDisabled();
  });

  it("create mode: submit enabled when name + content filled, calls create", async () => {
    vi.mocked(CreateSnippet).mockResolvedValue({ ID: 1 } as any);
    const onOpenChange = vi.fn();
    render(<SnippetFormDialog open={true} mode="create" onOpenChange={onOpenChange} />);

    const nameInput = screen.getByLabelText("snippet.form.labelName") as HTMLInputElement;
    fireEvent.change(nameInput, { target: { value: " ls " } });

    const contentInput = screen.getByLabelText("snippet.form.labelContent") as HTMLTextAreaElement;
    fireEvent.change(contentInput, { target: { value: "ls -al" } });

    const tagsInput = screen.getByLabelText("snippet.form.labelTags") as HTMLInputElement;
    fireEvent.change(tagsInput, { target: { value: "FOO, Bar ,baz" } });

    const submit = screen.getByRole("button", { name: "snippet.actions.create" });
    expect(submit).not.toBeDisabled();
    fireEvent.click(submit);

    await waitFor(() => expect(CreateSnippet).toHaveBeenCalled());
    const arg = vi.mocked(CreateSnippet).mock.calls[0][0] as any;
    expect(arg.name).toBe("ls"); // trimmed
    expect(arg.tags).toBe("foo,bar,baz"); // normalized
    expect(arg.content).toBe("ls -al");
    expect(arg.category).toBe("shell"); // first category by default
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it("edit mode: prefills fields and calls update", async () => {
    vi.mocked(UpdateSnippet).mockResolvedValue({ ID: 99 } as any);
    const initial = {
      ID: 99,
      Name: "old",
      Category: "shell",
      Content: "ls",
      Description: "d",
      Tags: "a,b",
      AssetID: 1,
      Source: "user",
      SourceRef: "",
      UseCount: 0,
      Status: 1,
      CreatedAt: "2024-01-01T00:00:00Z",
      UpdatedAt: "2024-01-01T00:00:00Z",
    } as any;
    const onOpenChange = vi.fn();
    render(<SnippetFormDialog open={true} mode="edit" initial={initial} onOpenChange={onOpenChange} />);

    const nameInput = screen.getByLabelText("snippet.form.labelName") as HTMLInputElement;
    expect(nameInput.value).toBe("old");

    // Category select is rendered as a Radix trigger button. It should be disabled in edit mode.
    const categoryTrigger = document.getElementById("snippet-category");
    expect(categoryTrigger).not.toBeNull();
    expect(categoryTrigger).toBeDisabled();

    fireEvent.change(nameInput, { target: { value: "new" } });
    const submit = screen.getByRole("button", { name: "snippet.actions.save" });
    fireEvent.click(submit);

    await waitFor(() => expect(UpdateSnippet).toHaveBeenCalled());
    const arg = vi.mocked(UpdateSnippet).mock.calls[0][0] as any;
    expect(arg.id).toBe(99);
    expect(arg.name).toBe("new");
  });

  it("hides Asset binding field when selected category has empty assetType (prompt)", () => {
    const initial = {
      ID: 1,
      Name: "x",
      Category: "prompt",
      Content: "c",
      Description: "",
      Tags: "",
      Source: "user",
      SourceRef: "",
      UseCount: 0,
      Status: 1,
      CreatedAt: "2024-01-01T00:00:00Z",
      UpdatedAt: "2024-01-01T00:00:00Z",
    } as any;
    render(<SnippetFormDialog open={true} mode="edit" initial={initial} onOpenChange={() => {}} />);
    expect(screen.queryByLabelText("snippet.form.labelAsset")).toBeNull();
  });

  it("create mode: category select enabled", () => {
    render(<SnippetFormDialog open={true} mode="create" onOpenChange={() => {}} />);
    const trigger = document.getElementById("snippet-category");
    expect(trigger).not.toBeNull();
    expect(trigger).not.toBeDisabled();
  });
});
