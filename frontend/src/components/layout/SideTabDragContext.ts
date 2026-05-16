import { createContext } from "react";
import type { Tab } from "@/stores/tabStore";
import type { TabDragContextValue } from "@/hooks/useTabDragAndDrop";

export interface SideTabDragCtx extends TabDragContextValue {
  moveTo: (id: string, toIndex: number) => void;
  tabs: Tab[];
}

export const SideTabDragContext = createContext<SideTabDragCtx | null>(null);
