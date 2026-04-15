"use client";

import * as React from "react";

import { cn } from "../lib/utils";

/**
 * A plain overflow-auto div that still scrolls via mouse wheel when placed
 * inside a Radix Dialog's portaled subtree (Popover, DropdownMenu, etc.).
 *
 * Radix Dialog locks body scroll via react-remove-scroll, which attaches a
 * non-passive wheel listener on document and calls preventDefault on targets
 * outside the dialog's content ref. Portaled popups live at the document root
 * and get caught by that filter, so native scrolling silently stops even
 * though a scrollbar is visible. This component attaches its own
 * { passive: false } wheel listener and drives scrollTop/scrollLeft manually,
 * bypassing the lock.
 */
function ScrollableContainer({ className, children, ...props }: Omit<React.ComponentProps<"div">, "ref">) {
  const ref = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const handler = (e: WheelEvent) => {
      e.preventDefault();
      el.scrollTop += e.deltaY;
      el.scrollLeft += e.deltaX;
    };
    el.addEventListener("wheel", handler, { passive: false });
    return () => el.removeEventListener("wheel", handler);
  }, []);

  return (
    <div
      ref={ref}
      data-slot="scrollable-container"
      className={cn("overflow-y-auto overflow-x-hidden", className)}
      {...props}
    >
      {children}
    </div>
  );
}

export { ScrollableContainer };
