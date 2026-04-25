import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UpdateSection } from "@/components/settings/UpdateSection";
import {
  GetAppVersion,
  GetAvailableMirrors,
  GetDebugMode,
  GetDownloadMirror,
  GetUpdateChannel,
} from "../../wailsjs/go/app/App";
import { BrowserOpenURL, EventsOn } from "../../wailsjs/runtime/runtime";

describe("UpdateSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(EventsOn).mockReturnValue(vi.fn());
    vi.mocked(GetAppVersion).mockResolvedValue("dev");
    vi.mocked(GetUpdateChannel).mockResolvedValue("stable");
    vi.mocked(GetDebugMode).mockResolvedValue(false);
    vi.mocked(GetDownloadMirror).mockResolvedValue("");
    vi.mocked(GetAvailableMirrors).mockResolvedValue([]);
  });

  it("opens the project repository from settings", async () => {
    render(<UpdateSection />);

    await userEvent.click(screen.getByRole("button", { name: "appUpdate.openRepository" }));

    expect(BrowserOpenURL).toHaveBeenCalledWith("https://github.com/opskat/opskat");
  });
});
