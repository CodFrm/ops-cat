import { afterEach, describe, expect, it } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { useSettingsUiStore } from "@/stores/settingsUiStore";

// Tiny probe component: mirrors how SettingsPage reads/writes the store.
// We use it (not SettingsPage itself) to keep the test focused on the
// store-backed persistence contract without dragging in 9 section
// components and their Wails bindings.
function TabsProbe() {
  const activeTab = useSettingsUiStore((s) => s.activeTab);
  const setActiveTab = useSettingsUiStore((s) => s.setActiveTab);
  return (
    <div>
      <span data-testid="active">{activeTab}</span>
      <button onClick={() => setActiveTab("backup")}>set-backup</button>
    </div>
  );
}

afterEach(() => {
  // Reset module-global store between tests.
  useSettingsUiStore.setState({ activeTab: "ai" });
});

describe("settingsUiStore (bug: settings sub-tab resets to ai on remount)", () => {
  it("defaults to ai on first mount", () => {
    render(<TabsProbe />);
    expect(screen.getByTestId("active").textContent).toBe("ai");
  });

  it("preserves activeTab across full unmount + remount (the actual bug repro)", () => {
    const first = render(<TabsProbe />);
    fireEvent.click(screen.getByText("set-backup"));
    expect(screen.getByTestId("active").textContent).toBe("backup");
    first.unmount();

    render(<TabsProbe />);
    expect(screen.getByTestId("active").textContent).toBe("backup");
  });
});
