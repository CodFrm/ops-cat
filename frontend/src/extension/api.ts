// frontend/src/extension/api.ts
import { CallExtensionAction, CallExtensionTool } from "../../wailsjs/go/app/App";
import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";
import type { ExtAPI, ExtEvent } from "./types";

export function createExtensionAPI(): ExtAPI {
  return {
    async callTool(extName: string, tool: string, args: unknown): Promise<unknown> {
      const argsJSON = JSON.stringify(args ?? {});
      const result = await CallExtensionTool(extName, tool, argsJSON);
      if (!result) return null;
      try {
        return JSON.parse(result);
      } catch {
        return result;
      }
    },

    async executeAction(
      extName: string,
      action: string,
      args: unknown,
      onEvent?: (e: ExtEvent) => void
    ): Promise<unknown> {
      let cleanup: (() => void) | undefined;

      if (onEvent) {
        const handler = (event: { extension: string; eventType: string; data: unknown }) => {
          if (event.extension === extName) {
            onEvent({ eventType: event.eventType, data: event.data });
          }
        };
        EventsOn("ext:action:event", handler);
        cleanup = () => EventsOff("ext:action:event");
      }

      try {
        const argsJSON = JSON.stringify(args ?? {});
        const result = await CallExtensionAction(extName, action, argsJSON);
        if (!result) return null;
        try {
          return JSON.parse(result);
        } catch {
          return result;
        }
      } finally {
        cleanup?.();
      }
    },
  };
}
