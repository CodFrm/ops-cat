import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UpdateSection } from "@/components/settings/UpdateSection";
import {
  CheckForUpdate,
  DownloadAndInstallUpdate,
  GetAppVersion,
  GetAvailableMirrors,
  GetDebugMode,
  GetDownloadMirror,
  GetUpdateChannel,
  RestartApp,
} from "../../wailsjs/go/app/App";
import { BrowserOpenURL, EventsOn, Quit } from "../../wailsjs/runtime/runtime";

describe("UpdateSection", () => {
  const repositoryURL = "https://github.com/opskat/opskat";

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(EventsOn).mockReturnValue(vi.fn());
    vi.mocked(GetAppVersion).mockResolvedValue("dev");
    vi.mocked(GetUpdateChannel).mockResolvedValue("stable");
    vi.mocked(GetDebugMode).mockResolvedValue(false);
    vi.mocked(GetDownloadMirror).mockResolvedValue("");
    vi.mocked(GetAvailableMirrors).mockResolvedValue([]);
    vi.mocked(CheckForUpdate).mockResolvedValue({
      hasUpdate: false,
      currentVersion: "dev",
      latestVersion: "dev",
      releaseNotes: "",
      releaseURL: "",
      publishedAt: "",
    });
    vi.mocked(DownloadAndInstallUpdate).mockResolvedValue();
    vi.mocked(RestartApp).mockResolvedValue();
  });

  it("shows and opens the project repository from settings", async () => {
    render(<UpdateSection />);

    expect(screen.getByText(repositoryURL)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: repositoryURL }));

    expect(BrowserOpenURL).toHaveBeenCalledWith(repositoryURL);
  });

  it("relaunches the app instead of only quitting after an update", async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue({
      hasUpdate: true,
      currentVersion: "v1.0.0",
      latestVersion: "v1.0.1",
      releaseNotes: "",
      releaseURL: "https://github.com/opskat/opskat/releases/tag/v1.0.1",
      publishedAt: "2026-05-14T00:00:00Z",
    });

    render(<UpdateSection />);

    await userEvent.click(screen.getByRole("button", { name: "appUpdate.checkUpdate" }));
    await userEvent.click(await screen.findByRole("button", { name: "appUpdate.download" }));
    await userEvent.click(await screen.findByRole("button", { name: "appUpdate.restartNow" }));

    expect(RestartApp).toHaveBeenCalledTimes(1);
    expect(Quit).not.toHaveBeenCalled();
  });
});
