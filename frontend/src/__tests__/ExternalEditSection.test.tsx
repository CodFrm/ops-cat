import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ExternalEditSection } from "../components/settings/ExternalEditSection";
import {
  getExternalEditSettings,
  saveExternalEditSettings,
  selectExternalEditorExecutable,
  selectExternalEditWorkspaceRoot,
} from "../lib/externalEditApi";

const { toastSuccess, toastError } = vi.hoisted(() => ({
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: {
    success: toastSuccess,
    error: toastError,
  },
}));

vi.mock("../lib/externalEditApi", () => ({
  getExternalEditSettings: vi.fn(),
  saveExternalEditSettings: vi.fn(),
  selectExternalEditorExecutable: vi.fn(),
  selectExternalEditWorkspaceRoot: vi.fn(),
}));

const builtInEditor = {
  id: "system-text",
  name: "System Text Editor",
  path: "/bin/editor",
  args: [],
  builtIn: true,
  available: true,
  default: false,
};

function makeSettings(overrides: Partial<Awaited<ReturnType<typeof getExternalEditSettings>>> = {}) {
  return {
    defaultEditorId: "system-text",
    workspaceRoot: "/tmp",
    cleanupRetentionDays: 7,
    maxReadFileSizeMB: 10,
    editors: [builtInEditor],
    customEditors: [],
    ...overrides,
  };
}

describe("ExternalEditSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(getExternalEditSettings).mockResolvedValue(makeSettings());
    vi.mocked(saveExternalEditSettings).mockImplementation(async (input) =>
      makeSettings({
        defaultEditorId: input.defaultEditorId,
        workspaceRoot: input.workspaceRoot,
        cleanupRetentionDays: input.cleanupRetentionDays,
        maxReadFileSizeMB: input.maxReadFileSizeMB,
        editors: [
          { ...builtInEditor, default: input.defaultEditorId === builtInEditor.id },
          ...(input.customEditors || []).map((editor) => ({
            id: editor.id,
            name: editor.name,
            path: editor.path,
            args: editor.args || [],
            builtIn: false,
            available: true,
            default: input.defaultEditorId === editor.id,
          })),
        ],
        customEditors: input.customEditors || [],
      })
    );
    vi.mocked(selectExternalEditorExecutable).mockResolvedValue("/bin/custom-editor");
    vi.mocked(selectExternalEditWorkspaceRoot).mockResolvedValue("/tmp");
  });

  it("uses an explicit dialog when editing one custom editor", async () => {
    vi.mocked(getExternalEditSettings).mockResolvedValueOnce(
      makeSettings({
        editors: [
          { ...builtInEditor, default: false },
          {
            id: "custom-1",
            name: "VS Code",
            path: "/bin/code",
            args: ["--wait"],
            builtIn: false,
            available: true,
            default: true,
          },
        ],
        customEditors: [{ id: "custom-1", name: "VS Code", path: "/bin/code", args: ["--wait"] }],
        defaultEditorId: "custom-1",
      })
    );

    const user = userEvent.setup();
    render(<ExternalEditSection />);

    expect(await screen.findByRole("button", { name: "action.edit" })).toBeInTheDocument();
    expect(screen.queryByDisplayValue("/bin/code")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "action.edit" }));

    const dialog = await screen.findByRole("dialog");
    expect(dialog).toBeInTheDocument();
    expect(within(dialog).getByDisplayValue("VS Code")).toBeInTheDocument();
    expect(within(dialog).getByDisplayValue("/bin/code")).toBeInTheDocument();
  });

  it("adds a custom editor via a dedicated dialog and saves it", async () => {
    const user = userEvent.setup();
    render(<ExternalEditSection />);

    expect(await screen.findByText("externalEdit.settings.emptyCustomEditors")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "externalEdit.settings.addEditor" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByLabelText("asset.name"), "Custom Vim");
    await user.type(within(dialog).getByLabelText("externalEdit.settings.editorPath"), "/bin/vim");
    await user.type(within(dialog).getByLabelText("externalEdit.settings.editorArgs"), "--clean");
    await user.click(within(dialog).getByRole("button", { name: "action.add" }));
    await user.click(screen.getByRole("button", { name: "action.save" }));

    await waitFor(() => {
      expect(saveExternalEditSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          cleanupRetentionDays: 7,
          maxReadFileSizeMB: 10,
          customEditors: [
            expect.objectContaining({
              name: "Custom Vim",
              path: "/bin/vim",
              args: ["--clean"],
            }),
          ],
        })
      );
    });
  });

  it("falls back to a built-in default editor after deleting the default custom editor", async () => {
    vi.mocked(getExternalEditSettings).mockResolvedValueOnce(
      makeSettings({
        editors: [
          { ...builtInEditor, default: false },
          {
            id: "custom-1",
            name: "VS Code",
            path: "/bin/code",
            args: ["--wait"],
            builtIn: false,
            available: true,
            default: true,
          },
        ],
        customEditors: [{ id: "custom-1", name: "VS Code", path: "/bin/code", args: ["--wait"] }],
        defaultEditorId: "custom-1",
      })
    );

    const user = userEvent.setup();
    render(<ExternalEditSection />);

    expect(await screen.findByRole("button", { name: "action.delete" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "action.delete" }));
    await user.click(screen.getByRole("button", { name: "action.save" }));

    await waitFor(() => {
      expect(saveExternalEditSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          defaultEditorId: "system-text",
          cleanupRetentionDays: 7,
          maxReadFileSizeMB: 10,
          customEditors: [],
        })
      );
    });
  });

  it("saves cleanup retention days with the settings snapshot", async () => {
    const user = userEvent.setup();
    render(<ExternalEditSection />);

    const retentionInput = await screen.findByLabelText("externalEdit.settings.cleanupRetentionDays");
    await user.clear(retentionInput);
    await user.type(retentionInput, "14");
    await user.click(screen.getByRole("button", { name: "action.save" }));

    await waitFor(() => {
      expect(saveExternalEditSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          cleanupRetentionDays: 14,
          maxReadFileSizeMB: 10,
        })
      );
    });
  });

  it("loads and saves max read file size in MB", async () => {
    vi.mocked(getExternalEditSettings).mockResolvedValueOnce(
      makeSettings({
        maxReadFileSizeMB: 32,
      })
    );

    const user = userEvent.setup();
    render(<ExternalEditSection />);

    const input = await screen.findByLabelText("externalEdit.settings.maxReadFileSizeMB");
    expect(input).toHaveValue(32);
    await user.clear(input);
    await user.type(input, "64");
    await user.click(screen.getByRole("button", { name: "action.save" }));

    await waitFor(() => {
      expect(saveExternalEditSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          maxReadFileSizeMB: 64,
        })
      );
    });
  });
});
