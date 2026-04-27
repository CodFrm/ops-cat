import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AISettingsSection } from "@/components/settings/AISettingsSection";
import {
  DetectOpsctl,
  DetectSkills,
  GetAppVersion,
  GetDataDir,
  GetOpsctlInstallDir,
  ListAIProviders,
} from "../../wailsjs/go/app/App";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";

describe("AISettingsSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(ListAIProviders).mockResolvedValue([]);
    vi.mocked(DetectOpsctl).mockResolvedValue({
      installed: false,
      path: "",
      version: "",
      embedded: false,
    });
    vi.mocked(DetectSkills).mockResolvedValue([]);
    vi.mocked(GetOpsctlInstallDir).mockResolvedValue("");
    vi.mocked(GetDataDir).mockResolvedValue("");
    vi.mocked(GetAppVersion).mockResolvedValue("dev");
  });

  it("opens the GitHub Releases page for manual opsctl CLI install", async () => {
    render(<AISettingsSection />);

    await userEvent.click(screen.getByRole("button", { name: /GitHub Releases/i }));

    expect(BrowserOpenURL).toHaveBeenCalledWith("https://github.com/opskat/opskat/releases");
  });
});
