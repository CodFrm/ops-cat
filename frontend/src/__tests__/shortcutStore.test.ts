import { describe, it, expect, beforeEach } from "vitest";
import { useShortcutStore, DEFAULT_SHORTCUTS, type ShortcutBinding } from "../stores/shortcutStore";

describe("shortcutStore", () => {
  beforeEach(() => {
    localStorage.clear();
    useShortcutStore.setState({
      shortcuts: { ...DEFAULT_SHORTCUTS },
      isRecording: false,
    });
  });

  describe("initial state", () => {
    it("has all default shortcuts", () => {
      const { shortcuts } = useShortcutStore.getState();
      expect(shortcuts["tab.1"]).toEqual(DEFAULT_SHORTCUTS["tab.1"]);
      expect(shortcuts["tab.close"]).toEqual(DEFAULT_SHORTCUTS["tab.close"]);
      expect(shortcuts["panel.ai"]).toEqual(DEFAULT_SHORTCUTS["panel.ai"]);
    });

    it("has panel.switch default", () => {
      const { shortcuts } = useShortcutStore.getState();
      expect(shortcuts["panel.switch"]).toEqual(DEFAULT_SHORTCUTS["panel.switch"]);
    });

    it("has panel.filter default", () => {
      const { shortcuts } = useShortcutStore.getState();
      expect(shortcuts["panel.filter"]).toEqual(DEFAULT_SHORTCUTS["panel.filter"]);
    });

    it("isRecording is false", () => {
      expect(useShortcutStore.getState().isRecording).toBe(false);
    });
  });

  describe("updateShortcut", () => {
    it("updates a single shortcut binding", () => {
      const newBinding: ShortcutBinding = { code: "KeyQ", mod: true, shift: false, alt: false };
      useShortcutStore.getState().updateShortcut("tab.close", newBinding);

      expect(useShortcutStore.getState().shortcuts["tab.close"]).toEqual(newBinding);
    });

    it("persists custom shortcuts to localStorage", () => {
      const newBinding: ShortcutBinding = { code: "KeyQ", mod: true, shift: false, alt: false };
      useShortcutStore.getState().updateShortcut("tab.close", newBinding);

      const stored = JSON.parse(localStorage.getItem("keyboard_shortcuts")!);
      expect(stored["tab.close"]).toEqual(newBinding);
    });

    it("does not persist shortcuts that match defaults", () => {
      // Set to non-default, then back to default
      const custom: ShortcutBinding = { code: "KeyQ", mod: true, shift: false, alt: false };
      useShortcutStore.getState().updateShortcut("tab.close", custom);
      useShortcutStore.getState().updateShortcut("tab.close", DEFAULT_SHORTCUTS["tab.close"]);

      expect(localStorage.getItem("keyboard_shortcuts")).toBeNull();
    });
  });

  describe("resetShortcut", () => {
    it("resets a single shortcut to default", () => {
      const custom: ShortcutBinding = { code: "KeyQ", mod: true, shift: false, alt: false };
      useShortcutStore.getState().updateShortcut("tab.close", custom);
      useShortcutStore.getState().resetShortcut("tab.close");

      expect(useShortcutStore.getState().shortcuts["tab.close"]).toEqual(DEFAULT_SHORTCUTS["tab.close"]);
    });
  });

  describe("resetAll", () => {
    it("resets all shortcuts to defaults and clears localStorage", () => {
      const custom: ShortcutBinding = { code: "KeyQ", mod: true, shift: false, alt: false };
      useShortcutStore.getState().updateShortcut("tab.close", custom);
      useShortcutStore.getState().updateShortcut("tab.1", { code: "KeyA", mod: true, shift: false, alt: false });

      useShortcutStore.getState().resetAll();

      const { shortcuts } = useShortcutStore.getState();
      expect(shortcuts["tab.close"]).toEqual(DEFAULT_SHORTCUTS["tab.close"]);
      expect(shortcuts["tab.1"]).toEqual(DEFAULT_SHORTCUTS["tab.1"]);
      expect(localStorage.getItem("keyboard_shortcuts")).toBeNull();
    });
  });

  describe("setIsRecording", () => {
    it("toggles recording state", () => {
      useShortcutStore.getState().setIsRecording(true);
      expect(useShortcutStore.getState().isRecording).toBe(true);

      useShortcutStore.getState().setIsRecording(false);
      expect(useShortcutStore.getState().isRecording).toBe(false);
    });
  });
});
