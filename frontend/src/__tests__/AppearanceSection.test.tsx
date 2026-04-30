import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { TooltipProvider } from "@opskat/ui";
import { ThemeProvider } from "../components/theme-provider";
import { TerminalSection } from "../components/settings/AppearanceSection";
import {
  DEFAULT_TERMINAL_FONT_FAMILY,
  DEFAULT_TERMINAL_FONT_PRESET_ID,
  useTerminalThemeStore,
} from "../stores/terminalThemeStore";

function renderTerminalSection() {
  return render(
    <ThemeProvider defaultTheme="light">
      <TooltipProvider>
        <TerminalSection />
      </TooltipProvider>
    </ThemeProvider>
  );
}

describe("AppearanceSection terminal font preset select", () => {
  beforeEach(() => {
    localStorage.clear();
    useTerminalThemeStore.setState({
      selectedThemeId: "default",
      customThemes: [],
      fontSize: 14,
      fontPresetId: DEFAULT_TERMINAL_FONT_PRESET_ID,
      fontFamily: DEFAULT_TERMINAL_FONT_FAMILY,
      scrollback: 25000,
    });
  });

  it("keeps scroll styling on the viewport for long preset lists", async () => {
    renderTerminalSection();

    const trigger = screen.getByRole("combobox");
    fireEvent.click(trigger);

    const content = await screen.findByRole("listbox");
    expect(content.className).toContain("overflow-hidden");
    expect(content.className).toContain("translate-y-1");

    const viewport = content.querySelector('[data-radix-select-viewport=""]');
    expect(viewport).not.toBeNull();
    expect(viewport?.className).toContain("overflow-y-auto");
    expect(viewport?.className).toContain("[scrollbar-gutter:stable]");
    expect(viewport?.className).toContain("overscroll-contain");
    expect(viewport?.className).toContain("min-w-[var(--radix-select-trigger-width)]");
  });

  it("marks the selected preset with radix checked state", async () => {
    renderTerminalSection();

    fireEvent.click(screen.getByRole("combobox"));

    const selected = await screen.findByRole("option", { name: "Source Code Pro" });
    expect(selected).toHaveAttribute("data-state", "checked");
  });
});
