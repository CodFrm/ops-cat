import { vi } from "vitest";
import "@testing-library/jest-dom/vitest";

// Mock Wails runtime
vi.mock("../../wailsjs/runtime/runtime", () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
  EventsEmit: vi.fn(),
  WindowIsFullscreen: vi.fn().mockResolvedValue(false),
}));

// Mock Wails backend bindings
vi.mock("../../wailsjs/go/app/App", async () => {
  // Read the real module's export names and replace each with vi.fn()
  const actual = await vi.importActual<Record<string, unknown>>("../../wailsjs/go/app/App");
  const mocked: Record<string, unknown> = {};
  for (const key of Object.keys(actual)) {
    mocked[key] = vi.fn();
  }
  return mocked;
});

// Mock react-i18next
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en", changeLanguage: vi.fn() },
  }),
  Trans: ({ i18nKey, children }: { i18nKey?: string; children?: React.ReactNode }) => i18nKey ?? children,
  initReactI18next: { type: "3rdParty", init: vi.fn() },
}));

// Mock localStorage
const store: Record<string, string> = {};
vi.stubGlobal("localStorage", {
  getItem: (key: string) => store[key] ?? null,
  setItem: (key: string, val: string) => {
    store[key] = val;
  },
  removeItem: (key: string) => {
    delete store[key];
  },
  clear: () => {
    for (const key of Object.keys(store)) delete store[key];
  },
});
